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

// PreloadedChangesArchivalManager runs an automated background cron job to archive
// code diffs (preloaded_changes) older than retentionDays from PostgreSQL
// reviews.metadata into external Blob Storage.
type PreloadedChangesArchivalManager struct {
	database          *sql.DB
	jobQueue          *jobqueue.JobQueue
	mutex             sync.Mutex
	enabled           bool
	cronExpr          string
	retentionDays     int
	batchSize         int
	interBatchDelayMs int
	cronRunner        *cron.Cron
	entryID           cron.EntryID
	context           context.Context
	cancel            context.CancelFunc
	running           atomic.Int32 // 1 while a cycle is in progress, 0 otherwise
}

// NewPreloadedChangesArchivalManager creates a new preloaded_changes archival manager.
func NewPreloadedChangesArchivalManager(database *sql.DB, jobQueue *jobqueue.JobQueue) *PreloadedChangesArchivalManager {
	ctx, cancel := context.WithCancel(context.Background())
	manager := &PreloadedChangesArchivalManager{
		database:          database,
		jobQueue:          jobQueue,
		enabled:           true,
		cronExpr:          defaultArchivalCronExpr,
		retentionDays:     defaultArchivalRetentionDays,
		batchSize:         defaultArchivalBatchSize,
		interBatchDelayMs: defaultArchivalInterBatchDelayMs,
		context:           ctx,
		cancel:            cancel,
	}

	manager.loadSettingsFromDB()
	return manager
}

// SetJobQueue configures or updates the job queue instance.
func (manager *PreloadedChangesArchivalManager) SetJobQueue(jobQueue *jobqueue.JobQueue) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	manager.jobQueue = jobQueue
}

func (manager *PreloadedChangesArchivalManager) loadSettingsFromDB() {
	var data []byte
	settingName := "preloaded_changes_archival_settings"
	err := manager.database.QueryRowContext(manager.context, "SELECT data FROM system_settings WHERE name = $1", settingName).Scan(&data)
	if err == nil && len(data) > 0 {
		var config struct {
			Enabled           *bool  `json:"enabled"`
			CronExpression    string `json:"cron_expression"`
			RetentionDays     int    `json:"retention_days"`
			BatchSize         int    `json:"batch_size"`
			InterBatchDelayMs int    `json:"inter_batch_delay_ms"`
		}
		if json.Unmarshal(data, &config) == nil {
			if config.Enabled != nil {
				manager.enabled = *config.Enabled
			}
			if strings.TrimSpace(config.CronExpression) != "" {
				manager.cronExpr = config.CronExpression
			}
			if config.RetentionDays > 0 {
				manager.retentionDays = config.RetentionDays
			}
			if config.BatchSize > 0 {
				manager.batchSize = config.BatchSize
			}
			if config.InterBatchDelayMs >= 0 {
				manager.interBatchDelayMs = config.InterBatchDelayMs
			}
		}
	}
}

// Start launches the background cron runner.
func (manager *PreloadedChangesArchivalManager) Start() {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	manager.cronRunner = cron.New(cron.WithLocation(time.UTC))
	entryID, err := manager.cronRunner.AddFunc(manager.cronExpr, func() {
		manager.runCycle()
	})
	if err != nil {
		log.Warn().Str("bad_cron_expr", manager.cronExpr).Err(err).Str("fallback", defaultArchivalCronExpr).Msg("[preloaded_changes_archival] invalid cron expression in settings, falling back to default")
		manager.cronExpr = defaultArchivalCronExpr
		entryID, err = manager.cronRunner.AddFunc(manager.cronExpr, func() {
			manager.runCycle()
		})
		if err != nil {
			log.Error().Err(err).Msg("[preloaded_changes_archival] failed to schedule even with default cron expression")
			return
		}
	}
	manager.entryID = entryID
	manager.cronRunner.Start()
	nextRun := manager.cronRunner.Entry(manager.entryID).Next
	log.Info().Str("schedule", manager.cronExpr).Bool("enabled", manager.enabled).Int("retention_days", manager.retentionDays).Time("next_run", nextRun).Msg("[preloaded_changes_archival] manager started")
}

// Stop gracefully shuts down the cron runner.
func (manager *PreloadedChangesArchivalManager) Stop() {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	log.Info().Msg("[preloaded_changes_archival] manager stopping")
	if manager.cronRunner != nil {
		stopContext := manager.cronRunner.Stop()
		select {
		case <-stopContext.Done():
		case <-time.After(5 * time.Second):
		}
	}
	manager.cancel()
}

// UpdateConfig dynamically reloads configuration without server restart.
func (manager *PreloadedChangesArchivalManager) UpdateConfig(enabled bool, cronExpr string, retentionDays int, batchSize int, delayMs int) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	manager.enabled = enabled
	manager.retentionDays = retentionDays
	if batchSize > 0 {
		manager.batchSize = batchSize
	}
	if delayMs >= 0 {
		manager.interBatchDelayMs = delayMs
	}

	if strings.TrimSpace(cronExpr) == "" {
		cronExpr = defaultArchivalCronExpr
	}

	if manager.cronExpr != cronExpr || manager.cronRunner == nil {
		if manager.cronRunner != nil {
			manager.cronRunner.Stop()
		}
		manager.cronRunner = cron.New(cron.WithLocation(time.UTC))
		entryID, err := manager.cronRunner.AddFunc(cronExpr, func() {
			manager.runCycle()
		})
		if err == nil {
			manager.entryID = entryID
			manager.cronRunner.Start()
		} else {
			log.Error().Str("cron_expr", cronExpr).Err(err).Msg("[preloaded_changes_archival] invalid cron expression")
		}
		manager.cronExpr = cronExpr
	}

	nextRun := time.Time{}
	if manager.cronRunner != nil {
		nextRun = manager.cronRunner.Entry(manager.entryID).Next
	}
	log.Info().Bool("enabled", manager.enabled).Str("schedule", manager.cronExpr).Int("retention_days", manager.retentionDays).Time("next_run", nextRun).Msg("[preloaded_changes_archival] config updated")
}

// TriggerManualCycle runs a cycle immediately in background.
func (manager *PreloadedChangesArchivalManager) TriggerManualCycle() {
	log.Info().Msg("[preloaded_changes_archival] manual cycle triggered")
	go manager.runCycle()
}

// ExecuteBulkArchivalTest exposes executeBulkArchival for testing.
func (manager *PreloadedChangesArchivalManager) ExecuteBulkArchivalTest(executionCtx context.Context, retentionDays int, batchSize int, delayMs int) (archived int64, errorCount int) {
	return manager.executeBulkArchival(executionCtx, retentionDays, batchSize, delayMs)
}

// runCycle executes archival in safe batches. Concurrent invocations are skipped.
func (manager *PreloadedChangesArchivalManager) runCycle() {
	if !manager.running.CompareAndSwap(0, 1) {
		log.Warn().Msg("[preloaded_changes_archival] cycle already in progress, skipping")
		return
	}
	defer manager.running.Store(0)

	manager.mutex.Lock()
	enabled := manager.enabled
	retentionDays := manager.retentionDays
	batchSize := manager.batchSize
	delayMs := manager.interBatchDelayMs
	manager.mutex.Unlock()

	if !enabled {
		log.Info().Msg("[preloaded_changes_archival] skipping cycle — archival is currently disabled in settings")
		return
	}

	start := time.Now()
	log.Info().Int("retention_days", retentionDays).Int("batch_size", batchSize).Msg("[preloaded_changes_archival] cycle start")

	archived, errorCount := manager.executeBulkArchival(manager.context, retentionDays, batchSize, delayMs)

	log.Info().Str("elapsed", time.Since(start).Round(time.Millisecond).String()).Int64("archived_reviews", archived).Int("errors", errorCount).Msg("[preloaded_changes_archival] cycle done")
}

func (manager *PreloadedChangesArchivalManager) executeBulkArchival(executionCtx context.Context, retentionDays int, batchSize int, delayMs int) (archived int64, errorCount int) {
	if batchSize <= 0 {
		batchSize = 10
	}

	query := `
		SELECT id, COALESCE(org_id, 0)
		FROM reviews
		WHERE created_at < NOW() - ($1 * INTERVAL '1 day')
		  AND trigger_type = 'cli_diff'
		  AND metadata ? 'preloaded_changes'
		ORDER BY created_at ASC
		LIMIT $2;
	`

	rows, err := manager.database.QueryContext(executionCtx, query, retentionDays, batchSize)
	if err != nil {
		log.Error().Err(err).Msg("[preloaded_changes_archival] failed to query eligible reviews for archival")
		return 0, 1
	}
	defer rows.Close()

	type reviewRecord struct {
		reviewID       int64
		organizationID int64
	}

	organizationReviewsMap := make(map[int64][]int64)
	var totalEligible int

	for rows.Next() {
		var record reviewRecord
		if err := rows.Scan(&record.reviewID, &record.organizationID); err == nil {
			organizationReviewsMap[record.organizationID] = append(organizationReviewsMap[record.organizationID], record.reviewID)
			totalEligible++
		}
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Msg("[preloaded_changes_archival] rows iteration error")
		return 0, 1
	}

	if totalEligible == 0 {
		log.Info().Msg("[preloaded_changes_archival] no eligible reviews found for offloading")
		return 0, 0
	}

	if manager.jobQueue == nil {
		log.Error().Msg("[preloaded_changes_archival] job queue is nil, cannot enqueue archival jobs")
		return 0, 1
	}

	var totalArchivalJobs int
	batchRunPrefix := fmt.Sprintf("batch-%d", time.Now().UnixNano())

	for organizationID, reviewIDs := range organizationReviewsMap {
		batchRunID := fmt.Sprintf("%s-org-%d", batchRunPrefix, organizationID)

		// 1 job per review — clean retry isolation per River design
		archivalJobs := make([]jobqueue.PreloadedChangesArchivalJobArgs, len(reviewIDs))
		for index, reviewID := range reviewIDs {
			archivalJobs[index] = jobqueue.PreloadedChangesArchivalJobArgs{
				ReviewID:   reviewID,
				OrgID:      organizationID,
				BatchRunID: batchRunID,
			}
		}

		count, err := manager.jobQueue.QueuePreloadedChangesArchivalJobs(executionCtx, archivalJobs)
		if err != nil {
			log.Error().Err(err).Int64("org_id", organizationID).Msg("[preloaded_changes_archival] error enqueuing archival jobs")
			errorCount++
			continue
		}
		totalArchivalJobs += count

		// Enqueue single purge job for this org's batch run
		purgeJob := jobqueue.PreloadedChangesArchivalPurgeJobArgs{
			BatchRunID: batchRunID,
			ReviewIDs:  reviewIDs,
			OrgID:      organizationID,
		}
		if err := manager.jobQueue.QueuePreloadedChangesArchivalPurgeJob(executionCtx, purgeJob); err != nil {
			log.Error().Err(err).Int64("org_id", organizationID).Msg("[preloaded_changes_archival] error enqueuing archival purge job")
			errorCount++
		}

		if delayMs > 0 {
			time.Sleep(time.Duration(delayMs) * time.Millisecond)
		}
	}

	log.Info().Int("total_eligible_reviews", totalEligible).Int("enqueued_archival_jobs", totalArchivalJobs).Msg("[preloaded_changes_archival] enqueued diff archival jobs into River queue (1 job per review)")
	return int64(totalEligible), errorCount
}
