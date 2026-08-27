package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

const defaultCompactionCronExpr = "30 20 * * *" // Daily at 2:00 AM IST (20:30 UTC)
const defaultRetentionDays = 30

type compactionLeaderLocker interface {
	TryAcquireEventCompactionLeaderLock(ctx context.Context) (bool, error)
}

// EventCompactionManager runs an automated background compaction job.
type EventCompactionManager struct {
	db            *sql.DB
	lockStore     compactionLeaderLocker
	mu            sync.Mutex
	enabled       bool
	cronExpr      string
	retentionDays int
	cronRunner    *cron.Cron
	entryID       cron.EntryID
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewEventCompactionManager creates a new compaction manager.
func NewEventCompactionManager(db *sql.DB, lockStore compactionLeaderLocker, customInterval time.Duration) *EventCompactionManager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &EventCompactionManager{
		db:            db,
		lockStore:     lockStore,
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
	err := m.db.QueryRow("SELECT data FROM system_settings WHERE name = 'event_compaction_settings'").Scan(&data)
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

// Start launches the background cron runner.
func (m *EventCompactionManager) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cronRunner = cron.New()
	entryID, err := m.cronRunner.AddFunc(m.cronExpr, func() {
		m.runCycle()
	})
	if err != nil {
		log.Printf("[compaction] failed to schedule cron %q: %v", m.cronExpr, err)
		return
	}
	m.entryID = entryID
	m.cronRunner.Start()
	log.Printf("[compaction] manager started schedule=%q enabled=%v retention_days=%d", m.cronExpr, m.enabled, m.retentionDays)
}

// Stop gracefully shuts down the cron runner.
func (m *EventCompactionManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Printf("[compaction] manager stopping")
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
				log.Printf("[compaction] invalid cron expression %q: %v", cronExpr, err)
			}
		}
		m.cronExpr = cronExpr
	}

	log.Printf("[compaction] config updated: enabled=%v schedule=%q retention_days=%d", m.enabled, m.cronExpr, m.retentionDays)
}

// TriggerManualCycle runs a cycle immediately.
func (m *EventCompactionManager) TriggerManualCycle() {
	log.Printf("[compaction] manual cycle triggered")
	m.runCycle()
}

// runCycle acquires the leader lock and executes bulk compaction.
func (m *EventCompactionManager) runCycle() {
	m.mu.Lock()
	enabled := m.enabled
	retentionDays := m.retentionDays
	m.mu.Unlock()

	if !enabled {
		log.Printf("[compaction] skipping cycle — compaction is currently disabled in settings")
		return
	}

	start := time.Now()
	log.Printf("[compaction] cycle start retention_days=%d", retentionDays)

	compacted, errs := m.executeBulkCompaction(m.ctx, retentionDays)

	log.Printf("[compaction] cycle done elapsed=%s compacted_reviews=%d errors=%d",
		time.Since(start).Round(time.Millisecond), compacted, errs)
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
		log.Printf("[compaction] insert summary markers failed: %v", err)
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
			log.Printf("[compaction] deletion context cancelled after deleting %d rows", totalDeleted)
			return markersInserted, 0
		default:
		}

		res, err := m.db.ExecContext(ctx, deleteQuery, retentionDays, batchSize)
		if err != nil {
			log.Printf("[compaction] batch delete error: %v", err)
			break
		}

		rows, _ := res.RowsAffected()
		totalDeleted += rows
		if rows == 0 {
			break
		}
	}

	log.Printf("[compaction] bulk cycle complete: reviews_compacted=%d rows_deleted=%d", markersInserted, totalDeleted)
	return markersInserted, 0
}
