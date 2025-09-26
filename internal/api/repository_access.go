package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/livereview/internal/providers/bitbucket"
	"github.com/livereview/internal/providers/github"
	"github.com/livereview/internal/providers/gitlab"
)

// ProjectWithStatus represents a project with its webhook status
type ProjectWithStatus struct {
	ProjectPath   string     `json:"project_path"`
	WebhookStatus string     `json:"webhook_status"` // "unconnected", "manual", "automatic"
	LastVerified  *time.Time `json:"last_verified,omitempty"`
	WebhookID     string     `json:"webhook_id,omitempty"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// RepositoryAccessResponse represents the response for repository access information
type RepositoryAccessResponse struct {
	ConnectorID        int                 `json:"connector_id"`
	Provider           string              `json:"provider"`
	BaseURL            string              `json:"base_url"`
	Projects           []string            `json:"projects"` // Keep for backward compatibility, can be null
	ProjectsWithStatus []ProjectWithStatus `json:"projects_with_status"`
	ProjectCount       int                 `json:"project_count"`
	Error              string              `json:"error,omitempty"`
	UpdatedAt          time.Time           `json:"updated_at"`
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
	var metadataBytes []byte

	query := `
		SELECT provider, provider_url, pat_token, projects_cache, COALESCE(metadata, '{}')
		FROM integration_tokens
		WHERE id = $1
	`

	err := s.db.QueryRow(query, connectorID).Scan(&provider, &providerURL, &patToken, &cachedDataJSON, &metadataBytes)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("connector not found")
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Parse metadata JSON
	var metadata map[string]interface{}
	if len(metadataBytes) > 0 {
		if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
			metadata = make(map[string]interface{})
		}
	} else {
		metadata = make(map[string]interface{})
	}

	// Initialize response
	response := &RepositoryAccessResponse{
		ConnectorID:        connectorID,
		Provider:           provider,
		BaseURL:            providerURL,
		Projects:           []string{},            // Initialize as empty array
		ProjectsWithStatus: []ProjectWithStatus{}, // Initialize as empty array
		ProjectCount:       0,
		UpdatedAt:          time.Now(),
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

			// Always fetch fresh webhook statuses (they change frequently)
			if len(cachedData.Projects) > 0 {
				webhookStatuses, err := s.fetchWebhookStatuses(connectorID, cachedData.Projects)
				if err != nil {
					fmt.Printf("Warning: Failed to fetch webhook statuses for cached data: %v\n", err)
				}

				// Build ProjectsWithStatus array
				response.ProjectsWithStatus = make([]ProjectWithStatus, 0, len(cachedData.Projects))
				for _, project := range cachedData.Projects {
					if status, exists := webhookStatuses[project]; exists {
						response.ProjectsWithStatus = append(response.ProjectsWithStatus, status)
					} else {
						response.ProjectsWithStatus = append(response.ProjectsWithStatus, ProjectWithStatus{
							ProjectPath:   project,
							WebhookStatus: "unconnected",
							UpdatedAt:     time.Now(),
						})
					}
				}
			}

			return response, nil
		}
		// If unmarshaling fails, continue with fresh fetch
	}

	// Support GitLab, GitHub, and Bitbucket providers
	if provider != "gitlab" && provider != "gitlab-com" && provider != "gitlab-self-hosted" &&
		provider != "github" && provider != "github-com" && provider != "github-enterprise" &&
		provider != "bitbucket" {
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

	var projects []string

	// Use the appropriate project discovery function based on provider
	if strings.HasPrefix(provider, "gitlab") {
		// Use the GitLab project discovery function
		projects, err = gitlab.DiscoverProjectsGitlab(providerURL, patToken)
	} else if strings.HasPrefix(provider, "github") {
		// Use the GitHub project discovery function
		projects, err = github.DiscoverProjectsGitHub(providerURL, patToken)
	} else if strings.HasPrefix(provider, "bitbucket") {
		// Use the Bitbucket project discovery function
		// For Bitbucket, we need email from metadata
		email, ok := metadata["email"].(string)
		if !ok || email == "" {
			response.Error = "Bitbucket connector missing email in metadata"
			if shouldCache {
				s.updateProjectsCache(connectorID, response)
			}
			return response, nil
		}
		projects, err = bitbucket.DiscoverProjectsBitbucket(providerURL, email, patToken)
	} else {
		response.Error = fmt.Sprintf("Unsupported provider: %s", provider)
		if shouldCache {
			s.updateProjectsCache(connectorID, response)
		}
		return response, nil
	}

	if err != nil {
		response.Error = fmt.Sprintf("Failed to discover projects: %s", err.Error())
		if shouldCache {
			s.updateProjectsCache(connectorID, response)
		}
		return response, nil
	}

	response.Projects = projects
	response.ProjectCount = len(projects)

	// Fetch webhook statuses for all projects
	webhookStatuses, err := s.fetchWebhookStatuses(connectorID, projects)
	if err != nil {
		fmt.Printf("Warning: Failed to fetch webhook statuses: %v\n", err)
	}

	// Build ProjectsWithStatus array
	response.ProjectsWithStatus = make([]ProjectWithStatus, 0, len(projects))
	for _, project := range projects {
		if status, exists := webhookStatuses[project]; exists {
			response.ProjectsWithStatus = append(response.ProjectsWithStatus, status)
		} else {
			// Default to unconnected if no webhook status found
			response.ProjectsWithStatus = append(response.ProjectsWithStatus, ProjectWithStatus{
				ProjectPath:   project,
				WebhookStatus: "unconnected",
				UpdatedAt:     time.Now(),
			})
		}
	}

	// Cache the result if requested
	if shouldCache {
		s.updateProjectsCache(connectorID, response)
	}

	return response, nil
}

// fetchWebhookStatuses fetches webhook statuses for all projects in a single query
func (s *Server) fetchWebhookStatuses(connectorID int, projects []string) (map[string]ProjectWithStatus, error) {
	if len(projects) == 0 {
		return make(map[string]ProjectWithStatus), nil
	}

	// Create a map to store results
	statusMap := make(map[string]ProjectWithStatus)

	// Initialize all projects with "unconnected" status
	for _, project := range projects {
		statusMap[project] = ProjectWithStatus{
			ProjectPath:   project,
			WebhookStatus: "unconnected",
			UpdatedAt:     time.Now(),
		}
	}

	// Fetch webhook statuses from database in a single query
	query := `
		SELECT project_full_name, status, last_verified_at, webhook_id, updated_at
		FROM webhook_registry 
		WHERE integration_token_id = $1
	`

	rows, err := s.db.Query(query, connectorID)
	if err != nil {
		return statusMap, fmt.Errorf("failed to fetch webhook statuses: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var projectPath, status, webhookID string
		var lastVerified, updatedAt sql.NullTime

		err := rows.Scan(&projectPath, &status, &lastVerified, &webhookID, &updatedAt)
		if err != nil {
			continue // Skip invalid rows
		}

		projectStatus := ProjectWithStatus{
			ProjectPath:   projectPath,
			WebhookStatus: status,
			WebhookID:     webhookID,
			UpdatedAt:     time.Now(),
		}

		if lastVerified.Valid {
			projectStatus.LastVerified = &lastVerified.Time
		}

		if updatedAt.Valid {
			projectStatus.UpdatedAt = updatedAt.Time
		}

		statusMap[projectPath] = projectStatus
	}

	return statusMap, nil
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
	// JWT authentication is already handled by the RequireAuth() middleware
	// No additional authentication checks needed

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
