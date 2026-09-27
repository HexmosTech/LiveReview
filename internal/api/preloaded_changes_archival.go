package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/livereview/internal/jobqueue"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

const defaultArchivalCronExpr = "30 21 * * *" // 21:30 UTC = exactly 3:00 AM IST
const defaultArchivalRetentionDays = 30

var (
	ErrJobQueueNil         = errors.New("Job queue is nil")
	ErrCycleAlreadyRunning = errors.New("An archival cycle is already actively running")
)

// PreloadedChangesArchivalManager manages settings and manual triggers for preloaded_changes archival.
// Automated periodic sweeps are executed by an in-memory cron runner that enqueues a sweep job to River.
type PreloadedChangesArchivalManager struct {
	database      *sql.DB
	jobQueue      *jobqueue.JobQueue
	mutex         sync.Mutex
	enabled       bool
	cronExpr      string
	retentionDays int
	cronRunner    *cron.Cron
	entryID       cron.EntryID
	context       context.Context
	cancel        context.CancelFunc
}

// NewPreloadedChangesArchivalManager creates a new preloaded_changes archival manager.
func NewPreloadedChangesArchivalManager(database *sql.DB, jobQueue *jobqueue.JobQueue) *PreloadedChangesArchivalManager {
	ctx, cancel := context.WithCancel(context.Background())
	manager := &PreloadedChangesArchivalManager{
		database:      database,
		jobQueue:      jobQueue,
		enabled:       true,
		cronExpr:      defaultArchivalCronExpr,
		retentionDays: defaultArchivalRetentionDays,
		context:       ctx,
		cancel:        cancel,
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
			Enabled        *bool  `json:"enabled"`
			CronExpression string `json:"cron_expression"`
			RetentionDays  int    `json:"retention_days"`
		}
		if err := json.Unmarshal(data, &config); err != nil {
			log.Warn().Err(err).Msg("[preloaded_changes_archival] failed to unmarshal settings from DB")
		} else {
			if config.Enabled != nil {
				manager.enabled = *config.Enabled
			}
			if strings.TrimSpace(config.CronExpression) != "" {
				manager.cronExpr = config.CronExpression
			}
			if config.RetentionDays > 0 {
				manager.retentionDays = config.RetentionDays
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

// Stop gracefully shuts down context and cron runner.
func (manager *PreloadedChangesArchivalManager) Stop() {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	log.Info().Msg("[preloaded_changes_archival] manager stopping")
	if manager.cronRunner != nil {
		ctx := manager.cronRunner.Stop()
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
	}
	manager.cancel()
}

// UpdateConfig dynamically reloads configuration without server restart.
func (manager *PreloadedChangesArchivalManager) UpdateConfig(enabled bool, cronExpr string, retentionDays int) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	manager.enabled = enabled
	manager.retentionDays = retentionDays

	if strings.TrimSpace(cronExpr) == "" {
		cronExpr = defaultArchivalCronExpr
	}

	if manager.cronExpr != cronExpr || manager.cronRunner == nil {
		if manager.cronRunner != nil {
			manager.cronRunner.Stop()
			manager.cronRunner = nil
		}
		newRunner := cron.New(cron.WithLocation(time.UTC))
		entryID, err := newRunner.AddFunc(cronExpr, func() {
			manager.runCycle()
		})
		if err == nil {
			manager.cronRunner = newRunner
			manager.entryID = entryID
			manager.cronRunner.Start()
			manager.cronExpr = cronExpr
		} else {
			log.Error().Str("cron_expr", cronExpr).Err(err).Msg("[preloaded_changes_archival] invalid cron expression")
			manager.cronExpr = ""
		}
	}

	nextRun := time.Time{}
	if manager.cronRunner != nil {
		nextRun = manager.cronRunner.Entry(manager.entryID).Next
	}
	log.Info().Bool("enabled", manager.enabled).Str("schedule", manager.cronExpr).Int("retention_days", manager.retentionDays).Time("next_run", nextRun).Msg("[preloaded_changes_archival] config updated")
}

// IsArchivalCycleRunning checks if ANY archival-related job is currently active.
func (manager *PreloadedChangesArchivalManager) IsArchivalCycleRunning() (bool, error) {
	manager.mutex.Lock()
	jobQueue := manager.jobQueue
	ctx := manager.context
	manager.mutex.Unlock()

	if jobQueue != nil {
		return jobQueue.IsArchivalCycleActive(ctx, 0)
	}
	return false, ErrJobQueueNil
}

// TriggerManualCycle enqueues a sweep job into River queue.
func (manager *PreloadedChangesArchivalManager) TriggerManualCycle() error {
	log.Info().Msg("[preloaded_changes_archival] manual cycle triggered (enqueuing sweep job to River)")
	return manager.runCycleCore(true)
}

func (manager *PreloadedChangesArchivalManager) runCycle() {
	manager.mutex.Lock()
	enabled := manager.enabled
	manager.mutex.Unlock()

	if !enabled {
		log.Info().Msg("[preloaded_changes_archival] skipping periodic sweep — feature is disabled in settings")
		return
	}

	log.Info().Msg("[preloaded_changes_archival] periodic cycle triggered (enqueuing sweep job to River)")
	if err := manager.runCycleCore(false); err != nil {
		log.Error().Err(err).Msg("[preloaded_changes_archival] failed to enqueue periodic sweep job")
	}
}

func (manager *PreloadedChangesArchivalManager) runCycleCore(isManual bool) error {
	isRunning, err := manager.IsArchivalCycleRunning()
	if err != nil {
		if isManual {
			log.Error().Err(err).Msg("[preloaded_changes_archival] failed to check if archival cycle is running")
		}
		return err
	}
	if isRunning {
		if isManual {
			log.Warn().Msg("[preloaded_changes_archival] manual trigger ignored: an archival cycle is already running")
		} else {
			log.Warn().Msg("[preloaded_changes_archival] periodic trigger ignored: an archival cycle is already running")
		}
		return ErrCycleAlreadyRunning
	}

	manager.mutex.Lock()
	retentionDays := manager.retentionDays
	jobQueue := manager.jobQueue
	ctx := manager.context
	manager.mutex.Unlock()

	if jobQueue == nil {
		return ErrJobQueueNil
	}

	err = jobQueue.EnqueuePreloadedChangesArchivalSweep(ctx, retentionDays)
	if err != nil {
		return fmt.Errorf("failed to enqueue sweep job to River: %w", err)
	}
	return nil
}

