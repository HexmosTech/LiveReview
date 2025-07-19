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
