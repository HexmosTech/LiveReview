package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/livereview/internal/providers/gitlab"
)

// RepositoryAccessResponse represents the response for repository access information
type RepositoryAccessResponse struct {
	ConnectorID  int      `json:"connector_id"`
	Provider     string   `json:"provider"`
	BaseURL      string   `json:"base_url"`
	Projects     []string `json:"projects"`
	ProjectCount int      `json:"project_count"`
	Error        string   `json:"error,omitempty"`
}

// GetRepositoryAccess fetches repository access information for a connector
func (s *Server) GetRepositoryAccess(c echo.Context) error {
	// Check authentication
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

	// Get connector ID from URL parameter
	connectorIDStr := c.Param("connectorId")
	connectorID, err := strconv.Atoi(connectorIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid connector ID",
		})
	}

	// Query the database for the connector information
	var provider, providerURL, patToken string
	err = s.db.QueryRow(`
		SELECT provider, provider_url, pat_token
		FROM integration_tokens
		WHERE id = $1
	`, connectorID).Scan(&provider, &providerURL, &patToken)

	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, ErrorResponse{
				Error: "Connector not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Database error: " + err.Error(),
		})
	}

	// Initialize response
	response := RepositoryAccessResponse{
		ConnectorID:  connectorID,
		Provider:     provider,
		BaseURL:      providerURL,
		Projects:     []string{},
		ProjectCount: 0,
	}

	// Only process GitLab providers for now
	if provider != "gitlab" && provider != "gitlab-com" && provider != "gitlab-self-hosted" {
		response.Error = fmt.Sprintf("Repository discovery not yet implemented for provider: %s", provider)
		return c.JSON(http.StatusOK, response)
	}

	// Check if we have a PAT token
	if patToken == "" {
		response.Error = "No PAT token found for this connector"
		return c.JSON(http.StatusOK, response)
	}

	// Use the GitLab project discovery function
	projects, err := gitlab.DiscoverProjectsGitlab(providerURL, patToken)
	if err != nil {
		response.Error = fmt.Sprintf("Failed to discover projects: %s", err.Error())
		return c.JSON(http.StatusOK, response)
	}

	response.Projects = projects
	response.ProjectCount = len(projects)

	return c.JSON(http.StatusOK, response)
}
