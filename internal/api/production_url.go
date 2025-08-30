package api

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

// ProductionURLRequest represents a request to update the production URL
type ProductionURLRequest struct {
	URL string `json:"url"`
}

// ProductionURLResponse is the response for production URL operations
type ProductionURLResponse struct {
	URL     string `json:"url"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// GetProductionURL retrieves the livereview_prod_url from instance_details
func (s *Server) GetProductionURL(c echo.Context) error {
	var url sql.NullString
	err := s.db.QueryRow("SELECT livereview_prod_url FROM instance_details LIMIT 1").Scan(&url)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusOK, ProductionURLResponse{
				URL:     "",
				Success: false,
				Message: "No production URL configured",
			})
		}
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to retrieve production URL: " + err.Error(),
		})
	}

	// Handle the case when URL is NULL in the database
	urlValue := ""
	if url.Valid {
		urlValue = url.String
	}

	message := "No production URL configured"
	if urlValue != "" {
		message = "Production URL retrieved successfully"
	}

	return c.JSON(http.StatusOK, ProductionURLResponse{
		URL:     urlValue,
		Success: urlValue != "",
		Message: message,
	})
}

// UpdateProductionURL updates the livereview_prod_url in instance_details
func (s *Server) UpdateProductionURL(c echo.Context) error {
	var req ProductionURLRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request format",
		})
	}

	// Validate URL
	// Empty URLs are allowed (will be stored as NULL)
	// We're allowing empty URL to effectively clear the current setting

	// Check if instance_details record exists
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM instance_details").Scan(&count)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to check instance details: " + err.Error(),
		})
	}

	if count == 0 {
		// Insert a new record if none exists, providing default value for legacy admin_password
		_, err = s.db.Exec(`
			INSERT INTO instance_details (livereview_prod_url, admin_password) 
			VALUES (NULLIF($1, ''), 'legacy_placeholder')
		`, req.URL)
	} else {
		// Update existing record
		_, err = s.db.Exec(`
			UPDATE instance_details 
			SET livereview_prod_url = NULLIF($1, ''), updated_at = CURRENT_TIMESTAMP 
			WHERE id = (SELECT id FROM instance_details LIMIT 1)
		`, req.URL)
	}

	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to update production URL: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, ProductionURLResponse{
		URL:     req.URL,
		Success: true,
		Message: "Production URL has been updated successfully",
	})
}

// GetProductionURLDirectly gets the production URL directly (for CLI use)
func (s *Server) GetProductionURLDirectly() (string, error) {
	var url sql.NullString
	err := s.db.QueryRow("SELECT livereview_prod_url FROM instance_details LIMIT 1").Scan(&url)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil // Return empty string with no error when no record exists
		}
		return "", fmt.Errorf("failed to retrieve production URL: %v", err)
	}

	// Handle NULL URL
	if !url.Valid {
		return "", nil
	}

	return url.String, nil
}

// UpdateProductionURLDirectly updates the production URL directly (for CLI use)
func (s *Server) UpdateProductionURLDirectly(url string) error {
	// Empty URLs are allowed (will be stored as NULL)
	// We're allowing empty URL to effectively clear the current setting

	// Check if instance_details record exists
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM instance_details").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check instance details: %v", err)
	}

	var result sql.Result
	if count == 0 {
		// Insert a new record if none exists, providing default value for legacy admin_password
		result, err = s.db.Exec(`
			INSERT INTO instance_details (livereview_prod_url, admin_password) 
			VALUES (NULLIF($1, ''), 'legacy_placeholder')
		`, url)
	} else {
		// Update existing record
		result, err = s.db.Exec(`
			UPDATE instance_details 
			SET livereview_prod_url = NULLIF($1, ''), updated_at = CURRENT_TIMESTAMP 
			WHERE id = (SELECT id FROM instance_details LIMIT 1)
		`, url)
	}

	if err != nil {
		return fmt.Errorf("failed to update production URL: %v", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("no records were updated")
	}

	return nil
}
