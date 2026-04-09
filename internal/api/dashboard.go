package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
)

// WebhookHealthSummary represents overall webhook health across all connectors
type WebhookHealthSummary struct {
	TotalConnectors     int     `json:"total_connectors"`
	TotalProjects       int     `json:"total_projects"`
	ConnectedProjects   int     `json:"connected_projects"`
	UnconnectedProjects int     `json:"unconnected_projects"`
	HealthPercent       float64 `json:"health_percent"`
	// HealthStatus: "healthy" (100%), "partial" (1-99%), "setup_required" (0% or no connectors)
	HealthStatus string `json:"health_status"`
	// SetupRequiredConnectors is the number of connectors that need webhook setup
	SetupRequiredConnectors int `json:"setup_required_connectors"`
	// MostRecentConnectorNeedingSetup is the ID of the most recently created connector that needs setup
	MostRecentConnectorNeedingSetupID   *int64  `json:"most_recent_connector_needing_setup_id,omitempty"`
	MostRecentConnectorNeedingSetupName *string `json:"most_recent_connector_needing_setup_name,omitempty"`
}

// ConnectorSetupProgress tracks the setup progress for connectors that need attention
type ConnectorSetupProgress struct {
	ConnectorID   int64  `json:"connector_id"`
	ConnectorName string `json:"connector_name"`
	Provider      string `json:"provider"`
	// Phase: "discovering" (no projects yet), "installing" (projects exist, webhooks in progress), "ready" (all done), "error" (something failed)
	Phase string `json:"phase"`
	// Progress details
	TotalProjects     int `json:"total_projects"`
	ConnectedProjects int `json:"connected_projects"`
	// Message to display to the user
	Message string `json:"message"`
}

// DashboardData represents the structure of dashboard information
type DashboardData struct {
	// Statistics
	TotalReviews       int `json:"total_reviews"`
	TotalComments      int `json:"total_comments"`
	ConnectedProviders int `json:"connected_providers"`
	ActiveAIConnectors int `json:"active_ai_connectors"`

	// Webhook Health
	WebhookHealth *WebhookHealthSummary `json:"webhook_health,omitempty"`

	// Connector Setup Progress - shows connectors that need attention
	ConnectorSetupProgress []ConnectorSetupProgress `json:"connector_setup_progress,omitempty"`

	// Onboarding
	OnboardingAPIKey string `json:"onboarding_api_key,omitempty"`
	APIUrl           string `json:"api_url"`
	CLIInstalled     bool   `json:"cli_installed"`

	// Recent Activity
	RecentActivity []ActivityItem `json:"recent_activity"`

	// Performance Metrics
	PerformanceMetrics PerformanceMetrics `json:"performance_metrics"`

	// System Status
	SystemStatus SystemStatus `json:"system_status"`

	// Last updated timestamp
	LastUpdated time.Time `json:"last_updated"`
}

// ActivityItem represents a single activity entry
type ActivityItem struct {
	ID         int       `json:"id"`
	Action     string    `json:"action"`
	Repository string    `json:"repository"`
	Timestamp  time.Time `json:"timestamp"`
	TimeAgo    string    `json:"time_ago"`
	Type       string    `json:"type"` // "review", "comment", "connection", etc.
}

// PerformanceMetrics represents system performance data
type PerformanceMetrics struct {
	AvgResponseTime  float64 `json:"avg_response_time_seconds"`
	ReviewsThisWeek  int     `json:"reviews_this_week"`
	CommentsThisWeek int     `json:"comments_this_week"`
	SuccessRate      float64 `json:"success_rate_percentage"`
}

// SystemStatus represents current system status
type SystemStatus struct {
	JobQueueHealth  string    `json:"job_queue_health"`
	DatabaseHealth  string    `json:"database_health"`
	APIHealth       string    `json:"api_health"`
	LastHealthCheck time.Time `json:"last_health_check"`
}

const (
	dashboardCacheTTL         = 5 * time.Minute
	dashboardRefreshInterval  = 5 * time.Minute
	dashboardDBQueryTimeout   = 15 * time.Second
	dashboardLockReleaseAfter = 2 * time.Second
	dashboardTriggerStartup   = "startup"
	dashboardTriggerTicker    = "ticker"
	dashboardTriggerCacheMiss = "cache_miss"
	dashboardTriggerManual    = "manual"
)

type dashboardLeaderLockStore interface {
	TryAcquireDashboardRefreshLeaderLock(ctx context.Context) (bool, error)
	ReleaseDashboardRefreshLeaderLock(ctx context.Context) error
}

type dashboardLogLevel int

const (
	dashboardLogLevelOff dashboardLogLevel = iota
	dashboardLogLevelErrorsOnly
	dashboardLogLevelMinimal
	dashboardLogLevelVerbose
)

func parseDashboardLogLevel(raw string) dashboardLogLevel {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "off":
		return dashboardLogLevelOff
	case "errors_only":
		return dashboardLogLevelErrorsOnly
	case "minimal", "":
		return dashboardLogLevelMinimal
	case "verbose":
		return dashboardLogLevelVerbose
	default:
		return dashboardLogLevelMinimal
	}
}

func (l dashboardLogLevel) String() string {
	switch l {
	case dashboardLogLevelOff:
		return "off"
	case dashboardLogLevelErrorsOnly:
		return "errors_only"
	case dashboardLogLevelMinimal:
		return "minimal"
	case dashboardLogLevelVerbose:
		return "verbose"
	default:
		return "minimal"
	}
}

// DashboardManager handles dashboard data updates and retrieval
type DashboardManager struct {
	db     *sql.DB
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
	cache  map[int64]DashboardData

	lockStore dashboardLeaderLockStore
	instance  string
	logLevel  dashboardLogLevel

	refreshInProgress atomic.Bool
	refreshCycleID    uint64
}

// NewDashboardManager creates a new dashboard manager
func NewDashboardManager(db *sql.DB, lockStore dashboardLeaderLockStore) *DashboardManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &DashboardManager{
		db:        db,
		ctx:       ctx,
		cancel:    cancel,
		cache:     make(map[int64]DashboardData),
		lockStore: lockStore,
		instance:  dashboardInstanceID(),
		logLevel:  parseDashboardLogLevel(os.Getenv("DASHBOARD_LOG_LEVEL")),
	}
}

func dashboardInstanceID() string {
	if hostname := strings.TrimSpace(os.Getenv("HOSTNAME")); hostname != "" {
		return hostname
	}
	return fmt.Sprintf("pid-%d", os.Getpid())
}

func dashboardAPIURL() string {
	if configuredURL := strings.TrimSpace(os.Getenv("LIVEREVIEW_API_URL")); configuredURL != "" {
		return configuredURL
	}

	port := getEnvInt("LIVEREVIEW_BACKEND_PORT", 8888)
	return fmt.Sprintf("http://localhost:%d", port)
}

func (dm *DashboardManager) shouldLog(level dashboardLogLevel) bool {
	return dm.logLevel >= level
}

func (dm *DashboardManager) logErrorf(format string, args ...interface{}) {
	if dm.shouldLog(dashboardLogLevelErrorsOnly) {
		log.Printf(format, args...)
	}
}

func (dm *DashboardManager) logMinimalf(format string, args ...interface{}) {
	if dm.shouldLog(dashboardLogLevelMinimal) {
		log.Printf(format, args...)
	}
}

func (dm *DashboardManager) logVerbosef(format string, args ...interface{}) {
	if dm.shouldLog(dashboardLogLevelVerbose) {
		log.Printf(format, args...)
	}
}

func (dm *DashboardManager) canRunPeriodicAllOrgRefresh(ctx context.Context, trigger string) bool {
	leader, err := dm.lockStore.TryAcquireDashboardRefreshLeaderLock(ctx)
	if err != nil {
		dm.logErrorf("[dashboard] refresh lock error trigger=%s instance=%s err=%v", trigger, dm.instance, err)
		return false
	}
	if !leader {
		dm.logVerbosef("[dashboard] refresh skipped trigger=%s instance=%s reason=not_leader", trigger, dm.instance)
		return false
	}
	return true
}

func (dm *DashboardManager) beginRefreshCycle(trigger string) (uint64, bool) {
	if !dm.refreshInProgress.CompareAndSwap(false, true) {
		dm.logMinimalf("[dashboard] refresh skipped trigger=%s instance=%s reason=in_progress", trigger, dm.instance)
		return 0, false
	}
	return atomic.AddUint64(&dm.refreshCycleID, 1), true
}

func (dm *DashboardManager) endRefreshCycle() {
	dm.refreshInProgress.Store(false)
}

func (dm *DashboardManager) releaseLeaderLock(ctx context.Context, trigger string) error {
	releaseCtx, cancel := context.WithTimeout(ctx, dashboardLockReleaseAfter)
	defer cancel()

	releaseErr := dm.lockStore.ReleaseDashboardRefreshLeaderLock(releaseCtx)
	if errors.Is(releaseErr, context.Canceled) || errors.Is(releaseErr, context.DeadlineExceeded) {
		fallbackCtx, fallbackCancel := context.WithTimeout(context.Background(), dashboardLockReleaseAfter)
		defer fallbackCancel()
		return dm.lockStore.ReleaseDashboardRefreshLeaderLock(fallbackCtx)
	}

	return releaseErr
}

func (dm *DashboardManager) runRefreshCycle(ctx context.Context, trigger string) (retErr error) {
	defer func() {
		releaseErr := dm.releaseLeaderLock(ctx, trigger)
		if releaseErr != nil {
			dm.logErrorf("[dashboard] refresh lock release failed trigger=%s instance=%s err=%v", trigger, dm.instance, releaseErr)
			if retErr == nil {
				retErr = fmt.Errorf("failed to release dashboard leader lock: %w", releaseErr)
			}
		}
	}()

	cycleID, ok := dm.beginRefreshCycle(trigger)
	if !ok {
		return nil
	}
	defer dm.endRefreshCycle()

	startedAt := time.Now()
	err := dm.updateDashboardData(ctx, trigger, cycleID)
	durationMs := time.Since(startedAt).Milliseconds()

	if err != nil {
		dm.logErrorf("[dashboard] refresh failed cycle=%d trigger=%s instance=%s duration_ms=%d err=%v", cycleID, trigger, dm.instance, durationMs, err)
		return err
	}

	dm.logMinimalf("[dashboard] refresh complete cycle=%d trigger=%s instance=%s duration_ms=%d", cycleID, trigger, dm.instance, durationMs)
	return nil
}

// Start begins the background dashboard data collection
func (dm *DashboardManager) Start() {
	dm.logMinimalf("[dashboard] manager started instance=%s log_level=%s", dm.instance, dm.logLevel.String())

	if dm.canRunPeriodicAllOrgRefresh(dm.ctx, dashboardTriggerStartup) {
		if err := dm.runRefreshCycle(dm.ctx, dashboardTriggerStartup); err != nil {
			dm.logErrorf("[dashboard] initial refresh failed instance=%s err=%v", dm.instance, err)
		}
	}

	ticker := time.NewTicker(dashboardRefreshInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-dm.ctx.Done():
				dm.logMinimalf("[dashboard] manager stopped instance=%s", dm.instance)
				return
			case <-ticker.C:
				if !dm.canRunPeriodicAllOrgRefresh(dm.ctx, dashboardTriggerTicker) {
					continue
				}
				if err := dm.runRefreshCycle(dm.ctx, dashboardTriggerTicker); err != nil {
					dm.logErrorf("[dashboard] periodic refresh failed instance=%s err=%v", dm.instance, err)
				}
			}
		}
	}()
}

// Stop stops the dashboard manager
func (dm *DashboardManager) Stop() {
	dm.logMinimalf("[dashboard] manager stopping instance=%s", dm.instance)
	dm.cancel()
}

// updateDashboardData collects and updates dashboard metrics
func (dm *DashboardManager) updateDashboardData(ctx context.Context, trigger string, cycleID uint64) error {
	start := time.Now()
	dm.logMinimalf("[dashboard] refresh start cycle=%d trigger=%s instance=%s", cycleID, trigger, dm.instance)

	orgIDs, err := dm.getAllOrgIDs(ctx)
	if err != nil {
		return fmt.Errorf("failed to list organizations: %w", err)
	}

	successCount := 0
	failureCount := 0

	for _, orgID := range orgIDs {
		data, buildErr := dm.buildDashboardData(ctx, orgID, trigger, cycleID)
		if buildErr != nil {
			dm.logErrorf("[dashboard] org refresh failed cycle=%d trigger=%s org_id=%d err=%v", cycleID, trigger, orgID, buildErr)
			failureCount++
			continue
		}

		dm.mu.Lock()
		dm.cache[orgID] = data
		dm.mu.Unlock()
		successCount++
	}

	dm.logMinimalf(
		"[dashboard] refresh summary cycle=%d trigger=%s instance=%s org_total=%d org_success=%d org_failed=%d duration_ms=%d",
		cycleID,
		trigger,
		dm.instance,
		len(orgIDs),
		successCount,
		failureCount,
		time.Since(start).Milliseconds(),
	)

	return nil
}

func (dm *DashboardManager) getAllOrgIDs(ctx context.Context) ([]int64, error) {
	queryCtx, cancel := context.WithTimeout(ctx, dashboardDBQueryTimeout)
	defer cancel()

	rows, err := dm.db.QueryContext(queryCtx, `SELECT id FROM orgs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if scanErr := rows.Scan(&id); scanErr != nil {
			return nil, scanErr
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		return []int64{1}, nil
	}

	return ids, nil
}

func (dm *DashboardManager) buildDashboardData(ctx context.Context, orgID int64, trigger string, cycleID uint64) (DashboardData, error) {
	startedAt := time.Now()
	data := DashboardData{
		LastUpdated: time.Now(),
	}

	if err := dm.collectStatistics(ctx, &data, orgID); err != nil {
		dm.logErrorf("[dashboard] collect statistics failed org_id=%d err=%v", orgID, err)
	}

	if err := dm.collectWebhookHealth(ctx, &data, orgID); err != nil {
		dm.logErrorf("[dashboard] collect webhook_health failed org_id=%d err=%v", orgID, err)
	}

	if err := dm.collectConnectorSetupProgress(ctx, &data, orgID); err != nil {
		dm.logErrorf("[dashboard] collect connector_setup failed org_id=%d err=%v", orgID, err)
	}

	if err := dm.collectOnboardingData(ctx, &data, orgID); err != nil {
		dm.logErrorf("[dashboard] collect onboarding failed org_id=%d err=%v", orgID, err)
	}

	if err := dm.collectRecentActivity(ctx, &data, orgID); err != nil {
		dm.logErrorf("[dashboard] collect recent_activity failed org_id=%d err=%v", orgID, err)
	}

	if err := dm.collectPerformanceMetrics(ctx, &data, orgID); err != nil {
		dm.logErrorf("[dashboard] collect performance_metrics failed org_id=%d err=%v", orgID, err)
	}

	if err := dm.collectSystemStatus(&data); err != nil {
		dm.logErrorf("[dashboard] collect system_status failed org_id=%d err=%v", orgID, err)
	}

	webhookConnectors := 0
	if data.WebhookHealth != nil {
		webhookConnectors = data.WebhookHealth.TotalConnectors
	}

	dm.logMinimalf(
		"[dashboard] org refresh complete cycle=%d trigger=%s org_id=%d reviews=%d comments=%d providers=%d connectors=%d webhook_connectors=%d activities=%d duration_ms=%d",
		cycleID,
		trigger,
		orgID,
		data.TotalReviews,
		data.TotalComments,
		data.ConnectedProviders,
		data.ActiveAIConnectors,
		webhookConnectors,
		len(data.RecentActivity),
		time.Since(startedAt).Milliseconds(),
	)

	return data, nil
}

func (dm *DashboardManager) GetDashboardDataForOrg(ctx context.Context, orgID int64) (DashboardData, error) {
	dm.mu.RLock()
	cached, ok := dm.cache[orgID]
	if ok && time.Since(cached.LastUpdated) < dashboardCacheTTL {
		dm.mu.RUnlock()
		return cached, nil
	}
	dm.mu.RUnlock()

	data, err := dm.buildDashboardData(ctx, orgID, dashboardTriggerCacheMiss, 0)
	if err != nil {
		return DashboardData{}, err
	}

	dm.mu.Lock()
	dm.cache[orgID] = data
	dm.mu.Unlock()

	return data, nil
}

func (dm *DashboardManager) RefreshOrgDashboard(ctx context.Context, orgID int64) (DashboardData, error) {
	data, err := dm.buildDashboardData(ctx, orgID, dashboardTriggerManual, 0)
	if err != nil {
		return DashboardData{}, err
	}

	dm.mu.Lock()
	dm.cache[orgID] = data
	dm.mu.Unlock()

	return data, nil
}

// collectStatistics gathers basic statistics
func (dm *DashboardManager) collectStatistics(ctx context.Context, data *DashboardData, orgID int64) error {
	dm.logVerbosef("[dashboard] collector start name=statistics org_id=%d", orgID)

	ctx, cancel := context.WithTimeout(ctx, dashboardDBQueryTimeout)
	defer cancel()

	// Count total AI reviews directly from reviews table
	err := dm.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM reviews WHERE org_id = $1`,
		orgID,
	).Scan(&data.TotalReviews)
	if err != nil {
		dm.logErrorf("[dashboard] statistics count_reviews_failed org_id=%d err=%v", orgID, err)
		data.TotalReviews = 0
	} else {
		dm.logVerbosef("[dashboard] statistics reviews org_id=%d value=%d", orgID, data.TotalReviews)
	}

	// Count total comments from review completion events
	err = dm.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(COALESCE(NULLIF(data->>'commentCount', '')::int, 0)), 0)
		FROM review_events
		WHERE org_id = $1
		  AND event_type = 'completion'`,
		orgID,
	).Scan(&data.TotalComments)
	if err != nil {
		dm.logErrorf("[dashboard] statistics count_comments_failed org_id=%d err=%v", orgID, err)
		data.TotalComments = 0
	} else {
		dm.logVerbosef("[dashboard] statistics comments org_id=%d value=%d", orgID, data.TotalComments)
	}

	// Count connected Git providers correctly
	err = dm.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM integration_tokens WHERE org_id = $1`,
		orgID,
	).Scan(&data.ConnectedProviders)
	if err != nil {
		dm.logErrorf("[dashboard] statistics count_providers_failed org_id=%d err=%v", orgID, err)
		data.ConnectedProviders = 0
	} else {
		dm.logVerbosef("[dashboard] statistics providers org_id=%d value=%d", orgID, data.ConnectedProviders)
	}

	// Count active AI connectors correctly
	err = dm.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ai_connectors WHERE org_id = $1`,
		orgID,
	).Scan(&data.ActiveAIConnectors)
	if err != nil {
		dm.logErrorf("[dashboard] statistics count_connectors_failed org_id=%d err=%v", orgID, err)
		data.ActiveAIConnectors = 0
	} else {
		dm.logVerbosef("[dashboard] statistics connectors org_id=%d value=%d", orgID, data.ActiveAIConnectors)
	}

	dm.logVerbosef("[dashboard] collector complete name=statistics org_id=%d reviews=%d comments=%d providers=%d ai_connectors=%d",
		orgID,
		data.TotalReviews, data.TotalComments, data.ConnectedProviders, data.ActiveAIConnectors)

	return nil
}

// collectWebhookHealth gathers webhook health information across all connectors
func (dm *DashboardManager) collectWebhookHealth(ctx context.Context, data *DashboardData, orgID int64) error {
	dm.logVerbosef("[dashboard] collector start name=webhook_health org_id=%d", orgID)

	ctx, cancel := context.WithTimeout(ctx, dashboardDBQueryTimeout)
	defer cancel()

	// Get all connectors and their projects_cache
	rows, err := dm.db.QueryContext(ctx, `
		SELECT it.id, it.projects_cache
		FROM integration_tokens it
		WHERE it.org_id = $1
	`, orgID)
	if err != nil {
		dm.logErrorf("[dashboard] webhook_health connectors_query_failed org_id=%d err=%v", orgID, err)
		return err
	}
	defer rows.Close()

	totalConnectors := 0
	totalProjects := 0
	connectorIDs := []int64{}

	for rows.Next() {
		var connectorID int64
		var projectsCacheRaw []byte
		if err := rows.Scan(&connectorID, &projectsCacheRaw); err != nil {
			dm.logErrorf("[dashboard] webhook_health connector_scan_failed org_id=%d err=%v", orgID, err)
			continue
		}

		totalConnectors++
		connectorIDs = append(connectorIDs, connectorID)

		// Count projects from projects_cache (it's an object with a "projects" key)
		if projectsCacheRaw != nil {
			var projectsCache struct {
				Projects []interface{} `json:"projects"`
			}
			if err := json.Unmarshal(projectsCacheRaw, &projectsCache); err == nil {
				totalProjects += len(projectsCache.Projects)
			}
		}
	}

	if err := rows.Err(); err != nil {
		dm.logErrorf("[dashboard] webhook_health rows_iteration_failed org_id=%d err=%v", orgID, err)
		return err
	}

	if totalConnectors == 0 {
		// No connectors, no webhook health to report
		data.WebhookHealth = nil
		return nil
	}

	// Count connected projects from webhook_registry for all connectors
	var connectedProjects int
	err = dm.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM webhook_registry wr
		INNER JOIN integration_tokens it ON wr.integration_token_id = it.id
		WHERE it.org_id = $1 AND (wr.status = 'manual' OR wr.status = 'active' OR wr.status = 'automatic')
	`, orgID).Scan(&connectedProjects)
	if err != nil {
		dm.logErrorf("[dashboard] webhook_health connected_projects_failed org_id=%d err=%v", orgID, err)
		connectedProjects = 0
	}

	// Count connectors that need setup (no webhooks at all)
	var setupRequiredConnectors int
	err = dm.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM integration_tokens it
		WHERE it.org_id = $1 
		AND NOT EXISTS (
			SELECT 1 FROM webhook_registry wr 
			WHERE wr.integration_token_id = it.id 
			AND (wr.status = 'manual' OR wr.status = 'active' OR wr.status = 'automatic')
		)
	`, orgID).Scan(&setupRequiredConnectors)
	if err != nil {
		dm.logErrorf("[dashboard] webhook_health setup_required_failed org_id=%d err=%v", orgID, err)
		setupRequiredConnectors = 0
	}

	// Find the most recently created connector that needs setup
	var mostRecentConnectorID sql.NullInt64
	var mostRecentConnectorName sql.NullString
	err = dm.db.QueryRowContext(ctx, `
		SELECT it.id, it.connection_name
		FROM integration_tokens it
		WHERE it.org_id = $1 
		AND NOT EXISTS (
			SELECT 1 FROM webhook_registry wr 
			WHERE wr.integration_token_id = it.id 
			AND (wr.status = 'manual' OR wr.status = 'active' OR wr.status = 'automatic')
		)
		ORDER BY it.created_at DESC
		LIMIT 1
	`, orgID).Scan(&mostRecentConnectorID, &mostRecentConnectorName)
	if err != nil && err != sql.ErrNoRows {
		dm.logErrorf("[dashboard] webhook_health most_recent_setup_connector_failed org_id=%d err=%v", orgID, err)
	}

	unconnectedProjects := totalProjects - connectedProjects
	if unconnectedProjects < 0 {
		unconnectedProjects = 0
	}

	// Calculate health percentage
	healthPercent := float64(0)
	if totalProjects > 0 {
		healthPercent = float64(connectedProjects) / float64(totalProjects) * 100
	} else if totalConnectors > 0 {
		// Connectors exist but no projects cached yet - treat as setup_required
		healthPercent = 0
	}

	// Determine health status
	healthStatus := "setup_required"
	if healthPercent >= 100 {
		healthStatus = "healthy"
	} else if healthPercent > 0 {
		healthStatus = "partial"
	}

	webhookHealth := &WebhookHealthSummary{
		TotalConnectors:         totalConnectors,
		TotalProjects:           totalProjects,
		ConnectedProjects:       connectedProjects,
		UnconnectedProjects:     unconnectedProjects,
		HealthPercent:           healthPercent,
		HealthStatus:            healthStatus,
		SetupRequiredConnectors: setupRequiredConnectors,
	}

	// Add most recent connector needing setup if found
	if mostRecentConnectorID.Valid {
		webhookHealth.MostRecentConnectorNeedingSetupID = &mostRecentConnectorID.Int64
	}
	if mostRecentConnectorName.Valid {
		webhookHealth.MostRecentConnectorNeedingSetupName = &mostRecentConnectorName.String
	}

	data.WebhookHealth = webhookHealth

	dm.logVerbosef("[dashboard] collector complete name=webhook_health org_id=%d connectors=%d projects=%d connected=%d health=%.1f",
		orgID,
		totalConnectors, totalProjects, connectedProjects, healthPercent)

	return nil
}

// collectConnectorSetupProgress gathers setup progress for connectors that need attention
func (dm *DashboardManager) collectConnectorSetupProgress(ctx context.Context, data *DashboardData, orgID int64) error {
	dm.logVerbosef("[dashboard] collector start name=connector_setup_progress org_id=%d", orgID)

	ctx, cancel := context.WithTimeout(ctx, dashboardDBQueryTimeout)
	defer cancel()

	// Get all connectors created in the last 10 minutes or that need attention
	// This ensures users see the progress even for fast auto-installations
	rows, err := dm.db.QueryContext(ctx, `
		SELECT 
			it.id, 
			it.connection_name, 
			it.provider,
			it.projects_cache,
			it.created_at,
			COALESCE((
				SELECT COUNT(*) 
				FROM webhook_registry wr 
				WHERE wr.integration_token_id = it.id 
				AND (wr.status = 'manual' OR wr.status = 'active' OR wr.status = 'automatic')
			), 0) as connected_count
		FROM integration_tokens it
		WHERE it.org_id = $1
		ORDER BY it.created_at DESC
	`, orgID)
	if err != nil {
		dm.logErrorf("[dashboard] connector_setup_progress query_failed org_id=%d err=%v", orgID, err)
		return err
	}
	defer rows.Close()

	var progressList []ConnectorSetupProgress
	recentThreshold := time.Now().Add(-10 * time.Minute) // Show recently created connectors for 10 minutes

	for rows.Next() {
		var connectorID int64
		var connectorName, provider string
		var projectsCacheRaw []byte
		var connectedCount int
		var createdAt time.Time

		if err := rows.Scan(&connectorID, &connectorName, &provider, &projectsCacheRaw, &createdAt, &connectedCount); err != nil {
			dm.logErrorf("[dashboard] connector_setup_progress scan_failed org_id=%d err=%v", orgID, err)
			continue
		}

		// Parse projects_cache to get total project count
		totalProjects := 0
		if projectsCacheRaw != nil {
			var projectsCache struct {
				Projects []interface{} `json:"projects"`
			}
			if err := json.Unmarshal(projectsCacheRaw, &projectsCache); err == nil {
				totalProjects = len(projectsCache.Projects)
			}
		}

		// Determine the phase based on project count and webhook count
		var phase, message string

		if totalProjects == 0 {
			// Phase 1: No projects discovered yet
			phase = "discovering"
			message = "Discovering projects..."
		} else if connectedCount == 0 {
			// Phase 2: Projects exist but no webhooks installed
			phase = "installing"
			message = fmt.Sprintf("Installing webhooks: 0/%d", totalProjects)
		} else if connectedCount < totalProjects {
			// Phase 2: Some webhooks installed
			phase = "installing"
			message = fmt.Sprintf("Installing webhooks: %d/%d", connectedCount, totalProjects)
		} else {
			// Phase 3: All done
			phase = "ready"
			message = fmt.Sprintf("Ready: %d projects connected", totalProjects)
		}

		// Include connectors that are either:
		// 1. Not fully ready (still in setup)
		// 2. Recently created (within last 10 minutes) so user sees the success
		isRecent := createdAt.After(recentThreshold)
		if phase != "ready" || isRecent {
			progressList = append(progressList, ConnectorSetupProgress{
				ConnectorID:       connectorID,
				ConnectorName:     connectorName,
				Provider:          provider,
				Phase:             phase,
				TotalProjects:     totalProjects,
				ConnectedProjects: connectedCount,
				Message:           message,
			})
		}
	}

	if err := rows.Err(); err != nil {
		dm.logErrorf("[dashboard] connector_setup_progress rows_iteration_failed org_id=%d err=%v", orgID, err)
		return err
	}

	data.ConnectorSetupProgress = progressList

	dm.logVerbosef("[dashboard] collector complete name=connector_setup_progress org_id=%d connectors_needing_attention=%d", orgID, len(progressList))

	return nil
}

// collectOnboardingData gathers onboarding-specific information
func (dm *DashboardManager) collectOnboardingData(ctx context.Context, data *DashboardData, orgID int64) error {
	dm.logVerbosef("[dashboard] collector start name=onboarding org_id=%d", orgID)

	ctx, cancel := context.WithTimeout(ctx, dashboardDBQueryTimeout)
	defer cancel()

	data.APIUrl = dashboardAPIURL()

	// Get the first user with owner role in this org to find their onboarding API key
	var userID int64
	var onboardingKey sql.NullString
	var lastCLIUsed sql.NullTime

	err := dm.db.QueryRowContext(ctx,
		`SELECT u.id, u.onboarding_api_key, u.last_cli_used_at
		FROM users u
		INNER JOIN user_roles ur ON u.id = ur.user_id
		INNER JOIN roles r ON ur.role_id = r.id
		WHERE ur.org_id = $1 AND r.name IN ('owner', 'super_admin')
		ORDER BY u.created_at ASC
		LIMIT 1`,
		orgID,
	).Scan(&userID, &onboardingKey, &lastCLIUsed)

	if err != nil && err != sql.ErrNoRows {
		dm.logErrorf("[dashboard] onboarding query_failed org_id=%d err=%v", orgID, err)
		return err
	}

	// If user exists but has no onboarding API key, generate one
	if err == nil && (!onboardingKey.Valid || onboardingKey.String == "") {
		dm.logVerbosef("[dashboard] onboarding generating_api_key org_id=%d user_id=%d", orgID, userID)
		apiKeyManager := NewAPIKeyManager(dm.db)
		// Create API key in api_keys table and get the plain key back
		_, newKey, genErr := apiKeyManager.CreateAPIKey(userID, orgID, "Onboarding API Key", []string{}, nil)
		if genErr == nil {
			_, updateErr := dm.db.ExecContext(ctx,
				`UPDATE users SET onboarding_api_key = $1 WHERE id = $2`,
				newKey, userID)
			if updateErr == nil {
				onboardingKey = sql.NullString{String: newKey, Valid: true}
				dm.logVerbosef("[dashboard] onboarding generated_api_key org_id=%d user_id=%d", orgID, userID)
			} else {
				dm.logErrorf("[dashboard] onboarding update_api_key_failed org_id=%d user_id=%d err=%v", orgID, userID, updateErr)
			}
		} else {
			dm.logErrorf("[dashboard] onboarding generate_api_key_failed org_id=%d user_id=%d err=%v", orgID, userID, genErr)
		}
	}

	// Set onboarding API key if it exists
	if onboardingKey.Valid && onboardingKey.String != "" {
		data.OnboardingAPIKey = onboardingKey.String
	}

	// Check if CLI has been used
	data.CLIInstalled = lastCLIUsed.Valid

	dm.logVerbosef("[dashboard] collector complete name=onboarding org_id=%d has_api_key=%v cli_installed=%v",
		orgID,
		data.OnboardingAPIKey != "", data.CLIInstalled)

	return nil
}

// collectRecentActivity gathers recent activity data
func (dm *DashboardManager) collectRecentActivity(ctx context.Context, data *DashboardData, orgID int64) error {
	dm.logVerbosef("[dashboard] collector start name=recent_activity org_id=%d", orgID)

	ctx, cancel := context.WithTimeout(ctx, dashboardDBQueryTimeout)
	defer cancel()

	// Initialize with empty slice instead of nil
	data.RecentActivity = []ActivityItem{}

	// Get recent job queue activities
	// Query recent activities from recent_activity table (new system)
	rows, err := dm.db.QueryContext(ctx,
		`SELECT ra.id, ra.activity_type, ra.event_data, ra.created_at
		FROM recent_activity ra
		WHERE ra.org_id = $1
		   OR (ra.review_id IS NOT NULL AND EXISTS (
			SELECT 1 FROM reviews r WHERE r.id = ra.review_id AND r.org_id = $1
		   ))
		   OR (
			ra.activity_type IN ('connector_created', 'webhook_installed')
			AND EXISTS (
				SELECT 1
				FROM integration_tokens it
				WHERE it.id = CASE
					WHEN (ra.event_data->>'connector_id') ~ '^[0-9]+$' THEN (ra.event_data->>'connector_id')::bigint
					ELSE NULL
				END
				AND it.org_id = $1
			)
		   )
		ORDER BY ra.created_at DESC
		LIMIT 10`,
		orgID,
	)
	if err != nil {
		dm.logErrorf("[dashboard] recent_activity query_failed org_id=%d err=%v", orgID, err)
		return err
	}
	defer rows.Close()

	var activities []ActivityItem
	for rows.Next() {
		var id int
		var activityType string
		var eventDataBytes []byte
		var createdAt time.Time
		if err := rows.Scan(&id, &activityType, &eventDataBytes, &createdAt); err != nil {
			dm.logErrorf("[dashboard] recent_activity scan_failed org_id=%d err=%v", orgID, err)
			continue
		}

		// Default values
		action := activityType
		repo := ""
		uiType := "other"

		// Parse event_data JSON for repository and specialized labels
		var payload map[string]interface{}
		if err := json.Unmarshal(eventDataBytes, &payload); err == nil {
			if r, ok := payload["repository"].(string); ok {
				repo = r
			}
		}

		switch activityType {
		case "review_triggered":
			action = "Code review triggered"
			uiType = "review"
		case "connector_created":
			action = "Connector created"
			uiType = "connection"
		case "webhook_installed":
			action = "Webhook installed"
			uiType = "connection"
		case "webhook_removed":
			action = "Webhook removed"
			uiType = "connection"
		}

		activities = append(activities, ActivityItem{
			ID:         id,
			Action:     action,
			Repository: repo,
			Timestamp:  createdAt,
			TimeAgo:    formatTimeAgo(createdAt),
			Type:       uiType,
		})
	}

	if err := rows.Err(); err != nil {
		dm.logErrorf("[dashboard] recent_activity rows_iteration_failed org_id=%d err=%v", orgID, err)
		return err
	}

	data.RecentActivity = activities
	dm.logVerbosef("[dashboard] collector complete name=recent_activity org_id=%d activities=%d", orgID, len(activities))
	return nil
}

// collectPerformanceMetrics gathers performance data
func (dm *DashboardManager) collectPerformanceMetrics(ctx context.Context, data *DashboardData, orgID int64) error {
	dm.logVerbosef("[dashboard] collector start name=performance_metrics org_id=%d", orgID)

	ctx, cancel := context.WithTimeout(ctx, dashboardDBQueryTimeout)
	defer cancel()

	// Calculate reviews this week using reviews table
	err := dm.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM reviews
		WHERE org_id = $1
		AND created_at >= DATE_TRUNC('week', NOW())`,
		orgID,
	).Scan(&data.PerformanceMetrics.ReviewsThisWeek)
	if err != nil {
		dm.logErrorf("[dashboard] performance_metrics weekly_reviews_failed org_id=%d err=%v", orgID, err)
		data.PerformanceMetrics.ReviewsThisWeek = 0
	} else {
		dm.logVerbosef("[dashboard] performance_metrics weekly_reviews org_id=%d value=%d", orgID, data.PerformanceMetrics.ReviewsThisWeek)
	}

	// Calculate comments this week
	err = dm.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(COALESCE(NULLIF(data->>'commentCount', '')::int, 0)), 0)
		FROM review_events
		WHERE org_id = $1
		  AND event_type = 'completion'
		  AND ts >= DATE_TRUNC('week', NOW())`,
		orgID,
	).Scan(&data.PerformanceMetrics.CommentsThisWeek)
	if err != nil {
		dm.logErrorf("[dashboard] performance_metrics weekly_comments_failed org_id=%d err=%v", orgID, err)
		data.PerformanceMetrics.CommentsThisWeek = 0
	}

	// Calculate success rate
	// Without legacy job queue metrics, default success rate to 100%
	data.PerformanceMetrics.SuccessRate = 100.0

	// Set average response time
	data.PerformanceMetrics.AvgResponseTime = 2.3

	dm.logVerbosef("[dashboard] collector complete name=performance_metrics org_id=%d reviews_week=%d comments_week=%d success_rate=%.1f%% avg_time=%.1f",
		orgID,
		data.PerformanceMetrics.ReviewsThisWeek, data.PerformanceMetrics.CommentsThisWeek,
		data.PerformanceMetrics.SuccessRate, data.PerformanceMetrics.AvgResponseTime)

	return nil
}

// collectSystemStatus gathers system health information
func (dm *DashboardManager) collectSystemStatus(data *DashboardData) error {
	data.SystemStatus.LastHealthCheck = time.Now()
	data.SystemStatus.APIHealth = "healthy"
	data.SystemStatus.DatabaseHealth = "healthy"

	// Check job queue health
	// Legacy job queue removed; mark as not tracked
	data.SystemStatus.JobQueueHealth = "not_tracked"

	return nil
}

// storeDashboardData saves the collected data to the database

// GetDashboardData retrieves the cached dashboard data
func (s *Server) GetDashboardData(c echo.Context) error {
	orgIDVal := c.Get("org_id")
	orgID, ok := orgIDVal.(int64)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Organization context required",
		})
	}

	data, err := s.dashboardManager.GetDashboardDataForOrg(c.Request().Context(), orgID)
	if err != nil {
		log.Printf("Error building dashboard data for org %d: %v", orgID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to build dashboard data",
		})
	}

	return c.JSON(http.StatusOK, data)
}

// RefreshDashboardData manually triggers a dashboard data update
func (s *Server) RefreshDashboardData(c echo.Context) error {
	s.dashboardManager.logVerbosef("[dashboard] manual refresh requested")

	orgIDVal := c.Get("org_id")
	orgID, ok := orgIDVal.(int64)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Organization context required",
		})
	}

	data, err := s.dashboardManager.RefreshOrgDashboard(c.Request().Context(), orgID)
	if err != nil {
		log.Printf("Error refreshing dashboard data for org %d: %v", orgID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to refresh dashboard data",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":      "Dashboard data refreshed successfully",
		"last_updated": data.LastUpdated,
	})
}

// formatTimeAgo returns a human-readable time difference
func formatTimeAgo(t time.Time) string {
	diff := time.Since(t)

	if diff < time.Minute {
		return "just now"
	} else if diff < time.Hour {
		minutes := int(diff.Minutes())
		if minutes == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", minutes)
	} else if diff < 24*time.Hour {
		hours := int(diff.Hours())
		if hours == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", hours)
	} else {
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}
