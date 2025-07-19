package api

import (
	"context"
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
}

// NewServer creates a new API server
func NewServer(port int) *Server {
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	server := &Server{
		echo: e,
		port: port,
	}

	// Setup routes
	server.setupRoutes()

	return server
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
	// Start server in a goroutine
	go func() {
		if err := s.echo.Start(fmt.Sprintf(":%d", s.port)); err != nil && err != http.ErrServerClosed {
			s.echo.Logger.Fatal("shutting down the server")
		}
	}()

	// Wait for interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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
