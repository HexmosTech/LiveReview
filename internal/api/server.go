package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/api/organizations"
	"github.com/livereview/internal/api/users"
	"github.com/livereview/internal/jobqueue"
	"github.com/livereview/internal/learnings"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/license/payment"
	bitbucketprovider "github.com/livereview/internal/provider_input/bitbucket"
	githubprovider "github.com/livereview/internal/provider_input/github"
	gitlabprovider "github.com/livereview/internal/provider_input/gitlab"
	bitbucketoutput "github.com/livereview/internal/provider_output/bitbucket"
	githuboutput "github.com/livereview/internal/provider_output/github"
	gitlaboutput "github.com/livereview/internal/provider_output/gitlab"
	// Import FetchGitLabProfile
)

// VersionInfo holds version information for the API
type VersionInfo struct {
	Version   string
	GitCommit string
	BuildTime string
	Dirty     bool
}

// DeploymentConfig holds deployment mode configuration
type DeploymentConfig struct {
	BackendPort     int
	FrontendPort    int
	ReverseProxy    bool
	IsCloud         bool   // cloud vs self-hosted deployment
	Mode            string // derived: "demo" or "production"
	WebhooksEnabled bool   // derived: based on mode
}

// getEnvInt retrieves an integer environment variable with a default value
func getEnvInt(key string, defaultValue int) int {
	if valueStr := os.Getenv(key); valueStr != "" {
		if value, err := strconv.Atoi(valueStr); err == nil {
			return value
		}
	}
	return defaultValue
}

// getEnvBool retrieves a boolean environment variable with a default value
func getEnvBool(key string, defaultValue bool) bool {
	valueStr := os.Getenv(key)
	fmt.Printf("Environment Variable '%s': %s\n", key, valueStr)
	if valueStr == "" {
		return defaultValue
	}
	valueStr = strings.ToLower(valueStr)
	valueStr = strings.TrimSpace(valueStr)
	return valueStr == "true" || valueStr == "1"
}

// isCloudMode checks if LiveReview is running in cloud mode
func isCloudMode() bool {
	return getEnvBool("LIVEREVIEW_IS_CLOUD", false)
}

// getDeploymentConfig reads deployment configuration from environment variables
func getDeploymentConfig() *DeploymentConfig {
	config := &DeploymentConfig{
		BackendPort:  getEnvInt("LIVEREVIEW_BACKEND_PORT", 8888),
		FrontendPort: getEnvInt("LIVEREVIEW_FRONTEND_PORT", 8081),
		ReverseProxy: getEnvBool("LIVEREVIEW_REVERSE_PROXY", false),
		IsCloud:      getEnvBool("LIVEREVIEW_IS_CLOUD", false),
	}

	// Auto-configure derived values
	if config.ReverseProxy {
		config.Mode = "production"
		config.WebhooksEnabled = true
	} else {
		config.Mode = "demo"
		config.WebhooksEnabled = false
	}

	return config
}

// Server represents the API server
type Server struct {
	echo                 *echo.Echo
	port                 int
	db                   *sql.DB
	jobQueue             *jobqueue.JobQueue
	dashboardManager     *DashboardManager
	autoWebhookInstaller *AutoWebhookInstaller
	versionInfo          *VersionInfo
	deploymentConfig     *DeploymentConfig
	authHandlers         *auth.AuthHandlers
	tokenService         *auth.TokenService
	userHandlers         *users.UserHandlers
	userService          *users.UserService
	profileHandlers      *users.ProfileHandlers
	orgHandlers          *organizations.OrganizationHandlers
	orgService           *organizations.OrganizationService
	testHandlers         *TestHandlers
	devMode              bool
	_licenseSvc          interface{} // holds *license.Service lazily (typed in license.go)
	licenseScheduler     *license.Scheduler

	// V2 Webhook Providers
	gitlabProviderV2    *gitlabprovider.GitLabV2Provider
	githubProviderV2    *githubprovider.GitHubV2Provider
	bitbucketProviderV2 *bitbucketprovider.BitbucketV2Provider

	gitlabAuthService *gitlabprovider.AuthService

	// V2 Webhook Registry
	webhookRegistryV2 *WebhookProviderRegistry

	// V2 Webhook Orchestrator
	webhookOrchestratorV2 *WebhookOrchestratorV2

	learningsService *learnings.Service
}

// NewServer creates a new API server
func NewServer(port int, versionInfo *VersionInfo) (*Server, error) {
	// Load environment variables from .env file
	env, err := loadEnvFile(".env")
	if err != nil {
		return nil, fmt.Errorf("error loading .env file: %v\n\nPlease create a .env file with DATABASE_URL like:\nDATABASE_URL=postgres://username:password@localhost:5432/dbname?sslmode=disable", err)
	}

	// print env variables
	fmt.Printf("Environment Variables: %+v\n", env)

	// Get deployment configuration from environment variables
	deploymentConfig := getDeploymentConfig()
	// print all the attributes of deploymentConfig
	fmt.Printf("Deployment Config: %+v\n", deploymentConfig)
	// print the present/active directory
	cwd, _ := os.Getwd()
	fmt.Printf("Current Working Directory: %s\n", cwd)

	// Override port from deployment config if provided
	if deploymentConfig.BackendPort != 8888 {
		port = deploymentConfig.BackendPort
	}

	// Get database URL
	dbURL, ok := env["DATABASE_URL"]
	if !ok || dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL not found in .env file\n\nPlease add DATABASE_URL to your .env file:\nDATABASE_URL=postgres://username:password@localhost:5432/dbname?sslmode=disable")
	}

	// Get JWT secret key (required for new auth system)
	jwtSecret, ok := env["JWT_SECRET"]
	if !ok || jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET not found in .env file\n\nPlease add JWT_SECRET to your .env file:\nJWT_SECRET=your-secure-random-secret-key")
	}

	// Check if development mode is enabled (for test endpoints and debug features)
	devMode := false
	if devModeStr, exists := env["DEV_MODE"]; exists {
		devMode = devModeStr == "true" || devModeStr == "1"
	}

	// Validate database connection
	err = validateDatabaseConnection(dbURL)
	if err != nil {
		return nil, err
	}

	// Open database connection
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %v", err)
	}

	// Initialize job queue
	jq, err := jobqueue.NewJobQueue(dbURL, db)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize job queue: %v", err)
	}

	// Initialize dashboard manager
	dashboardManager := NewDashboardManager(db)

	// Initialize auto webhook installer
	autoWebhookInstaller := NewAutoWebhookInstaller(db, nil, jq) // server will be set later

	// Initialize authentication system
	tokenService := auth.NewTokenService(db, jwtSecret)
	authHandlers := auth.NewAuthHandlers(tokenService, db)

	// Initialize user management system
	userService := users.NewUserService(db)
	userHandlers := users.NewUserHandlers(userService, db)

	// Initialize profile management system
	profileService := users.NewProfileService(db)
	profileHandlers := users.NewProfileHandlers(profileService)

	// Initialize organization management system
	logger := log.New(os.Stdout, "[ORG] ", log.LstdFlags)
	orgService := organizations.NewOrganizationService(db, logger)
	orgHandlers := organizations.NewOrganizationHandlers(orgService, logger)

	// Initialize test handlers for manual testing
	testHandlers := NewTestHandlers()

	// Start token cleanup scheduler
	tokenService.StartCleanupScheduler()

	e := echo.New()

	// Middleware
	// e.Use(middleware.Logger()) // Disabled to reduce log noise
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Add database to context
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("db", db)
			return next(c)
		}
	})

	triggerAutoInstall := func(integrationID int) {
		if autoWebhookInstaller != nil {
			autoWebhookInstaller.TriggerAutoInstallation(integrationID)
		}
	}

	learningsSvc := learnings.NewService(learnings.NewPostgresStore(db))

	server := &Server{
		echo:                 e,
		port:                 port,
		db:                   db,
		jobQueue:             jq,
		dashboardManager:     dashboardManager,
		autoWebhookInstaller: autoWebhookInstaller,
		versionInfo:          versionInfo,
		deploymentConfig:     deploymentConfig,
		authHandlers:         authHandlers,
		tokenService:         tokenService,
		userHandlers:         userHandlers,
		userService:          userService,
		profileHandlers:      profileHandlers,
		orgHandlers:          orgHandlers,
		orgService:           orgService,
		testHandlers:         testHandlers,
		devMode:              devMode,
		gitlabAuthService:    gitlabprovider.NewAuthService(db, triggerAutoInstall),
		learningsService:     learningsSvc,
	}

	// Initialize V2 webhook providers
	server.gitlabProviderV2 = gitlabprovider.NewGitLabV2Provider(db, gitlaboutput.NewAPIClient())
	server.githubProviderV2 = githubprovider.NewGitHubV2Provider(db, githuboutput.NewAPIClient())
	server.bitbucketProviderV2 = bitbucketprovider.NewBitbucketV2Provider(db, bitbucketoutput.NewAPIClient())

	// Initialize V2 webhook registry
	server.webhookRegistryV2 = NewWebhookProviderRegistry(server)

	// Initialize V2 webhook orchestrator
	server.webhookOrchestratorV2 = NewWebhookOrchestratorV2(server)

	// Set the server reference in auto webhook installer (circular dependency)
	autoWebhookInstaller.server = server

	if err := BackfillRecentActivityOrgIDs(db); err != nil {
		log.Printf("Failed to backfill recent activity org IDs: %v", err)
	}

	// Validate configuration before starting server
	if err := server.validateConfiguration(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// Setup routes
	server.setupRoutes()

	return server, nil
}

// validateConfiguration validates startup configuration and logs deployment mode
func (s *Server) validateConfiguration() error {
	log.Printf("[Config Validation] LIVEREVIEW_IS_CLOUD: %v", s.deploymentConfig.IsCloud)
	log.Printf("[Config Validation] LIVEREVIEW_REVERSE_PROXY: %v", s.deploymentConfig.ReverseProxy)
	log.Printf("[Config Validation] Deployment Mode: %s", s.deploymentConfig.Mode)

	if s.deploymentConfig.IsCloud {
		log.Printf("[Cloud Mode] Subscription enforcement: ENABLED")
		log.Printf("[Cloud Mode] License file validation: DISABLED")

		// Verify required cloud secrets
		if os.Getenv("CLOUD_JWT_SECRET") == "" {
			return fmt.Errorf("CLOUD_JWT_SECRET required in cloud mode")
		}
		log.Printf("[Cloud Mode] CLOUD_JWT_SECRET: configured ✓")
	} else {
		log.Printf("[Self-Hosted Mode] Subscription enforcement: DISABLED")
		log.Printf("[Self-Hosted Mode] License file validation: ENABLED")

		// Note: License validator accessibility check can be added here if needed
		// For now, we'll let it fail gracefully at runtime if unavailable
	}

	return nil
}

// DB exposes the underlying database handle (primarily for tests)
func (s *Server) DB() *sql.DB {
	return s.db
}

// BitbucketProviderV2 returns the initialized Bitbucket V2 provider
func (s *Server) BitbucketProviderV2() *bitbucketprovider.BitbucketV2Provider {
	return s.bitbucketProviderV2
}

// setupRoutes configures all API endpoints
func (s *Server) setupRoutes() {
	// Health check endpoint
	s.echo.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"status": "healthy",
		})
	})

	// Version endpoint
	s.echo.GET("/api/version", s.getVersion)

	// API v1 group
	v1 := s.echo.Group("/api/v1")

	// Public routes (no authentication required)
	public := v1.Group("")

	// Authentication endpoints (public)
	public.POST("/auth/login", s.authHandlers.Login)
	public.POST("/auth/refresh", s.authHandlers.RefreshToken)
	public.GET("/auth/setup-status", s.authHandlers.CheckSetupStatus)
	public.POST("/auth/setup", s.authHandlers.SetupAdmin)

	// System info endpoints (public)
	public.GET("/system/info", s.getSystemInfo)

	// Cloud user ensure endpoint (now public; handler performs CLOUD_JWT_SECRET validation)
	public.POST("/auth/ensure-cloud-user", s.authHandlers.EnsureCloudUser)

	// Protected routes (require authentication)
	protected := v1.Group("")
	protected.Use(auth.RequireAuth(s.tokenService, s.db))

	// Apply subscription enforcement middleware (cloud mode only)
	authMiddleware := auth.NewAuthMiddleware(s.tokenService, s.db)
	protected.Use(authMiddleware.EnforceSubscriptionLimits())

	// User management endpoints
	protected.GET("/auth/me", s.authHandlers.Me)
	protected.POST("/auth/logout", s.authHandlers.Logout)
	protected.POST("/auth/change-password", s.authHandlers.ChangePassword)

	// Self-service profile endpoints
	protected.GET("/users/profile", s.profileHandlers.GetProfile)
	protected.PUT("/users/profile", s.profileHandlers.UpdateProfile)
	protected.PUT("/users/password", s.profileHandlers.ChangePassword)

	// Development mode test endpoints (only enabled when DEV_MODE=true)
	if s.devMode {
		// TEST ENDPOINTS FOR MANUAL MIDDLEWARE TESTING
		// Public test endpoint (no auth required)
		public.GET("/test/public", s.testHandlers.PublicTest)

		// Protected test endpoints (require auth)
		protected.GET("/test/protected", s.testHandlers.ProtectedTest)
		protected.GET("/test/token-info", s.testHandlers.TokenInfoTest)
	}

	// Organization-scoped user management routes
	// Note: authMiddleware already created above

	// Org routes group - requires org context and permissions
	orgGroup := v1.Group("/orgs/:org_id")
	orgGroup.Use(authMiddleware.RequireAuth())
	orgGroup.Use(authMiddleware.BuildOrgContext())
	orgGroup.Use(authMiddleware.ValidateOrgAccess())
	orgGroup.Use(authMiddleware.BuildPermissionContext())

	// User management in organization
	orgGroup.GET("/users", s.orgHandlers.GetOrganizationMembers)
	orgGroup.POST("/users", s.userHandlers.CreateUser)
	orgGroup.GET("/users/:user_id", s.userHandlers.GetUser)
	orgGroup.PUT("/users/:user_id", s.userHandlers.UpdateUser)
	orgGroup.DELETE("/users/:user_id", s.userHandlers.DeactivateUser)
	orgGroup.PUT("/users/:user_id/role", s.userHandlers.ChangeUserRole)
	orgGroup.POST("/users/:user_id/force-password-reset", s.userHandlers.ForcePasswordReset)
	orgGroup.GET("/users/:user_id/audit-log", s.userHandlers.GetUserAuditLog)

	// Super admin routes - Production
	adminGroup := v1.Group("/admin")
	adminGroup.Use(authMiddleware.RequireAuth())
	adminGroup.Use(authMiddleware.RequireSuperAdmin())

	// Super admin user management endpoints
	adminGroup.GET("/users", s.userHandlers.ListAllUsers)
	adminGroup.POST("/orgs/:org_id/users", s.userHandlers.CreateUserInAnyOrg)
	adminGroup.PUT("/users/:user_id/org", s.userHandlers.TransferUserToOrg)
	adminGroup.GET("/analytics/users", s.userHandlers.GetUserAnalytics)

	// Organization management endpoints
	// User organization access (get their orgs) - needs permission context to detect super admin
	protectedOrgsGroup := protected.Group("")
	protectedOrgsGroup.Use(authMiddleware.BuildGlobalPermissionContext())
	protectedOrgsGroup.GET("/organizations", s.orgHandlers.GetUserOrganizations)
	protected.GET("/organizations/:org_id", s.orgHandlers.GetOrganization)

	// Organization management within org context
	orgGroup.GET("/members", s.orgHandlers.GetOrganizationMembers)
	orgGroup.PUT("/members/:user_id/role", s.orgHandlers.ChangeUserRole)
	orgGroup.GET("/analytics", s.orgHandlers.GetOrganizationAnalytics)
	orgGroup.PUT("", s.orgHandlers.UpdateOrganization) // Update org details (owners only)

	// Organization creation - available to all authenticated users
	protectedOrgsGroup.POST("/organizations", s.orgHandlers.CreateOrganization)

	// Super admin organization management
	adminGroup.DELETE("/organizations/:org_id", s.orgHandlers.DeactivateOrganization)

	// Learnings endpoints (organization-scoped, MVP)
	learningsHandler := NewLearningsHandler(s.db)
	learningsGroup := v1.Group("/learnings")
	learningsGroup.Use(authMiddleware.RequireAuth())
	learningsGroup.Use(authMiddleware.BuildOrgContextFromHeader())
	learningsGroup.Use(authMiddleware.ValidateOrgAccess())
	learningsGroup.Use(authMiddleware.BuildPermissionContext())
	learningsGroup.GET("", learningsHandler.List)
	learningsGroup.GET("/:id", learningsHandler.Get)
	learningsGroup.POST("", learningsHandler.Upsert)
	learningsGroup.PUT("/:id", learningsHandler.Update)
	learningsGroup.DELETE("/:id", learningsHandler.Delete)
	learningsGroup.POST("/apply-action-from-reply", learningsHandler.ApplyActionFromReply)

	// Development mode: Org-scoped and Admin test endpoints
	if s.devMode {
		// TEST: Org-scoped test endpoint
		orgGroup.GET("/test", s.testHandlers.OrgScopedTest)

		// Super admin routes - TEST
		adminGroup := v1.Group("/admin")
		adminGroup.Use(authMiddleware.RequireAuth())
		adminGroup.Use(authMiddleware.RequireSuperAdmin())

		// TEST: Super admin test endpoint
		adminGroup.GET("/test", s.testHandlers.SuperAdminTest)
	}

	// Reviews endpoints (organization-scoped)
	// These endpoints are moved to the reviewsGroup below for proper org scoping

	// Legacy password management endpoints (DEPRECATED - will be removed)
	// These are kept temporarily for backward compatibility during transition
	public.POST("/password", s.SetAdminPassword)               // DEPRECATED
	public.PUT("/password", s.ResetAdminPassword)              // DEPRECATED
	public.POST("/password/verify", s.VerifyAdminPassword)     // DEPRECATED
	public.GET("/password/status", s.CheckAdminPasswordStatus) // DEPRECATED

	// Production URL endpoints
	v1.GET("/production-url", s.GetProductionURL)
	v1.PUT("/production-url", s.UpdateProductionURL)

	// GitLab OAuth endpoints
	v1.POST("/gitlab/token", s.GitLabHandleCodeExchange)
	v1.POST("/gitlab/refresh", s.GitLabRefreshToken)

	// Connector endpoints (organization-scoped via headers)
	connectorGroup := v1.Group("/connectors")
	connectorGroup.Use(authMiddleware.RequireAuth())
	connectorGroup.Use(authMiddleware.BuildOrgContextFromHeader())
	connectorGroup.Use(authMiddleware.ValidateOrgAccess())
	connectorGroup.Use(authMiddleware.BuildPermissionContext())

	connectorGroup.GET("", s.GetConnectors)
	connectorGroup.GET("/:id", s.GetConnector)
	connectorGroup.DELETE("/:id", s.DeleteConnector)
	connectorGroup.GET("/:connectorId/repository-access", s.GetRepositoryAccess)
	connectorGroup.POST("/:connectorId/enable-manual-trigger", s.EnableManualTriggerForAllProjects)
	connectorGroup.POST("/:connectorId/disable-manual-trigger", s.DisableManualTriggerForAllProjects)
	connectorGroup.POST("/trigger-review", s.TriggerReviewV2)

	// GitLab profile validation endpoint
	v1.POST("/gitlab/validate-profile", s.ValidateGitLabProfile)

	// GitHub profile validation endpoint
	v1.POST("/github/validate-profile", s.ValidateGitHubProfile)

	// Bitbucket profile validation endpoint
	v1.POST("/bitbucket/validate-profile", s.ValidateBitbucketProfile)

	// Organization-scoped PAT creation (uses X-Org-Context header for organization context)
	patGroup := v1.Group("/integration_tokens")
	patGroup.Use(authMiddleware.RequireAuth())
	patGroup.Use(authMiddleware.BuildOrgContextFromHeader())
	patGroup.Use(authMiddleware.ValidateOrgAccess())
	patGroup.Use(authMiddleware.BuildPermissionContext())
	patGroup.POST("/pat", s.HandleCreatePATIntegrationToken)

	// Prompts management endpoints (Phase 7)
	promptsGroup := v1.Group("/prompts")
	promptsGroup.Use(authMiddleware.RequireAuth())
	promptsGroup.Use(authMiddleware.BuildOrgContextFromHeader())
	promptsGroup.Use(authMiddleware.ValidateOrgAccess())
	promptsGroup.Use(authMiddleware.BuildPermissionContext())
	// Catalog and variable listing available to org members
	promptsGroup.GET("/catalog", s.GetPromptsCatalog)
	promptsGroup.GET("/:key/render", s.RenderPromptPreview)
	promptsGroup.GET("/:key/variables", s.GetPromptVariables)
	// Mutations
	promptsGroup.POST("/:key/variables/:var/chunks", s.CreatePromptChunk)
	promptsGroup.POST("/:key/variables/:var/reorder", s.ReorderPromptChunks)

	// Webhook routes with connector_id for org context derivation
	// New connector-scoped webhook URLs: /api/v1/{provider}-hook/{connector_id}
	// The BuildOrgContextFromConnector middleware extracts connector_id from URL,
	// queries integration_tokens for org_id, and sets both in context
	webhookMiddleware := authMiddleware.BuildOrgContextFromConnector()

	// GitLab webhook handler (V2 Orchestrator)
	v1.POST("/gitlab-hook/:connector_id", s.WebhookOrchestratorV2Handler, webhookMiddleware)

	// GitLab comment webhook handler (V2 Orchestrator)
	v1.POST("/webhooks/gitlab/comments/:connector_id", s.WebhookOrchestratorV2Handler, webhookMiddleware)

	// GitHub webhook handler (V2 Orchestrator)
	v1.POST("/github-hook/:connector_id", s.WebhookOrchestratorV2Handler, webhookMiddleware)

	// Bitbucket webhook handler (V2 Orchestrator)
	v1.POST("/bitbucket-hook/:connector_id", s.WebhookOrchestratorV2Handler, webhookMiddleware)

	// Generic webhook handler (V2 Orchestrator)
	v1.POST("/webhook/:connector_id", s.WebhookOrchestratorV2Handler, webhookMiddleware)

	// Legacy V2 endpoint removed - old webhooks without connector_id will return 404
	// Users must re-enable manual trigger from connector settings to update webhook URLs

	// AI Connector endpoints (organization scoped)
	aiConnectorGroup := v1.Group("/aiconnectors")
	aiConnectorGroup.Use(authMiddleware.RequireAuth())
	aiConnectorGroup.Use(authMiddleware.BuildOrgContextFromHeader())
	aiConnectorGroup.Use(authMiddleware.ValidateOrgAccess())
	aiConnectorGroup.Use(authMiddleware.BuildPermissionContext())

	aiConnectorGroup.POST("/validate-key", s.ValidateAIConnectorKey)
	aiConnectorGroup.POST("", s.CreateAIConnector)
	aiConnectorGroup.GET("", s.GetAIConnectors)
	aiConnectorGroup.PUT("/:id", s.UpdateAIConnector)
	aiConnectorGroup.PUT("/reorder", s.ReorderAIConnectors)
	aiConnectorGroup.DELETE("/:id", s.DeleteAIConnector)
	aiConnectorGroup.POST("/ollama/models", s.FetchOllamaModels)

	// Dashboard endpoints (organization scoped)
	dashboardGroup := v1.Group("/dashboard")
	dashboardGroup.Use(authMiddleware.RequireAuth())
	dashboardGroup.Use(authMiddleware.BuildOrgContextFromHeader())
	dashboardGroup.Use(authMiddleware.ValidateOrgAccess())
	dashboardGroup.Use(authMiddleware.BuildPermissionContext())
	dashboardGroup.GET("", s.GetDashboardData)
	dashboardGroup.POST("/refresh", s.RefreshDashboardData)

	// Activity endpoints (organization scoped)
	activityGroup := v1.Group("/activities")
	activityGroup.Use(authMiddleware.RequireAuth())
	activityGroup.Use(authMiddleware.BuildOrgContextFromHeader())
	activityGroup.Use(authMiddleware.ValidateOrgAccess())
	activityGroup.Use(authMiddleware.BuildPermissionContext())
	activityGroup.GET("", s.GetRecentActivities)

	// License endpoints (Phase 3)
	s.attachLicenseRoutes(v1)

	// Review events endpoints (Phase 3) - Review Progress UI
	reviewsGroup := v1.Group("/reviews")
	reviewsGroup.Use(authMiddleware.RequireAuth())
	reviewsGroup.Use(authMiddleware.BuildOrgContextFromHeader())
	reviewsGroup.Use(authMiddleware.ValidateOrgAccess())
	reviewsGroup.Use(authMiddleware.BuildPermissionContext())

	// Main reviews endpoints (with org scoping)
	reviewsGroup.GET("", s.getReviews)
	reviewsGroup.POST("", s.createReview)
	reviewsGroup.GET("/:id", s.getReviewByID)

	// Initialize review events handler
	reviewEventsHandler := NewReviewEventsHandler(s.db)

	// Review events endpoints
	reviewsGroup.GET("/:id/events", reviewEventsHandler.GetReviewEvents)
	reviewsGroup.GET("/:id/events/:type", reviewEventsHandler.GetReviewEventsByType)
	reviewsGroup.GET("/:id/summary", reviewEventsHandler.GetReviewSummary)

	// Subscription endpoints (organization scoped)
	subscriptionsHandler := NewSubscriptionsHandler(s.db)
	subscriptionsGroup := v1.Group("/subscriptions")
	subscriptionsGroup.Use(authMiddleware.RequireAuth())
	subscriptionsGroup.Use(authMiddleware.BuildOrgContextFromHeader())
	subscriptionsGroup.Use(authMiddleware.ValidateOrgAccess())
	subscriptionsGroup.Use(authMiddleware.BuildPermissionContext())

	subscriptionsGroup.POST("", subscriptionsHandler.CreateSubscription)
	subscriptionsGroup.GET("/:id", subscriptionsHandler.GetSubscription)
	subscriptionsGroup.PATCH("/:id/quantity", subscriptionsHandler.UpdateQuantity)
	subscriptionsGroup.POST("/:id/cancel", subscriptionsHandler.CancelSubscription)
	subscriptionsGroup.POST("/:id/assign", subscriptionsHandler.AssignLicense)
	subscriptionsGroup.DELETE("/:id/users/:user_id", subscriptionsHandler.RevokeLicense)

	// List subscriptions - user can see their own subscriptions across all orgs
	protected.GET("/subscriptions", subscriptionsHandler.ListUserSubscriptions)

	// Razorpay webhook endpoint (public - signature verified in handler)
	webhookHandler := payment.NewRazorpayWebhookHandler(s.db, os.Getenv("RAZORPAY_WEBHOOK_SECRET"))
	v1.POST("/webhooks/razorpay", webhookHandler.HandleWebhook)
}

// Handler for creating PAT integration token, delegates to pat_token.go
func (s *Server) HandleCreatePATIntegrationToken(c echo.Context) error {
	// Create the PAT connector and get the ID
	connectorID, orgID, err := CreatePATIntegrationToken(s.db, c)
	if err != nil {
		// Handle specific error types
		if strings.Contains(err.Error(), "invalid request body") {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}
		if strings.Contains(err.Error(), "invalid metadata format") {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid metadata format"})
		}
		// Default to internal server error
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Send success response immediately (non-blocking)
	response := c.JSON(http.StatusOK, map[string]interface{}{"id": connectorID})

	// Track connector creation activity in background
	go func() {
		localOrgID := orgID
		var provider, providerURL string
		query := `SELECT provider, provider_url FROM integration_tokens WHERE id = $1`
		err := s.db.QueryRow(query, connectorID).Scan(&provider, &providerURL)
		if err == nil {
			TrackConnectorCreated(s.db, localOrgID, provider, providerURL, int(connectorID), 0) // repository count will be updated later
		}
	}()

	// Trigger automatic webhook installation in background (non-blocking)
	if s.autoWebhookInstaller != nil {
		s.autoWebhookInstaller.TriggerAutoInstallation(int(connectorID))
	}

	return response
}

// ValidateGitLabProfile validates GitLab PAT and base URL by fetching user profile
func (s *Server) ValidateGitLabProfile(c echo.Context) error {
	fmt.Println("Reached ValidateGitlabProfile")
	type reqBody struct {
		BaseURL string `json:"base_url"`
		PAT     string `json:"pat"`
	}
	var body reqBody
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if body.BaseURL == "" || body.PAT == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "base_url and pat are required"})
	}
	profile, err := gitlabprovider.FetchGitLabProfile(body.BaseURL, body.PAT)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, profile)
}

// ValidateGitHubProfile validates GitHub PAT by fetching user profile
func (s *Server) ValidateGitHubProfile(c echo.Context) error {
	type reqBody struct {
		PAT string `json:"pat"`
	}
	var body reqBody
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if body.PAT == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "pat is required"})
	}
	profile, err := githubprovider.FetchGitHubProfile(body.PAT)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, profile)
}

// ValidateBitbucketProfile validates Bitbucket API Token by fetching user profile
func (s *Server) ValidateBitbucketProfile(c echo.Context) error {
	type reqBody struct {
		Email    string `json:"email"`
		ApiToken string `json:"api_token"`
	}
	var body reqBody
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if body.Email == "" || body.ApiToken == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "email and api_token are required"})
	}

	// Validate credentials and fetch the profile in one call
	profile, err := FetchBitbucketProfile(body.Email, body.ApiToken)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, profile)
}

// Start begins the API server
func (s *Server) Start() error {
	// Determine bind address based on deployment mode
	// Always listen on 0.0.0.0 to be accessible inside containers and hosts
	bindAddress := fmt.Sprintf("0.0.0.0:%d", s.port)

	// Print server starting message with deployment mode info
	fmt.Printf("API server starting in %s mode\n", s.deploymentConfig.Mode)
	fmt.Printf("API server is running at http://localhost:%d (bound to 0.0.0.0)\n", s.port)
	if s.deploymentConfig.Mode == "demo" {
		fmt.Println("Demo Mode: Webhooks disabled, localhost access only")
	} else {
		fmt.Println("Production Mode: Webhooks enabled, configured for reverse proxy")
	}
	fmt.Println("Press Ctrl+C to stop the server")

	// Start job queue workers
	ctx := context.Background()
	go func() {
		if err := s.jobQueue.Start(ctx); err != nil {
			fmt.Printf("Error starting job queue: %v\n", err)
		}
	}()
	fmt.Println("Job queue workers started")

	// Start dashboard manager
	s.dashboardManager.Start()
	fmt.Println("Dashboard manager started")

	// Start server in a goroutine
	go func() {
		if err := s.echo.Start(bindAddress); err != nil && err != http.ErrServerClosed {
			s.echo.Logger.Errorf("Error starting server: %v", err)
		}
	}()

	// License scheduler start
	if s.licenseScheduler == nil {
		licSvc := s.licenseService() // returns *license.Service already
		if licSvc != nil {
			s.licenseScheduler = license.NewScheduler(licSvc)
			s.licenseScheduler.Start()
		}
	}

	// Wait for interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	fmt.Println("\nShutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Stop job queue workers
	if s.jobQueue != nil {
		if err := s.jobQueue.Stop(ctx); err != nil {
			fmt.Printf("Error stopping job queue: %v\n", err)
		} else {
			fmt.Println("Job queue workers stopped")
		}
	}

	// Stop dashboard manager
	if s.dashboardManager != nil {
		s.dashboardManager.Stop()
		fmt.Println("Dashboard manager stopped")
	}

	// Close database connection
	if s.db != nil {
		s.db.Close()
		fmt.Println("Database connection closed")
	}

	if s.licenseScheduler != nil {
		s.licenseScheduler.Stop()
		fmt.Println("License scheduler stopped")
	}

	return s.echo.Shutdown(ctx)
}

// Review data structures for API responses
type ReviewResponse struct {
	ID             int64                  `json:"id"`
	Repository     string                 `json:"repository"`
	Branch         *string                `json:"branch,omitempty"`
	CommitHash     *string                `json:"commitHash,omitempty"`
	PrMrUrl        *string                `json:"prMrUrl,omitempty"`
	ConnectorID    *int64                 `json:"connectorId,omitempty"`
	Status         string                 `json:"status"`
	TriggerType    string                 `json:"triggerType"`
	UserEmail      *string                `json:"userEmail,omitempty"`
	Provider       *string                `json:"provider,omitempty"`
	MRTitle        *string                `json:"mrTitle,omitempty"`
	AuthorName     *string                `json:"authorName,omitempty"`
	AuthorUsername *string                `json:"authorUsername,omitempty"`
	CreatedAt      time.Time              `json:"createdAt"`
	StartedAt      *time.Time             `json:"startedAt,omitempty"`
	CompletedAt    *time.Time             `json:"completedAt,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	OrgID          int64                  `json:"orgId"`
}

type ReviewsListResponse struct {
	Reviews     []ReviewResponse `json:"reviews"`
	Total       int              `json:"total"`
	Page        int              `json:"page"`
	PerPage     int              `json:"perPage"`
	TotalPages  int              `json:"totalPages"`
	HasNext     bool             `json:"hasNext"`
	HasPrevious bool             `json:"hasPrevious"`
}

// getReviews handles GET /api/v1/reviews with filtering and pagination
func (s *Server) getReviews(c echo.Context) error {
	// Extract org context from middleware
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}

	// Parse query parameters
	page := 1
	if pageStr := c.QueryParam("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	perPage := 20
	if perPageStr := c.QueryParam("per_page"); perPageStr != "" {
		if pp, err := strconv.Atoi(perPageStr); err == nil && pp > 0 && pp <= 100 {
			perPage = pp
		}
	}

	// Build base query
	baseQuery := `
		SELECT id, repository, branch, commit_hash, pr_mr_url, connector_id,
		       status, trigger_type, user_email, provider, mr_title, author_name, author_username,
		       created_at, started_at, completed_at, metadata, org_id
		FROM public.reviews 
		WHERE org_id = $1
	`
	countQuery := `SELECT COUNT(*) FROM public.reviews WHERE org_id = $1`

	args := []interface{}{orgID}
	argIndex := 2

	// Add filters
	status := c.QueryParam("status")
	if status != "" {
		baseQuery += fmt.Sprintf(" AND status = $%d", argIndex)
		countQuery += fmt.Sprintf(" AND status = $%d", argIndex)
		args = append(args, status)
		argIndex++
	}

	provider := c.QueryParam("provider")
	if provider != "" {
		baseQuery += fmt.Sprintf(" AND provider = $%d", argIndex)
		countQuery += fmt.Sprintf(" AND provider = $%d", argIndex)
		args = append(args, provider)
		argIndex++
	}

	// Add search functionality
	search := c.QueryParam("search")
	if search != "" {
		searchPattern := "%" + search + "%"
		baseQuery += fmt.Sprintf(" AND (repository ILIKE $%d OR pr_mr_url ILIKE $%d OR mr_title ILIKE $%d OR author_name ILIKE $%d OR author_username ILIKE $%d)", argIndex, argIndex, argIndex, argIndex, argIndex)
		countQuery += fmt.Sprintf(" AND (repository ILIKE $%d OR pr_mr_url ILIKE $%d OR mr_title ILIKE $%d OR author_name ILIKE $%d OR author_username ILIKE $%d)", argIndex, argIndex, argIndex, argIndex, argIndex)
		args = append(args, searchPattern)
		argIndex++
	}

	// Get total count
	var total int
	err := s.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to count reviews")
	}

	// Add ordering and pagination
	baseQuery += " ORDER BY created_at DESC"
	offset := (page - 1) * perPage
	baseQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, perPage, offset)

	// Execute query
	rows, err := s.db.Query(baseQuery, args...)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch reviews")
	}
	defer rows.Close()

	// Initialize as empty slice instead of nil to ensure JSON marshals to [] not null
	reviews := make([]ReviewResponse, 0)
	for rows.Next() {
		var review ReviewResponse
		var metadataJSON sql.NullString
		var mrTitleNS, authorNameNS, authorUsernameNS sql.NullString

		err := rows.Scan(
			&review.ID,
			&review.Repository,
			&review.Branch,
			&review.CommitHash,
			&review.PrMrUrl,
			&review.ConnectorID,
			&review.Status,
			&review.TriggerType,
			&review.UserEmail,
			&review.Provider,
			&mrTitleNS,
			&authorNameNS,
			&authorUsernameNS,
			&review.CreatedAt,
			&review.StartedAt,
			&review.CompletedAt,
			&metadataJSON,
			&review.OrgID,
		)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to scan review")
		}

		// Parse metadata JSON
		if metadataJSON.Valid && metadataJSON.String != "" {
			review.Metadata = make(map[string]interface{})
			if err := json.Unmarshal([]byte(metadataJSON.String), &review.Metadata); err != nil {
				// Log error but continue with empty metadata
				fmt.Printf("Failed to parse metadata for review %d: %v\n", review.ID, err)
			}
		}

		if mrTitleNS.Valid {
			review.MRTitle = &mrTitleNS.String
		}
		if authorNameNS.Valid {
			review.AuthorName = &authorNameNS.String
		}
		if authorUsernameNS.Valid {
			review.AuthorUsername = &authorUsernameNS.String
		}

		reviews = append(reviews, review)
	}

	if err = rows.Err(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Error iterating reviews")
	}

	// Calculate pagination metadata
	totalPages := (total + perPage - 1) / perPage
	hasNext := page < totalPages
	hasPrevious := page > 1

	response := ReviewsListResponse{
		Reviews:     reviews,
		Total:       total,
		Page:        page,
		PerPage:     perPage,
		TotalPages:  totalPages,
		HasNext:     hasNext,
		HasPrevious: hasPrevious,
	}

	return c.JSON(http.StatusOK, response)
}

// createReview handles POST /api/v1/reviews (trigger review creation)
func (s *Server) createReview(c echo.Context) error {
	// This should delegate to the existing TriggerReviewV2 functionality
	// For now, return a placeholder that explains how to trigger reviews
	return c.JSON(http.StatusOK, map[string]string{
		"message": "Use POST /api/v1/connectors/trigger-review to create reviews",
		"note":    "Direct review creation will be implemented in a future update",
	})
}

// getReviewByID handles GET /api/v1/reviews/:id
func (s *Server) getReviewByID(c echo.Context) error {
	// Extract org context from middleware
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}

	// Parse review ID
	reviewIDStr := c.Param("id")
	reviewID, err := strconv.ParseInt(reviewIDStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid review ID")
	}

	// Query review by ID with org scoping
	query := `
		SELECT id, repository, branch, commit_hash, pr_mr_url, connector_id,
		       status, trigger_type, user_email, provider, mr_title, author_name, author_username,
		       created_at, started_at, completed_at, metadata, org_id
		FROM public.reviews 
		WHERE id = $1 AND org_id = $2
	`

	var review ReviewResponse
	var metadataJSON sql.NullString
	var mrTitleNS, authorNameNS, authorUsernameNS sql.NullString

	err = s.db.QueryRow(query, reviewID, orgID).Scan(
		&review.ID,
		&review.Repository,
		&review.Branch,
		&review.CommitHash,
		&review.PrMrUrl,
		&review.ConnectorID,
		&review.Status,
		&review.TriggerType,
		&review.UserEmail,
		&review.Provider,
		&mrTitleNS,
		&authorNameNS,
		&authorUsernameNS,
		&review.CreatedAt,
		&review.StartedAt,
		&review.CompletedAt,
		&metadataJSON,
		&review.OrgID,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return echo.NewHTTPError(http.StatusNotFound, "Review not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch review")
	}

	// Parse metadata JSON
	if metadataJSON.Valid && metadataJSON.String != "" {
		review.Metadata = make(map[string]interface{})
		if err := json.Unmarshal([]byte(metadataJSON.String), &review.Metadata); err != nil {
			// Log error but continue with empty metadata
			fmt.Printf("Failed to parse metadata for review %d: %v\n", review.ID, err)
		}
	}

	if mrTitleNS.Valid {
		review.MRTitle = &mrTitleNS.String
	}
	if authorNameNS.Valid {
		review.AuthorName = &authorNameNS.String
	}
	if authorUsernameNS.Valid {
		review.AuthorUsername = &authorUsernameNS.String
	}

	if review.Provider == nil || strings.TrimSpace(*review.Provider) == "" || strings.EqualFold(*review.Provider, "unknown") {
		if provider, err := s.lookupProviderFromEvents(c.Request().Context(), review.ID, review.OrgID); err == nil && provider != "" {
			providerCopy := provider
			review.Provider = &providerCopy
		} else if err != nil {
			log.Printf("Failed to resolve provider for review %d: %v", review.ID, err)
		}
	}

	return c.JSON(http.StatusOK, review)
}

func (s *Server) lookupProviderFromEvents(ctx context.Context, reviewID, orgID int64) (string, error) {
	const query = `
		SELECT data->>'message'
		FROM public.review_events
		WHERE review_id = $1
		  AND org_id = $2
		  AND data->>'message' ILIKE '%provider%'
		ORDER BY ts DESC
		LIMIT 50
	`

	rows, err := s.db.QueryContext(ctx, query, reviewID, orgID)
	if err != nil {
		return "", fmt.Errorf("failed to query provider events: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var message sql.NullString
		if err := rows.Scan(&message); err != nil {
			return "", fmt.Errorf("failed to scan provider message: %w", err)
		}
		if !message.Valid {
			continue
		}
		if provider := extractProviderFromMessage(message.String); provider != "" {
			return provider, nil
		}
	}

	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("error iterating provider events: %w", err)
	}

	return "", nil
}

func extractProviderFromMessage(message string) string {
	text := strings.TrimSpace(message)
	lower := strings.ToLower(text)

	if strings.Contains(lower, "ai provider") {
		return ""
	}

	patterns := []string{"provider type:", "provider:", "provider=", "provider is"}
	for _, pattern := range patterns {
		if idx := strings.Index(lower, pattern); idx >= 0 {
			value := strings.TrimSpace(text[idx+len(pattern):])
			value = strings.Trim(value, " .,:;")
			if i := strings.IndexAny(value, ",;|\n"); i >= 0 {
				value = value[:i]
			}
			if i := strings.Index(value, " "); i >= 0 {
				value = value[:i]
			}
			return normalizeProviderValue(value)
		}
	}

	switch {
	case strings.Contains(lower, "gitlab"):
		return "gitlab"
	case strings.Contains(lower, "github"):
		return "github"
	case strings.Contains(lower, "bitbucket"):
		return "bitbucket"
	default:
		return ""
	}
}

func normalizeProviderValue(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	lower := strings.ToLower(v)
	switch {
	case strings.Contains(lower, "gitlab"):
		return "gitlab"
	case strings.Contains(lower, "github"):
		return "github"
	case strings.Contains(lower, "bitbucket"):
		return "bitbucket"
	default:
		return v
	}
}

// validateConnectorOwnership checks if a connector belongs to the user's organization
// Returns the connector's org_id and an error response if validation fails
func (s *Server) validateConnectorOwnership(c echo.Context, connectorID int) (int64, error) {
	// Get org context from middleware
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return 0, c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Org context not found",
		})
	}

	// Query connector's org_id
	var connectorOrgID int64
	ownershipQuery := `SELECT org_id FROM integration_tokens WHERE id = $1`
	err := s.db.QueryRow(ownershipQuery, connectorID).Scan(&connectorOrgID)
	if err == sql.ErrNoRows {
		return 0, c.JSON(http.StatusNotFound, map[string]string{
			"error": "Connector not found",
		})
	} else if err != nil {
		return 0, c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to verify connector ownership",
		})
	}

	// Verify org ownership
	if connectorOrgID != orgID {
		return 0, c.JSON(http.StatusForbidden, map[string]string{
			"error": "Connector does not belong to your organization",
		})
	}

	return connectorOrgID, nil
}

// getVersion returns version information about the LiveReview API
func (s *Server) getVersion(c echo.Context) error {
	response := map[string]interface{}{
		"apiVersion": "v1",
	}

	if s.versionInfo != nil {
		response["version"] = s.versionInfo.Version
		response["gitCommit"] = s.versionInfo.GitCommit
		response["buildTime"] = s.versionInfo.BuildTime
		response["dirty"] = s.versionInfo.Dirty
	} else {
		response["version"] = "development"
		response["gitCommit"] = "unknown"
		response["buildTime"] = "unknown"
		response["dirty"] = false
	}

	return c.JSON(http.StatusOK, response)
}

// EnableManualTriggerForAllProjects handles enabling manual trigger for all projects for a connector
func (s *Server) EnableManualTriggerForAllProjects(c echo.Context) error {
	connectorIdStr := c.Param("connectorId")
	connectorId, err := strconv.Atoi(connectorIdStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid connector ID",
		})
	}

	// Validate connector ownership
	if _, err := s.validateConnectorOwnership(c, connectorId); err != nil {
		return err
	}

	// Get repository access data for this connector
	repositoryData, err := s.fetchAndCacheRepositoryData(connectorId, false, false)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to fetch repository data: %v", err),
		})
	}

	if repositoryData.Error != "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("Repository access error: %s", repositoryData.Error),
		})
	}

	// Get connector details to pass to job queue
	var provider, providerURL, patToken string
	query := `
		SELECT provider, provider_url, pat_token
		FROM integration_tokens
		WHERE id = $1
	`
	err = s.db.QueryRow(query, connectorId).Scan(&provider, &providerURL, &patToken)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to fetch connector details: %v", err),
		})
	}

	// Queue webhook installation jobs for each project
	ctx := context.Background()
	jobsQueued := 0
	var queueErrors []string

	for _, projectPath := range repositoryData.Projects {
		err := s.jobQueue.QueueWebhookInstallJob(ctx, connectorId, projectPath, provider, providerURL, patToken)
		if err != nil {
			queueErrors = append(queueErrors, fmt.Sprintf("Failed to queue job for %s: %v", projectPath, err))
		} else {
			jobsQueued++
		}
	}

	response := map[string]interface{}{
		"message":        "Manual trigger configuration started",
		"connector_id":   connectorId,
		"status":         "success",
		"total_projects": len(repositoryData.Projects),
		"jobs_queued":    jobsQueued,
		"trigger_state":  "manual_pending",
	}

	if len(queueErrors) > 0 {
		response["errors"] = queueErrors
		response["status"] = "partial_success"
	}

	return c.JSON(http.StatusOK, response)
}

// DisableManualTriggerForAllProjects handles disabling manual trigger for all projects for a connector
func (s *Server) DisableManualTriggerForAllProjects(c echo.Context) error {
	connectorIdStr := c.Param("connectorId")
	connectorId, err := strconv.Atoi(connectorIdStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid connector ID",
		})
	}

	// Validate connector ownership
	if _, err := s.validateConnectorOwnership(c, connectorId); err != nil {
		return err
	}

	// Get repository access data for this connector
	repositoryData, err := s.fetchAndCacheRepositoryData(connectorId, false, false)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to fetch repository data: %v", err),
		})
	}

	if repositoryData.Error != "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("Repository access error: %s", repositoryData.Error),
		})
	}

	// Get connector details to pass to job queue
	var provider, providerURL, patToken string
	query := `
		SELECT provider, provider_url, pat_token
		FROM integration_tokens
		WHERE id = $1
	`
	err = s.db.QueryRow(query, connectorId).Scan(&provider, &providerURL, &patToken)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to fetch connector details: %v", err),
		})
	}

	// Queue webhook removal jobs for each project
	ctx := context.Background()
	jobsQueued := 0
	var queueErrors []string

	for _, projectPath := range repositoryData.Projects {
		err := s.jobQueue.QueueWebhookRemovalJob(ctx, connectorId, projectPath, provider, providerURL, patToken)
		if err != nil {
			queueErrors = append(queueErrors, fmt.Sprintf("Failed to queue removal job for %s: %v", projectPath, err))
		} else {
			jobsQueued++
		}
	}

	response := map[string]interface{}{
		"message":        "Manual trigger removal started",
		"connector_id":   connectorId,
		"status":         "success",
		"total_projects": len(repositoryData.Projects),
		"jobs_queued":    jobsQueued,
		"trigger_state":  "disconnected_pending",
	}

	if len(queueErrors) > 0 {
		response["errors"] = queueErrors
		response["status"] = "partial_success"
	}

	return c.JSON(http.StatusOK, response)
}

// getSystemInfo returns system configuration information
func (s *Server) getSystemInfo(c echo.Context) error {
	deploymentConfig := s.deploymentConfig

	// Derive webhook URL based on deployment mode
	var webhookURL string
	if deploymentConfig.ReverseProxy {
		// Production: derive from current request
		scheme := "https"
		if c.Scheme() == "http" {
			scheme = "http"
		}
		host := c.Request().Host
		webhookURL = fmt.Sprintf("%s://%s/api/v1/gitlab-hook", scheme, host)
	} else {
		// Demo: localhost for display only
		webhookURL = fmt.Sprintf("http://localhost:%d/api/v1/gitlab-hook", deploymentConfig.BackendPort)
	}

	// Get the current browser URL for auto-population hints
	currentURL := ""
	if deploymentConfig.ReverseProxy {
		scheme := "https"
		if c.Scheme() == "http" {
			scheme = "http"
		}
		host := c.Request().Host
		currentURL = fmt.Sprintf("%s://%s", scheme, host)
	}

	info := map[string]interface{}{
		"deployment_mode": deploymentConfig.Mode,
		"api_url":         fmt.Sprintf("http://localhost:%d", deploymentConfig.BackendPort),
		"webhook_url":     webhookURL,
		"current_url":     currentURL, // For frontend auto-population
		"capabilities": map[string]interface{}{
			"webhooks_enabled":     deploymentConfig.WebhooksEnabled,
			"manual_triggers_only": !deploymentConfig.WebhooksEnabled,
			"external_access":      deploymentConfig.Mode == "production",
			"proxy_mode":           deploymentConfig.ReverseProxy,
			"git_provider_setup":   true,
		},
		"version": map[string]interface{}{
			"version":   s.versionInfo.Version,
			"gitCommit": s.versionInfo.GitCommit,
			"buildTime": s.versionInfo.BuildTime,
			"dirty":     s.versionInfo.Dirty,
		},
		"dev_mode": s.devMode,
	}

	return c.JSON(http.StatusOK, info)
}

// WebhookOrchestratorV2Handler handles webhooks using the V2 orchestrator (full processing pipeline)
func (s *Server) WebhookOrchestratorV2Handler(c echo.Context) error {
	if s.webhookOrchestratorV2 == nil {
		log.Printf("[ERROR] Webhook orchestrator V2 not initialized")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Orchestrator not available",
		})
	}

	return s.webhookOrchestratorV2.ProcessWebhookEvent(c)
}
