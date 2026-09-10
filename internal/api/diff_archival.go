package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/livereview/internal/jobqueue"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

const defaultArchivalCronExpr = "30 21 * * *" // Daily at 9:00 PM IST (local time)
const defaultArchivalRetentionDays = 30
const defaultArchivalBatchSize = 50
const defaultArchivalInterBatchDelayMs = 50

// DiffArchivalManager runs an automated background cron job to archive
// code diffs (preloaded_changes) older than retentionDays from PostgreSQL
// reviews.metadata into external Blob Storage.
type DiffArchivalManager struct {
	db                *sql.DB
	jq                *jobqueue.JobQueue
	mu                sync.Mutex
	enabled           bool
	cronExpr          string
	retentionDays     int
	batchSize         int
	interBatchDelayMs int
	cronRunner        *cron.Cron
	entryID           cron.EntryID
	ctx               context.Context
	cancel            context.CancelFunc
	running           atomic.Int32 // 1 while a cycle is in progress, 0 otherwise
}

// NewDiffArchivalManager creates a new diff archival manager.
func NewDiffArchivalManager(db *sql.DB, jq *jobqueue.JobQueue) *DiffArchivalManager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &DiffArchivalManager{
		db:                db,
		jq:                jq,
		enabled:           true,
		cronExpr:          defaultArchivalCronExpr,
		retentionDays:     defaultArchivalRetentionDays,
		batchSize:         defaultArchivalBatchSize,
		interBatchDelayMs: defaultArchivalInterBatchDelayMs,
		ctx:               ctx,
		cancel:            cancel,
	}

	m.loadSettingsFromDB()
	return m
}

// SetJobQueue configures or updates the job queue instance.
func (m *DiffArchivalManager) SetJobQueue(jq *jobqueue.JobQueue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jq = jq
}

func (m *DiffArchivalManager) loadSettingsFromDB() {
	var data []byte
	err := m.db.QueryRowContext(m.ctx, "SELECT data FROM system_settings WHERE name = 'diff_archival_settings'").Scan(&data)
	if err == nil && len(data) > 0 {
		var cfg struct {
			Enabled           *bool  `json:"enabled"`
			CronExpression    string `json:"cron_expression"`
			RetentionDays     int    `json:"retention_days"`
			BatchSize         int    `json:"batch_size"`
			InterBatchDelayMs int    `json:"inter_batch_delay_ms"`
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
			if cfg.BatchSize > 0 {
				m.batchSize = cfg.BatchSize
			}
			if cfg.InterBatchDelayMs >= 0 {
				m.interBatchDelayMs = cfg.InterBatchDelayMs
			}
		}
	}
}

// Start launches the background cron runner.
func (m *DiffArchivalManager) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cronRunner = cron.New(cron.WithLocation(time.UTC))
	entryID, err := m.cronRunner.AddFunc(m.cronExpr, func() {
		m.runCycle()
	})
	if err != nil {
		log.Warn().Str("bad_cron_expr", m.cronExpr).Err(err).Str("fallback", defaultArchivalCronExpr).Msg("[diff_archival] invalid cron expression in settings, falling back to default")
		m.cronExpr = defaultArchivalCronExpr
		entryID, err = m.cronRunner.AddFunc(m.cronExpr, func() {
			m.runCycle()
		})
		if err != nil {
			log.Error().Err(err).Msg("[diff_archival] failed to schedule even with default cron expression")
			return
		}
	}
	m.entryID = entryID
	m.cronRunner.Start()
	nextRun := m.cronRunner.Entry(m.entryID).Next
	log.Info().Str("schedule", m.cronExpr).Bool("enabled", m.enabled).Int("retention_days", m.retentionDays).Time("next_run", nextRun).Msg("[diff_archival] manager started")
}

// Stop gracefully shuts down the cron runner.
func (m *DiffArchivalManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Info().Msg("[diff_archival] manager stopping")
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
func (m *DiffArchivalManager) UpdateConfig(enabled bool, cronExpr string, retentionDays int, batchSize int, delayMs int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.enabled = enabled
	m.retentionDays = retentionDays
	if batchSize > 0 {
		m.batchSize = batchSize
	}
	if delayMs >= 0 {
		m.interBatchDelayMs = delayMs
	}

	if strings.TrimSpace(cronExpr) == "" {
		cronExpr = defaultArchivalCronExpr
	}

	if m.cronExpr != cronExpr || m.cronRunner == nil {
		if m.cronRunner != nil {
			m.cronRunner.Stop()
		}
		m.cronRunner = cron.New(cron.WithLocation(time.UTC))
		entryID, err := m.cronRunner.AddFunc(cronExpr, func() {
			m.runCycle()
		})
		if err == nil {
			m.entryID = entryID
			m.cronRunner.Start()
		} else {
			log.Error().Str("cron_expr", cronExpr).Err(err).Msg("[diff_archival] invalid cron expression")
		}
		m.cronExpr = cronExpr
	}

	nextRun := time.Time{}
	if m.cronRunner != nil {
		nextRun = m.cronRunner.Entry(m.entryID).Next
	}
	log.Info().Bool("enabled", m.enabled).Str("schedule", m.cronExpr).Int("retention_days", m.retentionDays).Time("next_run", nextRun).Msg("[diff_archival] config updated")
}

// TriggerManualCycle runs a cycle immediately in background.
func (m *DiffArchivalManager) TriggerManualCycle() {
	log.Info().Msg("[diff_archival] manual cycle triggered")
	go m.runCycle()
}

// ExecuteBulkArchivalTest exposes executeBulkArchival for testing.
func (m *DiffArchivalManager) ExecuteBulkArchivalTest(ctx context.Context, retentionDays int, batchSize int, delayMs int) (archived int64, errs int) {
	return m.executeBulkArchival(ctx, retentionDays, batchSize, delayMs)
}

// runCycle executes archival in safe batches. Concurrent invocations are skipped.
func (m *DiffArchivalManager) runCycle() {
	if !m.running.CompareAndSwap(0, 1) {
		log.Warn().Msg("[diff_archival] cycle already in progress, skipping")
		return
	}
	defer m.running.Store(0)

	m.mu.Lock()
	enabled := m.enabled
	retentionDays := m.retentionDays
	batchSize := m.batchSize
	delayMs := m.interBatchDelayMs
	m.mu.Unlock()

	if !enabled {
		log.Info().Msg("[diff_archival] skipping cycle — archival is currently disabled in settings")
		return
	}

	start := time.Now()
	log.Info().Int("retention_days", retentionDays).Int("batch_size", batchSize).Msg("[diff_archival] cycle start")

	archived, errs := m.executeBulkArchival(m.ctx, retentionDays, batchSize, delayMs)

	log.Info().Str("elapsed", time.Since(start).Round(time.Millisecond).String()).Int64("archived_reviews", archived).Int("errors", errs).Msg("[diff_archival] cycle done")
}

func (m *DiffArchivalManager) executeBulkArchival(ctx context.Context, retentionDays int, batchSize int, delayMs int) (archived int64, errs int) {
	if batchSize <= 0 {
		batchSize = 10
	}

	query := `
		SELECT id, COALESCE(org_id, 0)
		FROM reviews
		WHERE created_at < NOW() - ($1 * INTERVAL '1 day')
		  AND trigger_type = 'cli_diff'
		  AND metadata ? 'preloaded_changes'
		ORDER BY created_at ASC;
	`

	rows, err := m.db.QueryContext(ctx, query, retentionDays)
	if err != nil {
		log.Error().Err(err).Msg("[diff_archival] failed to query eligible reviews for archival")
		return 0, 1
	}
	defer rows.Close()

	type reviewRecord struct {
		id    int64
		orgID int64
	}

	orgMap := make(map[int64][]int64)
	var totalEligible int

	for rows.Next() {
		var rec reviewRecord
		if err := rows.Scan(&rec.id, &rec.orgID); err == nil {
			orgMap[rec.orgID] = append(orgMap[rec.orgID], rec.id)
			totalEligible++
		}
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Msg("[diff_archival] rows iteration error")
		return 0, 1
	}

	if totalEligible == 0 {
		log.Info().Msg("[diff_archival] no eligible reviews found for offloading")
		return 0, 0
	}

	if m.jq != nil {
		var totalArchivalJobs int
		batchRunPrefix := fmt.Sprintf("batch-%d", time.Now().UnixNano())

		for orgID, reviewIDs := range orgMap {
			// 1 job per review — clean retry isolation per River design
			archivalJobs := make([]jobqueue.DiffArchivalJobArgs, len(reviewIDs))
			for i, id := range reviewIDs {
				archivalJobs[i] = jobqueue.DiffArchivalJobArgs{
					ReviewID: id,
					OrgID:    orgID,
				}
			}

			count, err := m.jq.QueueDiffArchivalJobs(ctx, archivalJobs)
			if err != nil {
				log.Error().Err(err).Int64("org_id", orgID).Msg("[diff_archival] error enqueuing archival jobs")
				errs++
				continue
			}
			totalArchivalJobs += count

			// Enqueue single purge job for this org's batch run
			batchRunID := fmt.Sprintf("%s-org-%d", batchRunPrefix, orgID)
			purgeJob := jobqueue.DiffArchivalPurgeJobArgs{
				BatchRunID: batchRunID,
				ReviewIDs:  reviewIDs,
				OrgID:      orgID,
			}
			if err := m.jq.QueueDiffArchivalPurgeJob(ctx, purgeJob); err != nil {
				log.Error().Err(err).Int64("org_id", orgID).Msg("[diff_archival] error enqueuing archival purge job")
				errs++
			}
		}

		log.Info().Int("total_eligible_reviews", totalEligible).Int("enqueued_archival_jobs", totalArchivalJobs).Msg("[diff_archival] enqueued diff archival jobs into River queue (1 job per review)")
		return int64(totalEligible), errs
	}

	log.Warn().Msg("[diff_archival] JobQueue is nil, skipping archival")
	return 0, 0
}
