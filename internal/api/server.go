package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	// Import FetchGitLabProfile
)

// Server represents the API server
type Server struct {
	echo *echo.Echo
	port int
	db   *sql.DB
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

	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	server := &Server{
		echo: e,
		port: port,
		db:   db,
	}

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
	v1.DELETE("/connectors/:id", s.DeleteConnector)

	// GitLab profile validation endpoint
	v1.POST("/gitlab/validate-profile", s.ValidateGitLabProfile)

	// GitHub profile validation endpoint
	v1.POST("/github/validate-profile", s.ValidateGitHubProfile)

	// Bitbucket profile validation endpoint
	v1.POST("/bitbucket/validate-profile", s.ValidateBitbucketProfile)

	// Create PAT integration token endpoint
	v1.POST("/integration_tokens/pat", s.HandleCreatePATIntegrationToken)

	// Review trigger endpoints
	v1.POST("/trigger-review", s.TriggerReview)

	// AI Connector endpoints
	v1.POST("/aiconnectors/validate-key", s.ValidateAIConnectorKey)
	v1.POST("/aiconnectors", s.CreateAIConnector)
	v1.GET("/aiconnectors", s.GetAIConnectors)
	v1.DELETE("/aiconnectors/:id", s.DeleteAIConnector)
}

// Handler for creating PAT integration token, delegates to pat_token.go
func (s *Server) HandleCreatePATIntegrationToken(c echo.Context) error {
	return HandleCreatePATIntegrationToken(s.db, c)
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
