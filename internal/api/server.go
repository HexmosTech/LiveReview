package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/livereview/internal/jobqueue"
	// Import FetchGitLabProfile
)

// Server represents the API server
type Server struct {
	echo                 *echo.Echo
	port                 int
	db                   *sql.DB
	jobQueue             *jobqueue.JobQueue
	dashboardManager     *DashboardManager
	autoWebhookInstaller *AutoWebhookInstaller
}

// NewServer creates a new API server
func NewServer(port int) (*Server, error) {
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

	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	server := &Server{
		echo:                 e,
		port:                 port,
		db:                   db,
		jobQueue:             jq,
		dashboardManager:     dashboardManager,
		autoWebhookInstaller: autoWebhookInstaller,
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

	// API v1 group
	v1 := s.echo.Group("/api/v1")

	// Reviews endpoints
	v1.GET("/reviews", s.getReviews)
	v1.POST("/reviews", s.createReview)
	v1.GET("/reviews/:id", s.getReviewByID)

	// Password management endpoints
	v1.POST("/password", s.SetAdminPassword)
	v1.PUT("/password", s.ResetAdminPassword)
	v1.POST("/password/verify", s.VerifyAdminPassword)
	v1.GET("/password/status", s.CheckAdminPasswordStatus)

	// Production URL endpoints
	v1.GET("/production-url", s.GetProductionURL)
	v1.PUT("/production-url", s.UpdateProductionURL)

	// GitLab OAuth endpoints
	v1.POST("/gitlab/token", s.GitLabHandleCodeExchange)
	v1.POST("/gitlab/refresh", s.GitLabRefreshToken)

	// Connector endpoints
	v1.GET("/connectors", s.GetConnectors)
	v1.GET("/connectors/:id", s.GetConnector)
	v1.DELETE("/connectors/:id", s.DeleteConnector)
	v1.GET("/connectors/:connectorId/repository-access", s.GetRepositoryAccess)
	v1.POST("/connectors/:connectorId/enable-manual-trigger", s.EnableManualTriggerForAllProjects)
	v1.POST("/connectors/:connectorId/disable-manual-trigger", s.DisableManualTriggerForAllProjects)

	// GitLab profile validation endpoint
	v1.POST("/gitlab/validate-profile", s.ValidateGitLabProfile)

	// GitHub profile validation endpoint
	v1.POST("/github/validate-profile", s.ValidateGitHubProfile)

	// Bitbucket profile validation endpoint
	v1.POST("/bitbucket/validate-profile", s.ValidateBitbucketProfile)

	// Create PAT integration token endpoint
	v1.POST("/integration_tokens/pat", s.HandleCreatePATIntegrationToken)

	// Review trigger endpoints
	v1.POST("/trigger-review", s.TriggerReviewV2)

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
	v1.DELETE("/aiconnectors/:id", s.DeleteAIConnector)

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
	// Print server starting message
	fmt.Printf("API server is running at http://localhost:%d\n", s.port)
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
		if err := s.echo.Start(fmt.Sprintf(":%d", s.port)); err != nil && err != http.ErrServerClosed {
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
