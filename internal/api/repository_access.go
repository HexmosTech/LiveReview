package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/livereview/internal/providers/gitlab"
)

// RepositoryAccessResponse represents the response for repository access information
type RepositoryAccessResponse struct {
	ConnectorID  int       `json:"connector_id"`
	Provider     string    `json:"provider"`
	BaseURL      string    `json:"base_url"`
	Projects     []string  `json:"projects"`
	ProjectCount int       `json:"project_count"`
	Error        string    `json:"error,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CachedProjectData represents the cached project data structure
type CachedProjectData struct {
	Projects     []string  `json:"projects"`
	ProjectCount int       `json:"project_count"`
	CachedAt     time.Time `json:"cached_at"`
	Error        string    `json:"error,omitempty"`
}

// fetchAndCacheRepositoryData fetches repository data and optionally caches it
// Parameters:
//   - connectorID: The ID of the connector to fetch data for
//   - forceRefresh: If true, ignores cache and fetches fresh data
//   - shouldCache: If true, stores the result in cache
//
// Returns:
//   - RepositoryAccessResponse: The repository access data
//   - error: Any error that occurred during the process
func (s *Server) fetchAndCacheRepositoryData(connectorID int, forceRefresh bool, shouldCache bool) (*RepositoryAccessResponse, error) {
	// Query the database for the connector information
	var provider, providerURL, patToken string
	var cachedDataJSON sql.NullString

	query := `
		SELECT provider, provider_url, pat_token, projects_cache
		FROM integration_tokens
		WHERE id = $1
	`

	err := s.db.QueryRow(query, connectorID).Scan(&provider, &providerURL, &patToken, &cachedDataJSON)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("connector not found")
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Initialize response
	response := &RepositoryAccessResponse{
		ConnectorID:  connectorID,
		Provider:     provider,
		BaseURL:      providerURL,
		Projects:     []string{},
		ProjectCount: 0,
		UpdatedAt:    time.Now(),
	}

	// Check if we have cached data and not forcing refresh
	if !forceRefresh && cachedDataJSON.Valid && cachedDataJSON.String != "" {
		var cachedData CachedProjectData
		if err := json.Unmarshal([]byte(cachedDataJSON.String), &cachedData); err == nil {
			// Use cached data
			response.Projects = cachedData.Projects
			response.ProjectCount = cachedData.ProjectCount
			response.Error = cachedData.Error
			response.UpdatedAt = cachedData.CachedAt
			return response, nil
		}
		// If unmarshaling fails, continue with fresh fetch
	}

	// Only process GitLab providers for now
	if provider != "gitlab" && provider != "gitlab-com" && provider != "gitlab-self-hosted" {
		response.Error = fmt.Sprintf("Repository discovery not yet implemented for provider: %s", provider)
		if shouldCache {
			s.updateProjectsCache(connectorID, response)
		}
		return response, nil
	}

	// Check if we have a PAT token
	if patToken == "" {
		response.Error = "No PAT token found for this connector"
		if shouldCache {
			s.updateProjectsCache(connectorID, response)
		}
		return response, nil
	}

	// Use the GitLab project discovery function
	projects, err := gitlab.DiscoverProjectsGitlab(providerURL, patToken)
	if err != nil {
		response.Error = fmt.Sprintf("Failed to discover projects: %s", err.Error())
		if shouldCache {
			s.updateProjectsCache(connectorID, response)
		}
		return response, nil
	}

	response.Projects = projects
	response.ProjectCount = len(projects)

	// Cache the result if requested
	if shouldCache {
		s.updateProjectsCache(connectorID, response)
	}

	return response, nil
}

// updateProjectsCache updates the projects_cache column for the given connector
func (s *Server) updateProjectsCache(connectorID int, response *RepositoryAccessResponse) {
	cachedData := CachedProjectData{
		Projects:     response.Projects,
		ProjectCount: response.ProjectCount,
		CachedAt:     time.Now(),
		Error:        response.Error,
	}

	cachedDataJSON, err := json.Marshal(cachedData)
	if err != nil {
		// Log error but don't fail the request
		fmt.Printf("Failed to marshal cache data: %v\n", err)
		return
	}

	_, err = s.db.Exec(`
		UPDATE integration_tokens 
		SET projects_cache = $1, updated_at = $2 
		WHERE id = $3
	`, cachedDataJSON, time.Now(), connectorID)

	if err != nil {
		// Log error but don't fail the request
		fmt.Printf("Failed to update cache: %v\n", err)
	}
}

// GetRepositoryAccess fetches repository access information for a connector
// Query Parameters:
//   - refresh: Set to "true" to force refresh the cached data (optional)
//
// Example usage:
//   - GET /api/repository-access/{connectorId} - Returns cached data if available
//   - GET /api/repository-access/{connectorId}?refresh=true - Forces fresh data fetch and updates cache
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

	// Check if refresh is requested
	forceRefresh := c.QueryParam("refresh") == "true"

	// Fetch data with caching
	response, err := s.fetchAndCacheRepositoryData(connectorID, forceRefresh, true)
	if err != nil {
		if err.Error() == "connector not found" {
			return c.JSON(http.StatusNotFound, ErrorResponse{
				Error: "Connector not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, response)
}
