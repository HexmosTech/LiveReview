package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

const defaultCompactionCronExpr = "30 20 * * *" // Daily at 2:00 AM IST (20:30 UTC)
const defaultRetentionDays = 30

// EventCompactionManager runs an automated background compaction job.
type EventCompactionManager struct {
	db            *sql.DB
	mu            sync.Mutex
	enabled       bool
	cronExpr      string
	retentionDays int
	cronRunner    *cron.Cron
	entryID       cron.EntryID
	ctx           context.Context
	cancel        context.CancelFunc
	running       atomic.Int32 // 1 while a cycle is in progress, 0 otherwise
}

// NewEventCompactionManager creates a new compaction manager.
func NewEventCompactionManager(db *sql.DB) *EventCompactionManager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &EventCompactionManager{
		db:            db,
		enabled:       true,
		cronExpr:      defaultCompactionCronExpr,
		retentionDays: defaultRetentionDays,
		ctx:           ctx,
		cancel:        cancel,
	}

	m.loadSettingsFromDB()
	return m
}

func (m *EventCompactionManager) loadSettingsFromDB() {
	var data []byte
	err := m.db.QueryRowContext(m.ctx, "SELECT data FROM system_settings WHERE name = 'event_compaction_settings'").Scan(&data)
	if err == nil && len(data) > 0 {
		var cfg struct {
			Enabled        *bool  `json:"enabled"`
			CronExpression string `json:"cron_expression"`
			RetentionDays  int    `json:"retention_days"`
		}
		if json.Unmarshal(data, &cfg) == nil {
			if cfg.Enabled != nil {
				m.enabled = *cfg.Enabled
			}
			if strings.TrimSpace(cfg.CronExpression) != "" {
				m.cronExpr = cfg.CronExpression
			}
			if cfg.RetentionDays > 0 {
				m.retentionDays = cfg.RetentionDays
			}
		}
	}
}

// Start launches the background cron runner. Falls back to the default cron
// expression if the configured one is invalid, so a bad DB setting never
// prevents the server from booting.
func (m *EventCompactionManager) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cronRunner = cron.New()
	entryID, err := m.cronRunner.AddFunc(m.cronExpr, func() {
		m.runCycle()
	})
	if err != nil {
		log.Warn().Str("bad_cron_expr", m.cronExpr).Err(err).Str("fallback", defaultCompactionCronExpr).Msg("[compaction] invalid cron expression in settings, falling back to default")
		m.cronExpr = defaultCompactionCronExpr
		entryID, err = m.cronRunner.AddFunc(m.cronExpr, func() {
			m.runCycle()
		})
		if err != nil {
			// Default expression is hardcoded and always valid — this should never happen.
			log.Error().Err(err).Msg("[compaction] failed to schedule even with default cron expression")
			return
		}
	}
	m.entryID = entryID
	m.cronRunner.Start()
	log.Info().Str("schedule", m.cronExpr).Bool("enabled", m.enabled).Int("retention_days", m.retentionDays).Msg("[compaction] manager started")
}

// Stop gracefully shuts down the cron runner.
func (m *EventCompactionManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Info().Msg("[compaction] manager stopping")
	if m.cronRunner != nil {
		ctx := m.cronRunner.Stop()
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
	}
	m.cancel()
}

// UpdateConfig dynamically reloads configuration without server restart.
func (m *EventCompactionManager) UpdateConfig(enabled bool, cronExpr string, retentionDays int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.enabled = enabled
	m.retentionDays = retentionDays

	if strings.TrimSpace(cronExpr) == "" {
		cronExpr = defaultCompactionCronExpr
	}

	if m.cronExpr != cronExpr {
		if m.cronRunner != nil {
			m.cronRunner.Remove(m.entryID)
			entryID, err := m.cronRunner.AddFunc(cronExpr, func() {
				m.runCycle()
			})
			if err == nil {
				m.entryID = entryID
			} else {
				log.Error().Str("cron_expr", cronExpr).Err(err).Msg("[compaction] invalid cron expression")
			}
		}
		m.cronExpr = cronExpr
	}

	log.Info().Bool("enabled", m.enabled).Str("schedule", m.cronExpr).Int("retention_days", m.retentionDays).Msg("[compaction] config updated")
}

// TriggerManualCycle runs a cycle immediately.
func (m *EventCompactionManager) TriggerManualCycle() {
	log.Info().Msg("[compaction] manual cycle triggered")
	m.runCycle()
}

// runCycle executes bulk compaction. Compaction runs in the backend process
// (single instance), so no distributed lock is required. Concurrent invocations
// (e.g. cron fires while a manual run is still in progress) are skipped.
func (m *EventCompactionManager) runCycle() {
	// Atomically claim the running slot. If another cycle is already running, skip.
	if !m.running.CompareAndSwap(0, 1) {
		log.Warn().Msg("[compaction] cycle already in progress, skipping")
		return
	}
	defer m.running.Store(0)

	m.mu.Lock()
	enabled := m.enabled
	retentionDays := m.retentionDays
	m.mu.Unlock()

	if !enabled {
		log.Info().Msg("[compaction] skipping cycle — compaction is currently disabled in settings")
		return
	}

	start := time.Now()
	log.Info().Int("retention_days", retentionDays).Msg("[compaction] cycle start")

	compacted, errs := m.executeBulkCompaction(m.ctx, retentionDays)

	log.Info().Str("elapsed", time.Since(start).Round(time.Millisecond).String()).Int64("compacted_reviews", compacted).Int("errors", errs).Msg("[compaction] cycle done")
}

// executeBulkCompaction executes compaction in safe batches:
//  1. BULK INSERT: Compaction summary markers for all eligible reviews.
//  2. BATCHED DELETE: Delete prunable log rows in chunks of 50,000 rows to prevent DB timeouts.
func (m *EventCompactionManager) executeBulkCompaction(ctx context.Context, retentionDays int) (compacted int64, errs int) {
	// Step 1: Insert summary markers for uncompacted reviews
	insertRes, err := m.db.ExecContext(ctx, `
		INSERT INTO public.review_events (review_id, org_id, ts, event_type, level, data)
		SELECT 
		    re.review_id, 
		    re.org_id, 
		    NOW(), 
		    'log', 
		    'info', 
		    jsonb_build_object(
		        'message', 'Review log events compacted',
		        'compacted', true,
		        'original_total_event_count', COUNT(*)
		    )
		FROM public.review_events re
		WHERE re.ts < NOW() - ($1 * INTERVAL '1 day')
		  AND NOT EXISTS (
		      SELECT 1 FROM public.review_events cx 
		      WHERE cx.review_id = re.review_id AND cx.org_id = re.org_id 
		        AND cx.event_type = 'log' AND (cx.data->>'compacted')::boolean = true
		  )
		GROUP BY re.review_id, re.org_id;
	`, retentionDays)
	if err != nil {
		log.Error().Err(err).Msg("[compaction] insert summary markers failed")
		return 0, 1
	}

	markersInserted, _ := insertRes.RowsAffected()

	// Step 2: Delete verbose log rows in batches of 50,000 rows to prevent query timeouts
	batchSize := 50000
	var totalDeleted int64 = 0

	deleteQuery := `
		DELETE FROM public.review_events
		WHERE ctid IN (
			SELECT ctid FROM public.review_events
			WHERE ts < NOW() - ($1 * INTERVAL '1 day')
			  AND event_type = 'log'
			  AND COALESCE(level, 'info') NOT IN ('error', 'warn')
			  AND (data->>'compacted')::boolean IS NOT TRUE
			  AND data->>'message' NOT ILIKE '%started%'
			  AND data->>'message' NOT ILIKE '%completed%'
			  AND data->>'message' NOT ILIKE '%posted%'
			  AND data->>'message' NOT ILIKE '%generated%'
			  AND data->>'message' NOT ILIKE '%CLI DIFF REVIEW STARTED%'
			LIMIT $2
		);
	`

	for {
		select {
		case <-ctx.Done():
			log.Info().Int64("rows_deleted", totalDeleted).Msg("[compaction] deletion context cancelled")
			return markersInserted, 0
		default:
		}

		res, err := m.db.ExecContext(ctx, deleteQuery, retentionDays, batchSize)
		if err != nil {
			log.Error().Err(err).Msg("[compaction] batch delete error")
			break
		}

		rows, _ := res.RowsAffected()
		totalDeleted += rows
		if rows == 0 {
			break
		}
	}

	log.Info().Int64("reviews_compacted", markersInserted).Int64("rows_deleted", totalDeleted).Msg("[compaction] bulk cycle complete")
	return markersInserted, 0
}
