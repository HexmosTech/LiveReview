package api

import (
	"net/http"
	"net/url"

	"github.com/labstack/echo/v4"
)

// TriggerReviewRequest represents the request to trigger a new code review
type TriggerReviewRequest struct {
	URL string `json:"url"`
}

// TriggerReviewResponse represents the response from triggering a code review
type TriggerReviewResponse struct {
	Message  string `json:"message"`
	URL      string `json:"url"`
	ReviewID string `json:"reviewId"`
}

// TriggerReview handles the request to trigger a code review from a URL
func (s *Server) TriggerReview(c echo.Context) error {
	// Check if the user is authenticated
	password := c.Request().Header.Get("X-Admin-Password")
	if password == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Authentication required",
		})
	}

	// Get the stored hashed password
	var hashedPassword string
	err := s.db.QueryRow("SELECT admin_password FROM instance_details LIMIT 1").Scan(&hashedPassword)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to check authentication: " + err.Error(),
		})
	}

	// Verify the provided password
	if !comparePasswords(hashedPassword, password) {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Invalid authentication",
		})
	}

	// Parse request body
	req := new(TriggerReviewRequest)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request format: " + err.Error(),
		})
	}

	// Validate URL
	if req.URL == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "URL is required",
		})
	}

	// Parse the URL to ensure it's valid
	parsedURL, err := url.Parse(req.URL)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid URL format: " + err.Error(),
		})
	}

	// Extract base URL for connector validation
	baseURL := parsedURL.Scheme + "://" + parsedURL.Host

	// Verify that the URL is from a connected Git provider
	var count int
	err = s.db.QueryRow("SELECT COUNT(*) FROM integration_tokens WHERE provider_url LIKE $1", "%"+baseURL+"%").Scan(&count)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Database error: " + err.Error(),
		})
	}

	if count == 0 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "URL is not from a connected Git provider. Please connect the provider first.",
		})
	}

	// TODO: Implement actual review triggering logic
	// For now, just return a dummy success response

	return c.JSON(http.StatusOK, TriggerReviewResponse{
		Message:  "Review triggered successfully",
		URL:      req.URL,
		ReviewID: "dummy-review-123", // This would be a real ID in the actual implementation
	})
}
