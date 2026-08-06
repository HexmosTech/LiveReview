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

	"github.com/livereview/storage/dashboard"
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

// DashboardData: fields marked "currently unpopulated" are trimmed since only onboarding fields and review_layers are needed today; omitempty hides them from the JSON response.
type DashboardData struct {
	// Statistics — currently unpopulated
	TotalReviews       int `json:"total_reviews,omitempty"`
	TotalComments      int `json:"total_comments,omitempty"`
	ConnectedProviders int `json:"connected_providers,omitempty"`
	ActiveAIConnectors int `json:"active_ai_connectors,omitempty"`

	// Webhook Health — currently unpopulated
	WebhookHealth *WebhookHealthSummary `json:"webhook_health,omitempty"`

	// Connector Setup Progress — currently unpopulated
	ConnectorSetupProgress []ConnectorSetupProgress `json:"connector_setup_progress,omitempty"`

	// Onboarding — still populated
	OnboardingAPIKey string `json:"onboarding_api_key,omitempty"`
	APIUrl           string `json:"api_url"`
	CLIInstalled     bool   `json:"cli_installed,omitempty"`

	// Recent Activity — currently unpopulated
	RecentActivity []ActivityItem `json:"recent_activity,omitempty"`

	// Performance Metrics — currently unpopulated; pointer + omitempty so it's fully absent from the response, not an empty object.
	PerformanceMetrics *PerformanceMetrics `json:"performance_metrics,omitempty"`

	// System Status — currently unpopulated, same as above.
	SystemStatus *SystemStatus `json:"system_status,omitempty"`

	// Review Layers — precomputed by DashboardManager's own background tick, read from dashboard_cache (see collectReviewLayers).
	ReviewLayers json.RawMessage `json:"review_layers,omitempty"`

	// System Overview — same precomputed dashboard_cache row, sibling key to review_layers (see collectSystemOverview).
	SystemOverview json.RawMessage `json:"system_overview,omitempty"`

	// People — same precomputed dashboard_cache row, sibling key to review_layers/system_overview (see collectPeople).
	People json.RawMessage `json:"people,omitempty"`

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
	// connectorSetupRecentThreshold: a connector created within this window is still
	// considered "setting up" rather than stalled if it has no active webhook yet.
	connectorSetupRecentThreshold = 10 * time.Minute
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

	// cacheStore is the read/write layer for dashboard_cache; nil is treated as "no row yet"/"nothing to do".
	cacheStore *dashboard.CacheStore

	refreshInProgress atomic.Bool
	refreshCycleID    uint64
}

// NewDashboardManager creates a new dashboard manager
func NewDashboardManager(db *sql.DB, lockStore dashboardLeaderLockStore, cacheStore *dashboard.CacheStore) *DashboardManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &DashboardManager{
		db:         db,
		ctx:        ctx,
		cancel:     cancel,
		cache:      make(map[int64]DashboardData),
		lockStore:  lockStore,
		cacheStore: cacheStore,
		instance:   dashboardInstanceID(),
		logLevel:   parseDashboardLogLevel(os.Getenv("DASHBOARD_LOG_LEVEL")),
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

	go func() {
		if dm.canRunPeriodicAllOrgRefresh(dm.ctx, dashboardTriggerStartup) {
			if err := dm.runRefreshCycle(dm.ctx, dashboardTriggerStartup); err != nil {
				dm.logErrorf("[dashboard] initial refresh failed instance=%s err=%v", dm.instance, err)
			}
		}
	}()

	
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

	// Batched across all orgs (3 read passes total) instead of buildDashboardData's ~6 queries
	// repeated per org — the per-org loop this replaced was the source of hundreds of individual
	// round trips every tick.
	dataByOrg, err := dm.buildDashboardDataBatch(ctx, orgIDs)
	if err != nil {
		return fmt.Errorf("build dashboard data batch: %w", err)
	}

	dm.mu.Lock()
	for orgID, data := range dataByOrg {
		dm.cache[orgID] = data
	}
	dm.mu.Unlock()

	dm.logMinimalf(
		"[dashboard] refresh summary cycle=%d trigger=%s instance=%s org_total=%d org_success=%d org_failed=%d duration_ms=%d",
		cycleID,
		trigger,
		dm.instance,
		len(orgIDs),
		len(dataByOrg),
		len(orgIDs)-len(dataByOrg),
		time.Since(start).Milliseconds(),
	)

	// Same cycle, org list, and leader lock as the onboarding refresh above — no separate ticker/lock per domain.
	// Order matters: each domain's GetBatch must run after the previous domain's UpsertBatch completes, or it would revert that domain's just-written update.
	if dm.cacheStore != nil {
		if err := dm.refreshAllReviewLayers(ctx, orgIDs); err != nil {
			dm.logErrorf("[dashboard_cache] refresh failed cycle=%d trigger=%s err=%v", cycleID, trigger, err)
		}
		if err := dm.refreshAllSystemOverview(ctx, orgIDs); err != nil {
			dm.logErrorf("[dashboard_cache] system_overview refresh failed cycle=%d trigger=%s err=%v", cycleID, trigger, err)
		}
		if err := dm.refreshAllPeople(ctx, orgIDs); err != nil {
			dm.logErrorf("[dashboard_cache] people refresh failed cycle=%d trigger=%s err=%v", cycleID, trigger, err)
		}
	}

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

	// total_reviews/active_ai_connectors drive the onboarding stepper's hasRunReview/hasAIProvider checks — still read by the frontend.
	if err := dm.collectStatistics(ctx, &data, orgID); err != nil {
		dm.logErrorf("[dashboard] collect statistics failed org_id=%d err=%v", orgID, err)
	}

	if err := dm.collectOnboardingData(ctx, &data, orgID); err != nil {
		dm.logErrorf("[dashboard] collect onboarding failed org_id=%d err=%v", orgID, err)
	}

	// connector_setup_progress drives the "connectors needing setup" banner — still read by the frontend.
	if err := dm.collectConnectorSetupProgress(ctx, &data, orgID); err != nil {
		dm.logErrorf("[dashboard] collect connector_setup failed org_id=%d err=%v", orgID, err)
	}

	// review_layers is fetched fresh per request instead, directly in the GetDashboardData handler below.

	dm.logVerbosef(
		"[dashboard] org refresh complete cycle=%d trigger=%s org_id=%d reviews=%d comments=%d providers=%d connectors=%d connectors_needing_setup=%d duration_ms=%d",
		cycleID,
		trigger,
		orgID,
		data.TotalReviews,
		data.TotalComments,
		data.ConnectedProviders,
		data.ActiveAIConnectors,
		len(data.ConnectorSetupProgress),
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

// collectConnectorSetupProgress gathers setup progress for connectors that need attention
func (dm *DashboardManager) collectConnectorSetupProgress(ctx context.Context, data *DashboardData, orgID int64) error {
	dm.logVerbosef("[dashboard] collector start name=connector_setup_progress org_id=%d", orgID)

	ctx, cancel := context.WithTimeout(ctx, dashboardDBQueryTimeout)
	defer cancel()

	// Recently created or still-setting-up connectors, so fast auto-installations still show progress.
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
	recentThreshold := time.Now().Add(-connectorSetupRecentThreshold)

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

		totalProjects := 0
		if projectsCacheRaw != nil {
			var projectsCache struct {
				Projects []interface{} `json:"projects"`
			}
			if err := json.Unmarshal(projectsCacheRaw, &projectsCache); err == nil {
				totalProjects = len(projectsCache.Projects)
			}
		}

		var phase, message string
		if totalProjects == 0 {
			phase = "discovering"
			message = "Discovering projects..."
		} else if connectedCount == 0 {
			phase = "installing"
			message = fmt.Sprintf("Installing webhooks: 0/%d", totalProjects)
		} else if connectedCount < totalProjects {
			phase = "installing"
			message = fmt.Sprintf("Installing webhooks: %d/%d", connectedCount, totalProjects)
		} else {
			phase = "ready"
			message = fmt.Sprintf("Ready: %d projects connected", totalProjects)
		}

		// Include connectors still in setup, or ready ones created recently so the user sees the success.
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

// collectReviewLayers is a cheap single-row read of the precomputed dashboard_cache; a missing row just leaves the field empty, not an error.
func (dm *DashboardManager) collectReviewLayers(ctx context.Context, data *DashboardData, orgID int64) error {
	if dm.cacheStore == nil {
		return nil
	}

	queryCtx, cancel := context.WithTimeout(ctx, dashboardDBQueryTimeout)
	defer cancel()

	raw, err := dm.cacheStore.GetFinalJSON(queryCtx, orgID)
	if err != nil {
		return err
	}
	if raw == nil {
		return nil
	}

	var wrapper struct {
		ReviewLayers json.RawMessage `json:"review_layers"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return fmt.Errorf("unmarshal final_json: %w", err)
	}

	data.ReviewLayers = wrapper.ReviewLayers
	return nil
}

// collectSystemOverview is a cheap single-row read of the precomputed dashboard_cache; a missing row just leaves the field empty, not an error.
func (dm *DashboardManager) collectSystemOverview(ctx context.Context, data *DashboardData, orgID int64) error {
	if dm.cacheStore == nil {
		return nil
	}

	queryCtx, cancel := context.WithTimeout(ctx, dashboardDBQueryTimeout)
	defer cancel()

	raw, err := dm.cacheStore.GetFinalJSON(queryCtx, orgID)
	if err != nil {
		return err
	}
	if raw == nil {
		return nil
	}

	var wrapper struct {
		SystemOverview json.RawMessage `json:"system_overview"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		log.Printf("[dashboard_cache] unmarshal final_json failed org_id=%d err=%v raw=%s", orgID, err, raw)
		return fmt.Errorf("unmarshal final_json: %w", err)
	}

	data.SystemOverview = wrapper.SystemOverview
	return nil
}

// collectPeople is a cheap single-row read of the precomputed dashboard_cache; a missing row just leaves the field empty, not an error.
func (dm *DashboardManager) collectPeople(ctx context.Context, data *DashboardData, orgID int64) error {
	if dm.cacheStore == nil {
		return nil
	}

	queryCtx, cancel := context.WithTimeout(ctx, dashboardDBQueryTimeout)
	defer cancel()

	raw, err := dm.cacheStore.GetFinalJSON(queryCtx, orgID)
	if err != nil {
		return err
	}
	if raw == nil {
		return nil
	}

	var wrapper struct {
		People json.RawMessage `json:"people"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		log.Printf("[dashboard_cache] unmarshal final_json failed org_id=%d err=%v raw=%s", orgID, err, raw)
		return fmt.Errorf("unmarshal final_json: %w", err)
	}

	data.People = wrapper.People
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

	// Fetched fresh on every request, deliberately not part of the cached data above.
	if err := s.dashboardManager.collectReviewLayers(c.Request().Context(), &data, orgID); err != nil {
		log.Printf("Error collecting review_layers for org %d: %v", orgID, err)
	}
	if err := s.dashboardManager.collectSystemOverview(c.Request().Context(), &data, orgID); err != nil {
		log.Printf("Error collecting system_overview for org %d: %v", orgID, err)
	}
	if err := s.dashboardManager.collectPeople(c.Request().Context(), &data, orgID); err != nil {
		log.Printf("Error collecting people for org %d: %v", orgID, err)
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

	response := map[string]interface{}{
		"message":      "Dashboard data refreshed successfully",
		"last_updated": data.LastUpdated,
	}

	// Best-effort: a failed recompute in one domain shouldn't fail the whole refresh.
	if s.dashboardManager.cacheStore != nil {
		reviewLayers, err := s.dashboardManager.RefreshOrgReviewLayers(c.Request().Context(), orgID)
		if err != nil {
			log.Printf("Error refreshing review_layers for org %d: %v", orgID, err)
		} else {
			response["review_layers"] = reviewLayers
		}

		systemOverview, err := s.dashboardManager.RefreshOrgSystemOverview(c.Request().Context(), orgID)
		if err != nil {
			log.Printf("Error refreshing system_overview for org %d: %v", orgID, err)
		} else {
			response["system_overview"] = systemOverview
		}

		people, err := s.dashboardManager.RefreshOrgPeople(c.Request().Context(), orgID)
		if err != nil {
			log.Printf("Error refreshing people for org %d: %v", orgID, err)
		} else {
			response["people"] = people
		}
	}

	return c.JSON(http.StatusOK, response)
}

