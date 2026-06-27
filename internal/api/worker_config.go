package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// WorkerConfigResponse is the response for worker configuration query
type WorkerConfigResponse struct {
	WorkerConcurrentReviews int `json:"worker_concurrent_reviews"`
}

// UpdateWorkerConfigRequest represents a request to update the worker concurrency limit
type UpdateWorkerConfigRequest struct {
	WorkerConcurrentReviews int `json:"worker_concurrent_reviews"`
}

// GetWorkerConfig retrieves the worker_concurrent_reviews from instance_details
func (s *Server) GetWorkerConfig(c echo.Context) error {
	var concurrency int
	// instance_details is a singleton table — the row always exists (created during admin password setup).
	err := s.db.QueryRow("SELECT COALESCE(worker_concurrent_reviews, 10) FROM instance_details ORDER BY id ASC LIMIT 1").Scan(&concurrency)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to retrieve worker concurrency: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, WorkerConfigResponse{
		WorkerConcurrentReviews: concurrency,
	})
}

// UpdateWorkerConfig updates the worker_concurrent_reviews in instance_details
func (s *Server) UpdateWorkerConfig(c echo.Context) error {
	var req UpdateWorkerConfigRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request format",
		})
	}

	// Validate the range limit [1, 40]
	if req.WorkerConcurrentReviews < 1 || req.WorkerConcurrentReviews > 40 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Worker concurrency limit must be between 1 and 40. High concurrency loads may require a connection pooler like pgBouncer.",
		})
	}

	// instance_details is a singleton table — the row always exists (created during admin password setup).
	// Only update worker_concurrent_reviews; never touch other columns like admin_password.
	_, err := s.db.Exec(`
		UPDATE instance_details
		SET worker_concurrent_reviews = $1, updated_at = CURRENT_TIMESTAMP
	`, req.WorkerConcurrentReviews)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to update worker concurrency: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":                   true,
		"message":                   "Worker concurrency updated successfully. A restart of the background worker process is required for changes to take effect.",
		"worker_concurrent_reviews": req.WorkerConcurrentReviews,
	})
}
