package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/livereview/internal/jobqueue"
	"github.com/rs/zerolog/log"
)

const defaultArchivalCronExpr = "30 21 * * *" // 21:30 UTC = exactly 3:00 AM IST
const defaultArchivalRetentionDays = 30

var (
	ErrJobQueueNil         = errors.New("Job queue is nil")
	ErrCycleAlreadyRunning = errors.New("An archival cycle is already actively running")
)

// PreloadedChangesArchivalManager manages settings and manual triggers for preloaded_changes archival.
// Automated periodic sweeps are executed by River job queue (River PeriodicJobs).
type PreloadedChangesArchivalManager struct {
	database      *sql.DB
	jobQueue      *jobqueue.JobQueue
	mutex         sync.Mutex
	enabled       bool
	cronExpr      string
	retentionDays int
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

// Start logs that manager is active and applies the configured schedule to River.
func (manager *PreloadedChangesArchivalManager) Start() {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	if manager.jobQueue != nil && manager.cronExpr != "" {
		if err := manager.jobQueue.UpdateArchivalSchedule(manager.cronExpr); err != nil {
			log.Error().Err(err).Str("cron_expr", manager.cronExpr).Msg("[preloaded_changes_archival] failed to apply River periodic schedule")
		}
	}

	log.Info().Str("schedule", manager.cronExpr).Bool("enabled", manager.enabled).Int("retention_days", manager.retentionDays).Msg("[preloaded_changes_archival] manager started (periodic execution handled by River)")
}

// Stop gracefully shuts down context.
func (manager *PreloadedChangesArchivalManager) Stop() {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	log.Info().Msg("[preloaded_changes_archival] manager stopping")
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
	manager.cronExpr = cronExpr

	if manager.jobQueue != nil {
		if err := manager.jobQueue.UpdateArchivalSchedule(cronExpr); err != nil {
			log.Error().Err(err).Str("cron_expr", cronExpr).Msg("[preloaded_changes_archival] failed to update River periodic schedule")
		}
	}

	log.Info().Bool("enabled", manager.enabled).Str("schedule", manager.cronExpr).Int("retention_days", manager.retentionDays).Msg("[preloaded_changes_archival] config updated")
}

// IsArchivalCycleRunning checks if ANY archival-related job is currently active.
func (manager *PreloadedChangesArchivalManager) IsArchivalCycleRunning() (bool, error) {
	if manager.jobQueue != nil {
		return manager.jobQueue.IsArchivalCycleActive(manager.context, 0)
	}
	return false, ErrJobQueueNil
}

// TriggerManualCycle enqueues a sweep job into River queue, ensuring manual triggers follow the exact same flow.
func (manager *PreloadedChangesArchivalManager) TriggerManualCycle() error {
	isRunning, err := manager.IsArchivalCycleRunning()
	if err != nil {
		log.Error().Err(err).Msg("[preloaded_changes_archival] failed to check if archival cycle is running")
		return err
	}
	if isRunning {
		log.Warn().Msg("[preloaded_changes_archival] manual trigger ignored: an archival cycle is already running")
		return ErrCycleAlreadyRunning
	}

	log.Info().Msg("[preloaded_changes_archival] manual cycle triggered (enqueuing sweep job to River)")
	if manager.jobQueue != nil {
		err := manager.jobQueue.EnqueuePreloadedChangesArchivalSweep(manager.context, manager.retentionDays)
		if err != nil {
			log.Error().Err(err).Msg("[preloaded_changes_archival] failed to enqueue manual sweep job to River")
			return fmt.Errorf("failed to enqueue manual sweep job to River: %w", err)
		}
		return nil
	}
	
	log.Error().Msg("[preloaded_changes_archival] job queue is nil, cannot enqueue sweep job")
	return ErrJobQueueNil
}

