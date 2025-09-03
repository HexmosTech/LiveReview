package api

import (
	"context"
	"database/sql"
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
	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/api/organizations"
	"github.com/livereview/internal/api/users"
	"github.com/livereview/internal/jobqueue"
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
	if valueStr == "" {
		return defaultValue
	}
	return valueStr == "true" || valueStr == "1"
}

// getDeploymentConfig reads deployment configuration from environment variables
func getDeploymentConfig() *DeploymentConfig {
	config := &DeploymentConfig{
		BackendPort:  getEnvInt("LIVEREVIEW_BACKEND_PORT", 8888),
		FrontendPort: getEnvInt("LIVEREVIEW_FRONTEND_PORT", 8081),
		ReverseProxy: getEnvBool("LIVEREVIEW_REVERSE_PROXY", false),
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
}

// NewServer creates a new API server
func NewServer(port int, versionInfo *VersionInfo) (*Server, error) {
	// Get deployment configuration from environment variables
	deploymentConfig := getDeploymentConfig()

	// Override port from deployment config if provided
	if deploymentConfig.BackendPort != 8888 {
		port = deploymentConfig.BackendPort
	}

	// Load environment variables from .env file
	env, err := loadEnvFile(".env")
	if err != nil {
		return nil, fmt.Errorf("error loading .env file: %v\n\nPlease create a .env file with DATABASE_URL like:\nDATABASE_URL=postgres://username:password@localhost:5432/dbname?sslmode=disable", err)
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
	jq, err := jobqueue.NewJobQueue(dbURL)
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
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Add database to context
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("db", db)
			return next(c)
		}
	})

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
	}

	// Set the server reference in auto webhook installer (circular dependency)
	autoWebhookInstaller.server = server

	// Setup routes
	server.setupRoutes()

	return server, nil
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

	// Protected routes (require authentication)
	protected := v1.Group("")
	protected.Use(auth.RequireAuth(s.tokenService, s.db))

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
	authMiddleware := auth.NewAuthMiddleware(s.tokenService, s.db)

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
	// User organization access (get their orgs)
	protected.GET("/organizations", s.orgHandlers.GetUserOrganizations)
	protected.GET("/organizations/:org_id", s.orgHandlers.GetOrganization)

	// Organization management within org context
	orgGroup.GET("/members", s.orgHandlers.GetOrganizationMembers)
	orgGroup.PUT("/members/:user_id/role", s.orgHandlers.ChangeUserRole)
	orgGroup.GET("/analytics", s.orgHandlers.GetOrganizationAnalytics)
	orgGroup.PUT("", s.orgHandlers.UpdateOrganization) // Update org details (owners only)

	// Super admin organization management
	adminGroup.POST("/organizations", s.orgHandlers.CreateOrganization)
	adminGroup.DELETE("/organizations/:org_id", s.orgHandlers.DeactivateOrganization)

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

	// Reviews endpoints (protected)
	protected.GET("/reviews", s.getReviews)
	protected.POST("/reviews", s.createReview)
	protected.GET("/reviews/:id", s.getReviewByID)

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

	// GitLab webhook handler
	v1.POST("/gitlab-hook", s.GitLabWebhookHandler)

	// GitHub webhook handler
	v1.POST("/github-hook", s.GitHubWebhookHandler)

	// Bitbucket webhook handler
	v1.POST("/bitbucket-hook", s.BitbucketWebhookHandler)

	// AI Connector endpoints
	v1.POST("/aiconnectors/validate-key", s.ValidateAIConnectorKey)
	v1.POST("/aiconnectors", s.CreateAIConnector)
	v1.GET("/aiconnectors", s.GetAIConnectors)
	v1.PUT("/aiconnectors/:id", s.UpdateAIConnector)
	v1.PUT("/aiconnectors/reorder", s.ReorderAIConnectors)
	v1.DELETE("/aiconnectors/:id", s.DeleteAIConnector)

	// Register additional AI connector handlers (including Ollama)
	aiconnectors.RegisterHandlers(s.echo)

	// Dashboard endpoints
	v1.GET("/dashboard", s.GetDashboardData)
	v1.POST("/dashboard/refresh", s.RefreshDashboardData)

	// Activity endpoints
	v1.GET("/activities", s.GetRecentActivities)
}

// Handler for creating PAT integration token, delegates to pat_token.go
func (s *Server) HandleCreatePATIntegrationToken(c echo.Context) error {
	// Create the PAT connector and get the ID
	connectorID, err := CreatePATIntegrationToken(s.db, c)
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
		// Get connector details from database to track them properly
		var provider, providerURL string
		query := `SELECT provider, provider_url FROM integration_tokens WHERE id = $1`
		err := s.db.QueryRow(query, connectorID).Scan(&provider, &providerURL)
		if err == nil {
			TrackConnectorCreated(s.db, provider, providerURL, int(connectorID), 0) // repository count will be updated later
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
	profile, err := FetchGitLabProfile(body.BaseURL, body.PAT)
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
	profile, err := FetchGitHubProfile(body.PAT)
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
	var bindAddress string
	if s.deploymentConfig.Mode == "demo" {
		bindAddress = fmt.Sprintf("127.0.0.1:%d", s.port)
	} else {
		bindAddress = fmt.Sprintf("127.0.0.1:%d", s.port)
	}

	// Print server starting message with deployment mode info
	fmt.Printf("API server starting in %s mode\n", s.deploymentConfig.Mode)
	fmt.Printf("API server is running at http://localhost:%d\n", s.port)
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

	return s.echo.Shutdown(ctx)
}

// Sample handler implementations
func (s *Server) getReviews(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"message": "Get all reviews - to be implemented",
	})
}

func (s *Server) createReview(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"message": "Create review - to be implemented",
	})
}

func (s *Server) getReviewByID(c echo.Context) error {
	id := c.Param("id")
	return c.JSON(http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Get review with ID: %s - to be implemented", id),
	})
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

	info := map[string]interface{}{
		"deployment_mode": deploymentConfig.Mode,
		"api_url":         fmt.Sprintf("http://localhost:%d", deploymentConfig.BackendPort),
		"capabilities": map[string]interface{}{
			"webhooks_enabled":     deploymentConfig.WebhooksEnabled,
			"manual_triggers_only": !deploymentConfig.WebhooksEnabled,
			"external_access":      deploymentConfig.Mode == "production",
			"proxy_mode":           deploymentConfig.ReverseProxy,
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
