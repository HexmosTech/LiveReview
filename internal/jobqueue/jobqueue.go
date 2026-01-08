/*
Package jobqueue provides a River-based job queue system for managing webhook installations.

For configuration options, retry policies, and tuning parameters, see queue_config.go.
All configurable values have been moved there for easier management.

WEBHOOK MIGRATION STRATEGY:
Existing webhooks with old URLs (without connector_id) will NOT be automatically migrated.
New webhook URL format: {baseURL}/api/v1/{provider}-hook/{connector_id}
Old webhook URL format: {baseURL}/api/v1/{provider}-hook (deprecated)

Users must manually re-enable manual trigger from the ConnectorDetails UI page to update
their webhooks to the new connector-scoped URL format. This ensures proper org context
derivation and multi-org isolation. Old webhooks will return 404 after deployment,
prompting users to reconfigure via the "Enable Manual Trigger" button.
*/
package jobqueue

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livereview/internal/providers/gitea"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// GitLab API response structures
type GitLabProject struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	NameWithNamespace string `json:"name_with_namespace"`
	PathWithNamespace string `json:"path_with_namespace"`
	WebURL            string `json:"web_url"`
}

type GitLabHook struct {
	ID                    int    `json:"id"`
	URL                   string `json:"url"`
	PushEvents            bool   `json:"push_events"`
	IssuesEvents          bool   `json:"issues_events"`
	MergeRequestsEvents   bool   `json:"merge_requests_events"`
	TagPushEvents         bool   `json:"tag_push_events"`
	NoteEvents            bool   `json:"note_events"`
	JobEvents             bool   `json:"job_events"`
	PipelineEvents        bool   `json:"pipeline_events"`
	EnableSSLVerification bool   `json:"enable_ssl_verification"`
}

type WebhookPayload struct {
	URL                   string `json:"url"`
	Token                 string `json:"token"`
	PushEvents            bool   `json:"push_events"`
	IssuesEvents          bool   `json:"issues_events"`
	MergeRequestsEvents   bool   `json:"merge_requests_events"`
	TagPushEvents         bool   `json:"tag_push_events"`
	NoteEvents            bool   `json:"note_events"`
	JobEvents             bool   `json:"job_events"`
	PipelineEvents        bool   `json:"pipeline_events"`
	EnableSSLVerification bool   `json:"enable_ssl_verification"`
}

// GitHub API response structures
type GitHubRepository struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
	Private bool `json:"private"`
}

type GitHubHook struct {
	ID     int      `json:"id"`
	Name   string   `json:"name"`
	Active bool     `json:"active"`
	Events []string `json:"events"`
	Config struct {
		URL         string `json:"url"`
		ContentType string `json:"content_type"`
		Secret      string `json:"secret,omitempty"`
		InsecureSSL string `json:"insecure_ssl"`
	} `json:"config"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type GitHubWebhookPayload struct {
	Name   string   `json:"name"`
	Active bool     `json:"active"`
	Events []string `json:"events"`
	Config struct {
		URL         string `json:"url"`
		ContentType string `json:"content_type"`
		Secret      string `json:"secret,omitempty"`
		InsecureSSL string `json:"insecure_ssl"`
	} `json:"config"`
}

// Gitea API response structures
type GiteaHook struct {
	ID     int64    `json:"id"`
	Type   string   `json:"type"`
	Active bool     `json:"active"`
	Events []string `json:"events"`
	Config struct {
		URL         string `json:"url"`
		ContentType string `json:"content_type"`
		Secret      string `json:"secret"`
		InsecureSSL bool   `json:"insecure_ssl"`
	} `json:"config"`
}

// WebhookInstallJobArgs represents the arguments for a webhook installation job
type WebhookInstallJobArgs struct {
	ConnectorID int    `json:"connector_id"`
	ProjectPath string `json:"project_path"`
	Provider    string `json:"provider"`
	BaseURL     string `json:"base_url"`
	PAT         string `json:"pat"`
}

// Kind returns the job kind for River
func (WebhookInstallJobArgs) Kind() string {
	return "webhook_install"
}

// WebhookInstallWorker handles webhook installation jobs
type WebhookInstallWorker struct {
	river.WorkerDefaults[WebhookInstallJobArgs]
	pool   *pgxpool.Pool
	config *QueueConfig
}

// WebhookRemovalJobArgs represents the arguments for a webhook removal job
type WebhookRemovalJobArgs struct {
	ConnectorID int    `json:"connector_id"`
	ProjectPath string `json:"project_path"`
	Provider    string `json:"provider"`
	BaseURL     string `json:"base_url"`
	PAT         string `json:"pat"`
}

// Kind returns the job kind for River
func (WebhookRemovalJobArgs) Kind() string {
	return "webhook_removal"
}

// WebhookRemovalWorker handles webhook removal jobs
type WebhookRemovalWorker struct {
	river.WorkerDefaults[WebhookRemovalJobArgs]
	pool   *pgxpool.Pool
	config *QueueConfig
}

// getWebhookEndpointForProvider returns the correct webhook endpoint based on the provider
func (w *WebhookInstallWorker) getWebhookEndpointForProvider(provider string) string {
	baseURL := w.config.WebhookConfig.PublicEndpoint

	// Remove any trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Replace provider-specific endpoint
	switch provider {
	case "gitlab", "gitlab-com", "gitlab-enterprise":
		// Replace any existing endpoint with gitlab-hook
		if strings.HasSuffix(baseURL, "/github-hook") {
			baseURL = strings.TrimSuffix(baseURL, "/github-hook")
		} else if strings.HasSuffix(baseURL, "/gitlab-hook") {
			// Already correct
			return baseURL
		}
		return baseURL + "/gitlab-hook"
	case "github", "github-com", "github-enterprise":
		// Replace any existing endpoint with github-hook
		if strings.HasSuffix(baseURL, "/gitlab-hook") {
			baseURL = strings.TrimSuffix(baseURL, "/gitlab-hook")
		} else if strings.HasSuffix(baseURL, "/github-hook") {
			// Already correct
			return baseURL
		}
		return baseURL + "/github-hook"
	case "bitbucket", "bitbucket-cloud":
		// Replace any existing endpoint with bitbucket-hook
		if strings.HasSuffix(baseURL, "/gitlab-hook") {
			baseURL = strings.TrimSuffix(baseURL, "/gitlab-hook")
		} else if strings.HasSuffix(baseURL, "/github-hook") {
			baseURL = strings.TrimSuffix(baseURL, "/github-hook")
		} else if strings.HasSuffix(baseURL, "/bitbucket-hook") {
			// Already correct
			return baseURL
		}
		return baseURL + "/bitbucket-hook"
	case "gitea":
		if strings.HasSuffix(baseURL, "/gitlab-hook") {
			baseURL = strings.TrimSuffix(baseURL, "/gitlab-hook")
		} else if strings.HasSuffix(baseURL, "/github-hook") {
			baseURL = strings.TrimSuffix(baseURL, "/github-hook")
		} else if strings.HasSuffix(baseURL, "/bitbucket-hook") {
			baseURL = strings.TrimSuffix(baseURL, "/bitbucket-hook")
		} else if strings.HasSuffix(baseURL, "/gitea-hook") {
			return baseURL
		}
		return baseURL + "/gitea-hook"
	default:
		// For unknown providers, try to strip known endpoints and return base
		if strings.HasSuffix(baseURL, "/gitlab-hook") {
			baseURL = strings.TrimSuffix(baseURL, "/gitlab-hook")
		} else if strings.HasSuffix(baseURL, "/github-hook") {
			baseURL = strings.TrimSuffix(baseURL, "/github-hook")
		} else if strings.HasSuffix(baseURL, "/bitbucket-hook") {
			baseURL = strings.TrimSuffix(baseURL, "/bitbucket-hook")
		}
		return baseURL
	}
}

// getWebhookEndpointForProviderWithCustomEndpoint builds provider-specific endpoint with custom base URL and connector_id
// New URL format: {baseURL}/api/v1/{provider}-hook/{connector_id}
// This enables org context derivation via middleware that extracts connector_id from URL path
func (w *WebhookInstallWorker) getWebhookEndpointForProviderWithCustomEndpoint(provider, customEndpoint string, connectorID int) string {
	// Remove any trailing slash from base URL
	baseURL := strings.TrimSuffix(customEndpoint, "/")

	// Build connector-scoped webhook URL with provider-specific path
	var providerPath string
	switch provider {
	case "gitlab", "gitlab-com", "gitlab-enterprise":
		providerPath = "/api/v1/gitlab-hook"
	case "github", "github-com", "github-enterprise":
		providerPath = "/api/v1/github-hook"
	case "bitbucket", "bitbucket-cloud":
		providerPath = "/api/v1/bitbucket-hook"
	case "gitea":
		providerPath = "/api/v1/gitea-hook"
	default:
		// Fallback to generic webhook endpoint
		providerPath = "/api/v1/webhook"
	}

	// Append connector_id to URL for org context derivation
	return fmt.Sprintf("%s%s/%d", baseURL, providerPath, connectorID)
}

// GitLab API client methods
func (w *WebhookInstallWorker) makeGitLabRequest(method, endpoint string, payload interface{}, baseURL, pat string) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
		log.Printf("DEBUG: Request payload: %s", string(jsonData))
	}

	fullURL := baseURL + endpoint
	log.Printf("DEBUG: Making %s request to: %s", method, fullURL)

	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("PRIVATE-TOKEN", pat)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	log.Printf("DEBUG: Request headers: %v", req.Header)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("ERROR: HTTP request failed: %v", err)
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	log.Printf("DEBUG: HTTP response status: %d", resp.StatusCode)
	return resp, nil
}

func (w *WebhookInstallWorker) getProjectID(projectPath, baseURL, pat string) (int, error) {
	log.Printf("DEBUG: Getting project ID for path: %s", projectPath)

	// If it's already a numeric ID, return it
	if id, err := strconv.Atoi(projectPath); err == nil {
		log.Printf("DEBUG: Project path is already numeric ID: %d", id)
		return id, nil
	}

	// Otherwise, resolve the path to a numeric ID
	encodedPath := url.QueryEscape(projectPath)
	endpoint := "/api/v4/projects/" + encodedPath
	log.Printf("DEBUG: Making GitLab API request to: %s%s", baseURL, endpoint)

	resp, err := w.makeGitLabRequest("GET", endpoint, nil, baseURL, pat)
	if err != nil {
		log.Printf("ERROR: Failed to make GitLab API request: %v", err)
		return 0, fmt.Errorf("failed to get project: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body for debugging
	body, bodyErr := io.ReadAll(resp.Body)
	if bodyErr != nil {
		log.Printf("ERROR: Failed to read response body: %v", bodyErr)
		return 0, fmt.Errorf("failed to read response body: %w", bodyErr)
	}

	log.Printf("DEBUG: GitLab API response status: %d", resp.StatusCode)
	log.Printf("DEBUG: GitLab API response body: %s", string(body))

	if resp.StatusCode != http.StatusOK {
		log.Printf("ERROR: GitLab API returned status %d: %s", resp.StatusCode, string(body))
		return 0, fmt.Errorf("GitLab API error (status %d): %s", resp.StatusCode, string(body))
	}

	var project GitLabProject
	if err := json.Unmarshal(body, &project); err != nil {
		log.Printf("ERROR: Failed to decode project response: %v", err)
		return 0, fmt.Errorf("failed to decode project response: %w", err)
	}

	log.Printf("DEBUG: Successfully resolved project %s to ID: %d", projectPath, project.ID)
	return project.ID, nil
}

func (w *WebhookInstallWorker) webhookExists(projectID int, webhookURL, baseURL, pat string) (*GitLabHook, error) {
	resp, err := w.makeGitLabRequest("GET", fmt.Sprintf("/api/v4/projects/%d/hooks", projectID), nil, baseURL, pat)
	if err != nil {
		return nil, fmt.Errorf("failed to list webhooks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitLab API error (status %d): %s", resp.StatusCode, string(body))
	}

	var hooks []GitLabHook
	if err := json.NewDecoder(resp.Body).Decode(&hooks); err != nil {
		return nil, fmt.Errorf("failed to decode hooks response: %w", err)
	}

	for _, hook := range hooks {
		if hook.URL == webhookURL {
			return &hook, nil
		}
	}

	return nil, nil // Not found
}

func (w *WebhookInstallWorker) installGitLabWebhook(projectID int, baseURL, pat string, connectorID int) (*GitLabHook, error) {
	// Get the current production URL from database (don't use cached config)
	db, err := sql.Open("postgres", w.pool.Config().ConnString())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	currentEndpoint, err := getWebhookPublicEndpoint(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get current webhook endpoint: %w", err)
	}
	if currentEndpoint == "" {
		return nil, fmt.Errorf("webhook endpoint not configured: please set livereview_prod_url in settings before installing webhooks")
	}

	// Get provider-specific webhook endpoint with connector_id for org scoping
	webhookURL := w.getWebhookEndpointForProviderWithCustomEndpoint("gitlab", currentEndpoint, connectorID)

	// Check if webhook already exists
	existingHook, err := w.webhookExists(projectID, webhookURL, baseURL, pat)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing webhooks: %w", err)
	}

	payload := WebhookPayload{
		URL:                   webhookURL,
		Token:                 w.config.WebhookConfig.Secret,
		PushEvents:            w.config.WebhookConfig.Events.PushEvents,
		IssuesEvents:          w.config.WebhookConfig.Events.IssuesEvents,
		MergeRequestsEvents:   w.config.WebhookConfig.Events.MergeRequestsEvents,
		TagPushEvents:         w.config.WebhookConfig.Events.TagPushEvents,
		NoteEvents:            w.config.WebhookConfig.Events.NoteEvents,
		JobEvents:             w.config.WebhookConfig.Events.JobEvents,
		PipelineEvents:        w.config.WebhookConfig.Events.PipelineEvents,
		EnableSSLVerification: w.config.WebhookConfig.EnableSSL,
	}

	if existingHook != nil {
		// Update existing webhook
		resp, err := w.makeGitLabRequest("PUT", fmt.Sprintf("/api/v4/projects/%d/hooks/%d", projectID, existingHook.ID), payload, baseURL, pat)
		if err != nil {
			return nil, fmt.Errorf("failed to update webhook: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("GitLab API error updating webhook (status %d): %s", resp.StatusCode, string(body))
		}

		var updatedHook GitLabHook
		if err := json.NewDecoder(resp.Body).Decode(&updatedHook); err != nil {
			return nil, fmt.Errorf("failed to decode updated webhook response: %w", err)
		}

		log.Printf("Updated existing webhook #%d for project %d", updatedHook.ID, projectID)
		return &updatedHook, nil
	} else {
		// Create new webhook
		resp, err := w.makeGitLabRequest("POST", fmt.Sprintf("/api/v4/projects/%d/hooks", projectID), payload, baseURL, pat)
		if err != nil {
			return nil, fmt.Errorf("failed to create webhook: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("GitLab API error creating webhook (status %d): %s", resp.StatusCode, string(body))
		}

		var newHook GitLabHook
		if err := json.NewDecoder(resp.Body).Decode(&newHook); err != nil {
			return nil, fmt.Errorf("failed to decode new webhook response: %w", err)
		}

		log.Printf("Created new webhook #%d for project %d", newHook.ID, projectID)
		return &newHook, nil
	}
}

// Work performs the webhook installation
func (w *WebhookInstallWorker) Work(ctx context.Context, job *river.Job[WebhookInstallJobArgs]) error {
	args := job.Args

	log.Printf("Processing webhook installation for project: %s (connector: %d, provider: %s)",
		args.ProjectPath, args.ConnectorID, args.Provider)

	// Handle different providers
	if strings.HasPrefix(args.Provider, "gitlab") {
		return w.handleGitLabWebhookInstall(ctx, args)
	} else if strings.HasPrefix(args.Provider, "github") {
		return w.handleGitHubWebhookInstall(ctx, args)
	} else if strings.HasPrefix(args.Provider, "bitbucket") {
		return w.handleBitbucketWebhookInstall(ctx, args)
	} else if strings.HasPrefix(args.Provider, "gitea") {
		return w.handleGiteaWebhookInstall(ctx, args)
	} else {
		return fmt.Errorf("unsupported provider: %s", args.Provider)
	}
}

// handleGitLabWebhookInstall handles GitLab webhook installation
func (w *WebhookInstallWorker) handleGitLabWebhookInstall(ctx context.Context, args WebhookInstallJobArgs) error {
	// Get the numeric project ID from GitLab
	projectID, err := w.getProjectID(args.ProjectPath, args.BaseURL, args.PAT)
	if err != nil {
		log.Printf("Failed to get project ID for %s: %v", args.ProjectPath, err)
		return fmt.Errorf("failed to get project ID: %w", err)
	}

	log.Printf("Resolved project %s to ID: %d", args.ProjectPath, projectID)

	// Install the webhook in GitLab with connector_id for org scoping
	webhook, err := w.installGitLabWebhook(projectID, args.BaseURL, args.PAT, args.ConnectorID)
	if err != nil {
		log.Printf("Failed to install webhook for project %s (ID: %d): %v", args.ProjectPath, projectID, err)
		return fmt.Errorf("failed to install webhook: %w", err)
	}

	log.Printf("Successfully installed GitLab webhook #%d for project %s", webhook.ID, args.ProjectPath)

	// Update the webhook_registry with the actual webhook details
	err = w.updateWebhookRegistryGitLab(ctx, args, webhook)
	if err != nil {
		log.Printf("Failed to update webhook registry for project %s: %v", args.ProjectPath, err)
		// Don't return error here since webhook was successfully installed
		// Just log the issue
	}

	log.Printf("GitLab webhook installation completed for project: %s", args.ProjectPath)
	return nil
}

// handleGitHubWebhookInstall handles GitHub webhook installation
func (w *WebhookInstallWorker) handleGitHubWebhookInstall(ctx context.Context, args WebhookInstallJobArgs) error {
	// Parse GitHub repository from project path (format: owner/repo)
	parts := strings.SplitN(args.ProjectPath, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid GitHub repository format: %s (expected: owner/repo)", args.ProjectPath)
	}
	owner, repo := parts[0], parts[1]

	log.Printf("Installing GitHub webhook for repository: %s/%s", owner, repo)

	// Install the webhook in GitHub with connector_id for org scoping
	webhook, err := w.installGitHubWebhook(owner, repo, args.BaseURL, args.PAT, args.ConnectorID)
	if err != nil {
		log.Printf("Failed to install webhook for GitHub repository %s/%s: %v", owner, repo, err)
		return fmt.Errorf("failed to install webhook: %w", err)
	}

	log.Printf("Successfully installed GitHub webhook #%d for repository %s/%s", webhook.ID, owner, repo)

	// Update the webhook_registry with the actual webhook details
	err = w.updateWebhookRegistryGitHub(ctx, args, webhook)
	if err != nil {
		log.Printf("Failed to update webhook registry for GitHub repository %s/%s: %v", owner, repo, err)
		// Don't return error here since webhook was successfully installed
		// Just log the issue
	}

	log.Printf("GitHub webhook installation completed for repository: %s/%s", owner, repo)
	return nil
}

// updateWebhookRegistry creates or updates the webhook registry entry for a project
func (w *WebhookInstallWorker) updateWebhookRegistryGitLab(ctx context.Context, args WebhookInstallJobArgs, webhook *GitLabHook) error {
	// First check if a record already exists
	var existingID int
	checkQuery := `
		SELECT id FROM webhook_registry 
		WHERE integration_token_id = $1 AND project_full_name = $2
	`
	err := w.pool.QueryRow(ctx, checkQuery, args.ConnectorID, args.ProjectPath).Scan(&existingID)

	now := time.Now()

	// Extract project name from full path (last part after /)
	projectName := args.ProjectPath
	if lastSlash := len(args.ProjectPath) - 1; lastSlash >= 0 {
		for i := lastSlash; i >= 0; i-- {
			if args.ProjectPath[i] == '/' {
				projectName = args.ProjectPath[i+1:]
				break
			}
		}
	}

	// Prepare webhook details
	webhookID := ""
	webhookURL := "" // Will be populated from webhook object if available
	webhookName := "LiveReview Webhook"
	events := "merge_requests,notes"
	status := "automatic" // Changed from "manual" since we actually installed a webhook

	if webhook != nil {
		webhookID = fmt.Sprintf("%d", webhook.ID)
		webhookURL = webhook.URL // Use actual URL from GitLab's response
	}

	if err == pgx.ErrNoRows {
		// No existing record, create a new one
		insertQuery := `
			INSERT INTO webhook_registry (
				provider,
				provider_project_id,
				project_name,
				project_full_name,
				webhook_id,
				webhook_url,
				webhook_secret,
				webhook_name,
				events,
				status,
				last_verified_at,
				created_at,
				updated_at,
				integration_token_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		`

		_, err = w.pool.Exec(ctx, insertQuery,
			args.Provider,                 // provider
			args.ProjectPath,              // provider_project_id (using project path as ID)
			projectName,                   // project_name
			args.ProjectPath,              // project_full_name
			webhookID,                     // webhook_id
			webhookURL,                    // webhook_url
			w.config.WebhookConfig.Secret, // webhook_secret
			webhookName,                   // webhook_name
			events,                        // events
			status,                        // status
			now,                           // last_verified_at
			now,                           // created_at
			now,                           // updated_at
			args.ConnectorID,              // integration_token_id
		)

		if err != nil {
			return fmt.Errorf("failed to insert webhook registry: %w", err)
		}

		log.Printf("Created webhook_registry entry for project %s with status '%s' and webhook ID %s", args.ProjectPath, status, webhookID)
	} else if err != nil {
		// Some other error occurred
		return fmt.Errorf("failed to check existing webhook registry: %w", err)
	} else {
		// Record exists, update it
		updateQuery := `
			UPDATE webhook_registry 
			SET status = $1, 
			    updated_at = $2,
			    last_verified_at = $3,
			    webhook_name = $4,
			    events = $5,
			    webhook_id = $6,
			    webhook_url = $7,
			    webhook_secret = $8
			WHERE id = $9
		`

		_, err = w.pool.Exec(ctx, updateQuery,
			status,                        // status
			now,                           // updated_at
			now,                           // last_verified_at
			webhookName,                   // webhook_name
			events,                        // events
			webhookID,                     // webhook_id
			webhookURL,                    // webhook_url
			w.config.WebhookConfig.Secret, // webhook_secret
			existingID,                    // id
		)

		if err != nil {
			return fmt.Errorf("failed to update webhook registry: %w", err)
		}

		log.Printf("Updated webhook_registry entry for project %s with status '%s' and webhook ID %s", args.ProjectPath, status, webhookID)
	}

	return nil
}

// GitHub webhook installation methods

// makeGitHubRequest makes a request to the GitHub API
func (w *WebhookInstallWorker) makeGitHubRequest(method, endpoint string, payload interface{}, baseURL, pat string) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
		log.Printf("DEBUG: GitHub request payload: %s", string(jsonData))
	}

	// Determine API base URL
	apiBaseURL := "https://api.github.com"
	if baseURL != "" && baseURL != "https://github.com" {
		// For GitHub Enterprise, the API is typically at /api/v3
		apiBaseURL = strings.TrimSuffix(baseURL, "/") + "/api/v3"
	}

	fullURL := apiBaseURL + endpoint
	log.Printf("DEBUG: Making %s request to: %s", method, fullURL)

	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+pat)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "LiveReview/1.0")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	log.Printf("DEBUG: Request headers: %v", req.Header)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("ERROR: HTTP request failed: %v", err)
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	log.Printf("DEBUG: HTTP response status: %d", resp.StatusCode)
	return resp, nil
}

// gitHubWebhookExists checks if a webhook already exists for the repository
func (w *WebhookInstallWorker) gitHubWebhookExists(owner, repo, webhookURL, baseURL, pat string) (*GitHubHook, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/hooks", owner, repo)
	resp, err := w.makeGitHubRequest("GET", endpoint, nil, baseURL, pat)
	if err != nil {
		return nil, fmt.Errorf("failed to list webhooks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}

	var hooks []GitHubHook
	if err := json.NewDecoder(resp.Body).Decode(&hooks); err != nil {
		return nil, fmt.Errorf("failed to decode hooks response: %w", err)
	}

	for _, hook := range hooks {
		if hook.Config.URL == webhookURL {
			return &hook, nil
		}
	}

	return nil, nil // Not found
}

// installGitHubWebhook installs a webhook in GitHub repository
func (w *WebhookInstallWorker) installGitHubWebhook(owner, repo, baseURL, pat string, connectorID int) (*GitHubHook, error) {
	// Get the current production URL from database (don't use cached config)
	db, err := sql.Open("postgres", w.pool.Config().ConnString())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	currentEndpoint, err := getWebhookPublicEndpoint(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get current webhook endpoint: %w", err)
	}
	if currentEndpoint == "" {
		return nil, fmt.Errorf("webhook endpoint not configured: please set livereview_prod_url in settings before installing webhooks")
	}

	// Get provider-specific webhook endpoint with connector_id for org scoping
	webhookURL := w.getWebhookEndpointForProviderWithCustomEndpoint("github", currentEndpoint, connectorID)

	// Check if webhook already exists
	existingHook, err := w.gitHubWebhookExists(owner, repo, webhookURL, baseURL, pat)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing webhooks: %w", err)
	}

	// Define GitHub events we want to subscribe to
	events := []string{
		"pull_request",
		"pull_request_review",
		"pull_request_review_comment",
		"issue_comment",
	}

	payload := GitHubWebhookPayload{
		Name:   "web",
		Active: true,
		Events: events,
		Config: struct {
			URL         string `json:"url"`
			ContentType string `json:"content_type"`
			Secret      string `json:"secret,omitempty"`
			InsecureSSL string `json:"insecure_ssl"`
		}{
			URL:         webhookURL,
			ContentType: "json",
			Secret:      w.config.WebhookConfig.Secret,
			InsecureSSL: func() string {
				if w.config.WebhookConfig.EnableSSL {
					return "0"
				}
				return "1"
			}(),
		},
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/hooks", owner, repo)

	if existingHook != nil {
		// Update existing webhook
		endpoint = fmt.Sprintf("/repos/%s/%s/hooks/%d", owner, repo, existingHook.ID)
		resp, err := w.makeGitHubRequest("PATCH", endpoint, payload, baseURL, pat)
		if err != nil {
			return nil, fmt.Errorf("failed to update webhook: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("GitHub API error updating webhook (status %d): %s", resp.StatusCode, string(body))
		}

		var updatedHook GitHubHook
		if err := json.NewDecoder(resp.Body).Decode(&updatedHook); err != nil {
			return nil, fmt.Errorf("failed to decode updated webhook response: %w", err)
		}

		log.Printf("Updated existing GitHub webhook #%d for repository %s/%s", updatedHook.ID, owner, repo)
		return &updatedHook, nil
	} else {
		// Create new webhook
		resp, err := w.makeGitHubRequest("POST", endpoint, payload, baseURL, pat)
		if err != nil {
			return nil, fmt.Errorf("failed to create webhook: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("GitHub API error creating webhook (status %d): %s", resp.StatusCode, string(body))
		}

		var newHook GitHubHook
		if err := json.NewDecoder(resp.Body).Decode(&newHook); err != nil {
			return nil, fmt.Errorf("failed to decode new webhook response: %w", err)
		}

		log.Printf("Created new GitHub webhook #%d for repository %s/%s", newHook.ID, owner, repo)
		return &newHook, nil
	}
}

// updateWebhookRegistryGitHub creates or updates the webhook registry entry for a GitHub repository
func (w *WebhookInstallWorker) updateWebhookRegistryGitHub(ctx context.Context, args WebhookInstallJobArgs, webhook *GitHubHook) error {
	// First check if a record already exists
	var existingID int
	checkQuery := `
		SELECT id FROM webhook_registry 
		WHERE integration_token_id = $1 AND project_full_name = $2
	`
	err := w.pool.QueryRow(ctx, checkQuery, args.ConnectorID, args.ProjectPath).Scan(&existingID)

	now := time.Now()

	// Extract project name from full path (last part after /)
	projectName := args.ProjectPath
	if lastSlash := strings.LastIndex(args.ProjectPath, "/"); lastSlash >= 0 {
		projectName = args.ProjectPath[lastSlash+1:]
	}

	// Prepare webhook details
	webhookID := ""
	webhookURL := "" // Will be populated from webhook object if available
	webhookName := "LiveReview Webhook"
	events := "pull_request,issue_comment"
	status := "manual" // GitHub webhooks enable manual trigger

	if webhook != nil {
		webhookID = fmt.Sprintf("%d", webhook.ID)
		webhookURL = webhook.Config.URL // Use actual URL from GitHub's response
	}

	if err == pgx.ErrNoRows {
		// No existing record, create a new one
		insertQuery := `
			INSERT INTO webhook_registry (
				provider,
				provider_project_id,
				project_name,
				project_full_name,
				webhook_id,
				webhook_url,
				webhook_secret,
				webhook_name,
				events,
				status,
				last_verified_at,
				created_at,
				updated_at,
				integration_token_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		`

		_, err = w.pool.Exec(ctx, insertQuery,
			args.Provider,                 // provider
			args.ProjectPath,              // provider_project_id (using project path as ID)
			projectName,                   // project_name
			args.ProjectPath,              // project_full_name
			webhookID,                     // webhook_id
			webhookURL,                    // webhook_url
			w.config.WebhookConfig.Secret, // webhook_secret
			webhookName,                   // webhook_name
			events,                        // events
			status,                        // status
			now,                           // last_verified_at
			now,                           // created_at
			now,                           // updated_at
			args.ConnectorID,              // integration_token_id
		)

		if err != nil {
			return fmt.Errorf("failed to insert webhook registry: %w", err)
		}

		log.Printf("Created webhook_registry entry for GitHub repository %s with status '%s' and webhook ID %s", args.ProjectPath, status, webhookID)
	} else if err != nil {
		// Some other error occurred
		return fmt.Errorf("failed to check existing webhook registry: %w", err)
	} else {
		// Record exists, update it
		updateQuery := `
			UPDATE webhook_registry 
			SET status = $1, 
			    updated_at = $2,
			    last_verified_at = $3,
			    webhook_name = $4,
			    events = $5,
			    webhook_id = $6,
			    webhook_url = $7,
			    webhook_secret = $8
			WHERE id = $9
		`

		_, err = w.pool.Exec(ctx, updateQuery,
			status,                        // status
			now,                           // updated_at
			now,                           // last_verified_at
			webhookName,                   // webhook_name
			events,                        // events
			webhookID,                     // webhook_id
			webhookURL,                    // webhook_url
			w.config.WebhookConfig.Secret, // webhook_secret
			existingID,                    // id
		)

		if err != nil {
			return fmt.Errorf("failed to update webhook registry: %w", err)
		}

		log.Printf("Updated webhook_registry entry for GitHub repository %s with status '%s' and webhook ID %s", args.ProjectPath, status, webhookID)
	}

	return nil
}

// Bitbucket webhook installation methods

// BitbucketHook represents a Bitbucket webhook
type BitbucketHook struct {
	UUID        string   `json:"uuid"`
	URL         string   `json:"url"`
	Description string   `json:"description"`
	Active      bool     `json:"active"`
	Events      []string `json:"events"`
}

// BitbucketWebhookPayload represents the payload for creating/updating Bitbucket webhooks
type BitbucketWebhookPayload struct {
	URL         string   `json:"url"`
	Description string   `json:"description"`
	Active      bool     `json:"active"`
	Events      []string `json:"events"`
}

// handleBitbucketWebhookInstall handles Bitbucket webhook installation
func (w *WebhookInstallWorker) handleBitbucketWebhookInstall(ctx context.Context, args WebhookInstallJobArgs) error {
	// Parse Bitbucket repository from project path (format: workspace/repo)
	parts := strings.SplitN(args.ProjectPath, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid Bitbucket repository format: %s (expected: workspace/repo)", args.ProjectPath)
	}
	workspace, repo := parts[0], parts[1]

	log.Printf("Installing Bitbucket webhook for repository: %s/%s", workspace, repo)

	// Get email from connector metadata (needed for Bitbucket authentication)
	email, err := w.getBitbucketEmailFromConnector(args.ConnectorID)
	if err != nil {
		return fmt.Errorf("failed to get Bitbucket email for connector %d: %w", args.ConnectorID, err)
	}

	// Install the webhook in Bitbucket with connector_id for org scoping
	webhook, err := w.installBitbucketWebhook(workspace, repo, email, args.PAT, args.ConnectorID)
	if err != nil {
		log.Printf("Failed to install webhook for Bitbucket repository %s/%s: %v", workspace, repo, err)
		return fmt.Errorf("failed to install webhook: %w", err)
	}

	log.Printf("Successfully installed Bitbucket webhook %s for repository %s/%s", webhook.UUID, workspace, repo)

	// Update the webhook_registry with the actual webhook details
	err = w.updateWebhookRegistryBitbucket(ctx, args, webhook)
	if err != nil {
		log.Printf("Failed to update webhook registry for Bitbucket repository %s/%s: %v", workspace, repo, err)
		// Don't return error here since webhook was successfully installed
		// Just log the issue
	}

	log.Printf("Bitbucket webhook installation completed for repository: %s/%s", workspace, repo)
	return nil
}

// getBitbucketEmailFromConnector retrieves the email from connector metadata
func (w *WebhookInstallWorker) getBitbucketEmailFromConnector(connectorID int) (string, error) {
	var metadataBytes []byte
	query := `SELECT COALESCE(metadata, '{}') FROM integration_tokens WHERE id = $1`

	err := w.pool.QueryRow(context.Background(), query, connectorID).Scan(&metadataBytes)
	if err != nil {
		return "", fmt.Errorf("failed to get connector metadata: %w", err)
	}

	var metadata map[string]interface{}
	if len(metadataBytes) > 0 {
		if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
			return "", fmt.Errorf("failed to parse metadata: %w", err)
		}
	}

	email, ok := metadata["email"].(string)
	if !ok || email == "" {
		return "", fmt.Errorf("email not found in connector metadata")
	}

	return email, nil
}

// makeBitbucketRequest makes a request to the Bitbucket API
func (w *WebhookInstallWorker) makeBitbucketRequest(method, endpoint string, payload interface{}, email, apiToken string) (*http.Response, error) {
	var reqBody io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	// Always use Bitbucket Cloud API
	url := "https://api.bitbucket.org/2.0" + endpoint
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set Basic Auth with email and API token
	req.SetBasicAuth(email, apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LiveReview/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

// bitbucketWebhookExists checks if a webhook already exists for the repository
func (w *WebhookInstallWorker) bitbucketWebhookExists(workspace, repo, webhookURL, email, apiToken string) (*BitbucketHook, error) {
	endpoint := fmt.Sprintf("/repositories/%s/%s/hooks", workspace, repo)
	resp, err := w.makeBitbucketRequest("GET", endpoint, nil, email, apiToken)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch webhooks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bitbucket API error (status %d): %s", resp.StatusCode, string(body))
	}

	var response struct {
		Values []BitbucketHook `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode webhook list: %w", err)
	}

	// Look for existing LiveReview webhook
	for _, hook := range response.Values {
		if hook.URL == webhookURL && strings.Contains(hook.Description, "LiveReview") {
			return &hook, nil
		}
	}

	return nil, nil
}

// installBitbucketWebhook installs a webhook in Bitbucket repository
func (w *WebhookInstallWorker) installBitbucketWebhook(workspace, repo, email, apiToken string, connectorID int) (*BitbucketHook, error) {
	// Get the current production URL from database (don't use cached config)
	db, err := sql.Open("postgres", w.pool.Config().ConnString())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	currentEndpoint, err := getWebhookPublicEndpoint(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get current webhook endpoint: %w", err)
	}
	if currentEndpoint == "" {
		return nil, fmt.Errorf("webhook endpoint not configured: please set livereview_prod_url in settings before installing webhooks")
	}

	// Get provider-specific webhook endpoint with connector_id for org scoping
	webhookURL := w.getWebhookEndpointForProviderWithCustomEndpoint("bitbucket", currentEndpoint, connectorID)

	// Check if webhook already exists
	existingHook, err := w.bitbucketWebhookExists(workspace, repo, webhookURL, email, apiToken)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing webhooks: %w", err)
	}

	// Define Bitbucket events we want to subscribe to
	events := []string{
		"pullrequest:created",
		"pullrequest:updated",
		"pullrequest:approved",
		"pullrequest:unapproved",
		"pullrequest:comment_created",
		"pullrequest:comment_updated",
	}

	payload := BitbucketWebhookPayload{
		URL:         webhookURL,
		Description: "LiveReview Webhook - Automated code review",
		Active:      true,
		Events:      events,
	}

	if existingHook != nil {
		// Update existing webhook
		endpoint := fmt.Sprintf("/repositories/%s/%s/hooks/%s", workspace, repo, existingHook.UUID)
		resp, err := w.makeBitbucketRequest("PUT", endpoint, payload, email, apiToken)
		if err != nil {
			return nil, fmt.Errorf("failed to update webhook: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("bitbucket API error updating webhook (status %d): %s", resp.StatusCode, string(body))
		}

		var updatedHook BitbucketHook
		if err := json.NewDecoder(resp.Body).Decode(&updatedHook); err != nil {
			return nil, fmt.Errorf("failed to decode updated webhook response: %w", err)
		}

		log.Printf("Updated existing Bitbucket webhook %s for repository %s/%s", updatedHook.UUID, workspace, repo)
		return &updatedHook, nil
	} else {
		// Create new webhook
		endpoint := fmt.Sprintf("/repositories/%s/%s/hooks", workspace, repo)
		resp, err := w.makeBitbucketRequest("POST", endpoint, payload, email, apiToken)
		if err != nil {
			return nil, fmt.Errorf("failed to create webhook: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("bitbucket API error creating webhook (status %d): %s", resp.StatusCode, string(body))
		}

		var newHook BitbucketHook
		if err := json.NewDecoder(resp.Body).Decode(&newHook); err != nil {
			return nil, fmt.Errorf("failed to decode new webhook response: %w", err)
		}

		log.Printf("Created new Bitbucket webhook %s for repository %s/%s", newHook.UUID, workspace, repo)
		return &newHook, nil
	}
}

// updateWebhookRegistryBitbucket creates or updates the webhook registry entry for a Bitbucket repository
func (w *WebhookInstallWorker) updateWebhookRegistryBitbucket(ctx context.Context, args WebhookInstallJobArgs, webhook *BitbucketHook) error {
	// First check if a record already exists
	var existingID int
	checkQuery := `
		SELECT id FROM webhook_registry 
		WHERE integration_token_id = $1 AND project_full_name = $2
	`
	err := w.pool.QueryRow(ctx, checkQuery, args.ConnectorID, args.ProjectPath).Scan(&existingID)

	now := time.Now()

	// Extract project name from full path (last part after /)
	projectName := args.ProjectPath
	if lastSlash := strings.LastIndex(args.ProjectPath, "/"); lastSlash >= 0 {
		projectName = args.ProjectPath[lastSlash+1:]
	}

	// Prepare webhook details
	webhookID := ""
	webhookURL := "" // Will be populated from webhook object if available
	webhookName := "LiveReview Webhook"
	events := "pullrequest:created,pullrequest:updated,pullrequest:comment_created"
	status := "manual" // Bitbucket webhooks enable manual trigger

	if webhook != nil {
		webhookID = webhook.UUID
		webhookURL = webhook.URL // Use actual URL from Bitbucket's response
	}

	if err == pgx.ErrNoRows {
		// No existing record, create a new one
		insertQuery := `
			INSERT INTO webhook_registry (
				provider,
				provider_project_id,
				project_name,
				project_full_name,
				webhook_id,
				webhook_url,
				webhook_secret,
				webhook_name,
				events,
				status,
				last_verified_at,
				created_at,
				updated_at,
				integration_token_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		`

		_, err = w.pool.Exec(ctx, insertQuery,
			args.Provider,                 // provider
			args.ProjectPath,              // provider_project_id (using project path as ID)
			projectName,                   // project_name
			args.ProjectPath,              // project_full_name
			webhookID,                     // webhook_id
			webhookURL,                    // webhook_url
			w.config.WebhookConfig.Secret, // webhook_secret
			webhookName,                   // webhook_name
			events,                        // events
			status,                        // status
			now,                           // last_verified_at
			now,                           // created_at
			now,                           // updated_at
			args.ConnectorID,              // integration_token_id
		)

		if err != nil {
			return fmt.Errorf("failed to insert webhook registry: %w", err)
		}

		log.Printf("Created webhook_registry entry for Bitbucket repository %s with status '%s' and webhook ID %s", args.ProjectPath, status, webhookID)
	} else if err != nil {
		// Some other error occurred
		return fmt.Errorf("failed to check existing webhook registry: %w", err)
	} else {
		// Record exists, update it
		updateQuery := `
			UPDATE webhook_registry 
			SET status = $1, 
			    updated_at = $2,
			    last_verified_at = $3,
			    webhook_name = $4,
			    events = $5,
			    webhook_id = $6,
			    webhook_url = $7,
			    webhook_secret = $8
			WHERE id = $9
		`

		_, err = w.pool.Exec(ctx, updateQuery,
			status,                        // status
			now,                           // updated_at
			now,                           // last_verified_at
			webhookName,                   // webhook_name
			events,                        // events
			webhookID,                     // webhook_id
			webhookURL,                    // webhook_url
			w.config.WebhookConfig.Secret, // webhook_secret
			existingID,                    // id
		)

		if err != nil {
			return fmt.Errorf("failed to update webhook registry: %w", err)
		}

		log.Printf("Updated webhook_registry entry for Bitbucket repository %s with status '%s' and webhook ID %s", args.ProjectPath, status, webhookID)
	}

	return nil
}

// Gitea webhook installation methods

// handleGiteaWebhookInstall handles Gitea webhook installation
func (w *WebhookInstallWorker) handleGiteaWebhookInstall(ctx context.Context, args WebhookInstallJobArgs) error {
	parts := strings.SplitN(args.ProjectPath, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid Gitea repository format: %s (expected: owner/repo)", args.ProjectPath)
	}
	owner, repo := parts[0], parts[1]

	// Unpack PAT if it's in packed format
	pat := gitea.UnpackGiteaPAT(args.PAT)

	log.Printf("Installing Gitea webhook for repository: %s/%s", owner, repo)

	webhook, err := w.installGiteaWebhook(owner, repo, args.BaseURL, pat, args.ConnectorID)
	if err != nil {
		log.Printf("Failed to install webhook for Gitea repository %s/%s: %v", owner, repo, err)
		return fmt.Errorf("failed to install webhook: %w", err)
	}

	if webhook != nil {
		log.Printf("Successfully installed Gitea webhook #%d for repository %s/%s", webhook.ID, owner, repo)
	} else {
		log.Printf("Successfully verified existing Gitea webhook for repository %s/%s", owner, repo)
	}

	if err := w.updateWebhookRegistryGitea(ctx, args, webhook); err != nil {
		log.Printf("Failed to update webhook registry for Gitea repository %s/%s: %v", owner, repo, err)
		// Do not fail the job if registry update fails after webhook creation
	}

	log.Printf("Gitea webhook installation completed for repository: %s/%s", owner, repo)
	return nil
}

// makeGiteaRequest makes a request to the Gitea API
func (w *WebhookInstallWorker) makeGiteaRequest(method, endpoint string, payload interface{}, baseURL, pat string) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
	}

	apiBase := gitea.NormalizeGiteaBaseURL(baseURL)
	// Gitea API endpoints always use /api/v1 prefix
	fullURL := fmt.Sprintf("%s/api/v1%s", apiBase, endpoint)

	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+pat)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	return resp, nil
}

// giteaWebhookExists checks if a webhook already exists for the repository
func (w *WebhookInstallWorker) giteaWebhookExists(owner, repo, webhookURL, baseURL, pat string) (*GiteaHook, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/hooks", owner, repo)
	resp, err := w.makeGiteaRequest(http.MethodGet, endpoint, nil, baseURL, pat)
	if err != nil {
		return nil, fmt.Errorf("failed to list webhooks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitea API error (status %d): %s", resp.StatusCode, string(body))
	}

	var hooks []GiteaHook
	if err := json.NewDecoder(resp.Body).Decode(&hooks); err != nil {
		return nil, fmt.Errorf("failed to decode hooks response: %w", err)
	}

	oldWebhookURL := w.getWebhookEndpointForProvider("gitea")
	for _, hook := range hooks {
		if hook.Config.URL == webhookURL || hook.Config.URL == oldWebhookURL {
			return &hook, nil
		}
	}

	return nil, nil
}

// installGiteaWebhook installs or updates a webhook for a Gitea repository
func (w *WebhookInstallWorker) installGiteaWebhook(owner, repo, baseURL, pat string, connectorID int) (*GiteaHook, error) {
	// Get the current production URL from database (do not use cached config)
	db, err := sql.Open("postgres", w.pool.Config().ConnString())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	currentEndpoint, err := getWebhookPublicEndpoint(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get current webhook endpoint: %w", err)
	}
	if currentEndpoint == "" {
		return nil, fmt.Errorf("webhook endpoint not configured: please set livereview_prod_url in settings before installing webhooks")
	}

	webhookURL := w.getWebhookEndpointForProviderWithCustomEndpoint("gitea", currentEndpoint, connectorID)

	// Check if webhook already exists
	existingHook, err := w.giteaWebhookExists(owner, repo, webhookURL, baseURL, pat)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing webhooks: %w", err)
	}

	events := []string{"pull_request", "issue_comment"}
	payload := map[string]interface{}{
		"type": "gitea",
		"config": map[string]interface{}{
			"url":          webhookURL,
			"content_type": "json",
			"secret":       w.config.WebhookConfig.Secret,
		},
		"events": events,
		"active": true,
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/hooks", owner, repo)
	method := http.MethodPost
	if existingHook != nil {
		endpoint = fmt.Sprintf("/repos/%s/%s/hooks/%d", owner, repo, existingHook.ID)
		method = http.MethodPatch
	}

	resp, err := w.makeGiteaRequest(method, endpoint, payload, baseURL, pat)
	if err != nil {
		return nil, fmt.Errorf("failed to install webhook: %w", err)
	}
	defer resp.Body.Close()

	// Gitea returns 200 for update and 201 for create
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitea API error installing webhook (status %d): %s", resp.StatusCode, string(body))
	}

	var hook GiteaHook
	if err := json.NewDecoder(resp.Body).Decode(&hook); err != nil {
		return nil, fmt.Errorf("failed to decode webhook response: %w", err)
	}

	return &hook, nil
}

// updateWebhookRegistryGitea creates or updates the webhook registry entry for a Gitea repository
func (w *WebhookInstallWorker) updateWebhookRegistryGitea(ctx context.Context, args WebhookInstallJobArgs, webhook *GiteaHook) error {
	var existingID int
	checkQuery := `
		SELECT id FROM webhook_registry 
		WHERE integration_token_id = $1 AND project_full_name = $2
	`

	err := w.pool.QueryRow(ctx, checkQuery, args.ConnectorID, args.ProjectPath).Scan(&existingID)
	now := time.Now()

	projectName := args.ProjectPath
	if slash := strings.LastIndex(args.ProjectPath, "/"); slash != -1 {
		projectName = args.ProjectPath[slash+1:]
	}

	webhookID := ""
	webhookURL := ""
	webhookName := "LiveReview Webhook"
	events := "pull_request,issue_comment"
	status := "automatic"

	if webhook != nil {
		webhookID = fmt.Sprintf("%d", webhook.ID)
		webhookURL = webhook.Config.URL
	}

	if err == pgx.ErrNoRows {
		insertQuery := `
			INSERT INTO webhook_registry (
				provider,
				provider_project_id,
				project_name,
				project_full_name,
				webhook_id,
				webhook_url,
				webhook_secret,
				webhook_name,
				events,
				status,
				last_verified_at,
				created_at,
				updated_at,
				integration_token_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		`

		_, err = w.pool.Exec(ctx, insertQuery,
			args.Provider,
			args.ProjectPath,
			projectName,
			args.ProjectPath,
			webhookID,
			webhookURL,
			w.config.WebhookConfig.Secret,
			webhookName,
			events,
			status,
			now,
			now,
			now,
			args.ConnectorID,
		)

		if err != nil {
			return fmt.Errorf("failed to insert webhook registry: %w", err)
		}

		log.Printf("Created webhook_registry entry for Gitea project %s with status '%s'", args.ProjectPath, status)
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check existing webhook registry: %w", err)
	}

	updateQuery := `
		UPDATE webhook_registry 
		SET status = $1,
		    updated_at = $2,
		    last_verified_at = $3,
		    webhook_name = $4,
		    events = $5,
		    webhook_id = $6,
		    webhook_url = $7,
		    webhook_secret = $8
		WHERE id = $9
	`

	_, err = w.pool.Exec(ctx, updateQuery,
		status,
		now,
		now,
		webhookName,
		events,
		webhookID,
		webhookURL,
		w.config.WebhookConfig.Secret,
		existingID,
	)

	if err != nil {
		return fmt.Errorf("failed to update webhook registry: %w", err)
	}

	log.Printf("Updated webhook_registry entry for Gitea project %s with status '%s'", args.ProjectPath, status)
	return nil
}

// WebhookRemovalWorker methods

// Work processes a webhook removal job
func (w *WebhookRemovalWorker) Work(ctx context.Context, job *river.Job[WebhookRemovalJobArgs]) error {
	args := job.Args

	log.Printf("Processing webhook removal for project: %s (connector: %d, provider: %s)",
		args.ProjectPath, args.ConnectorID, args.Provider)

	// Handle different providers
	if strings.HasPrefix(args.Provider, "gitlab") {
		return w.handleGitLabWebhookRemoval(ctx, args)
	} else if strings.HasPrefix(args.Provider, "github") {
		return w.handleGitHubWebhookRemoval(ctx, args)
	} else if strings.HasPrefix(args.Provider, "bitbucket") {
		return w.handleBitbucketWebhookRemoval(ctx, args)
	} else if strings.HasPrefix(args.Provider, "gitea") {
		return w.handleGiteaWebhookRemoval(ctx, args)
	} else {
		return fmt.Errorf("unsupported provider: %s", args.Provider)
	}
}

// handleGitLabWebhookRemoval handles GitLab webhook removal
func (w *WebhookRemovalWorker) handleGitLabWebhookRemoval(ctx context.Context, args WebhookRemovalJobArgs) error {
	// Get the numeric project ID from GitLab
	projectID, err := w.getProjectID(args.ProjectPath, args.BaseURL, args.PAT)
	if err != nil {
		log.Printf("Failed to get project ID for %s: %v", args.ProjectPath, err)
		return fmt.Errorf("failed to get project ID: %w", err)
	}

	log.Printf("Resolved project %s to ID: %d", args.ProjectPath, projectID)

	// Remove webhooks from GitLab
	err = w.removeWebhooks(projectID, args.BaseURL, args.PAT, args.ConnectorID, args.ProjectPath)
	if err != nil {
		log.Printf("Failed to remove webhooks for project %s (ID: %d): %v", args.ProjectPath, projectID, err)
		return fmt.Errorf("failed to remove webhooks: %w", err)
	}

	log.Printf("Successfully removed webhooks for project %s", args.ProjectPath)

	// Update the webhook registry to mark as unconnected
	err = w.updateWebhookRegistryForRemoval(ctx, args, projectID)
	if err != nil {
		log.Printf("Failed to update webhook registry for project %s: %v", args.ProjectPath, err)
		return fmt.Errorf("failed to update webhook registry: %w", err)
	}

	return nil
}

// handleGitHubWebhookRemoval handles GitHub webhook removal
func (w *WebhookRemovalWorker) handleGitHubWebhookRemoval(ctx context.Context, args WebhookRemovalJobArgs) error {
	// Parse GitHub repository from project path (format: owner/repo)
	parts := strings.SplitN(args.ProjectPath, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid GitHub repository format: %s (expected: owner/repo)", args.ProjectPath)
	}
	owner, repo := parts[0], parts[1]

	log.Printf("Removing GitHub webhooks for repository: %s/%s", owner, repo)

	// Remove webhooks from GitHub
	err := w.removeGitHubWebhooks(owner, repo, args.BaseURL, args.PAT, args.ConnectorID)
	if err != nil {
		log.Printf("Failed to remove webhooks for GitHub repository %s/%s: %v", owner, repo, err)
		return fmt.Errorf("failed to remove webhooks: %w", err)
	}

	log.Printf("Successfully removed webhooks for GitHub repository %s/%s", owner, repo)

	// Update the webhook registry to mark as unconnected
	err = w.updateWebhookRegistryForGitHubRemoval(ctx, args)
	if err != nil {
		log.Printf("Failed to update webhook registry for GitHub repository %s/%s: %v", owner, repo, err)
		return fmt.Errorf("failed to update webhook registry: %w", err)
	}

	return nil
}

// getProjectID gets the numeric project ID from GitLab API using project path
func (w *WebhookRemovalWorker) getProjectID(projectPath, baseURL, pat string) (int, error) {
	// URL encode the project path
	encodedPath := url.PathEscape(projectPath)
	endpoint := fmt.Sprintf("/api/v4/projects/%s", encodedPath)

	resp, err := w.makeGitLabRequest("GET", endpoint, nil, baseURL, pat)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch project: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("GitLab API error (status %d): %s", resp.StatusCode, string(body))
	}

	var project GitLabProject
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		return 0, fmt.Errorf("failed to decode project response: %w", err)
	}

	log.Printf("Successfully resolved project path '%s' to ID %d", projectPath, project.ID)
	return project.ID, nil
}

// getWebhookEndpointForProviderWithConnector returns webhook endpoint with connector ID
func (w *WebhookRemovalWorker) getWebhookEndpointForProviderWithConnector(provider string, connectorID int) string {
	baseURL := w.config.WebhookConfig.PublicEndpoint
	baseURL = strings.TrimSuffix(baseURL, "/")

	switch provider {
	case "gitlab", "gitlab-com", "gitlab-enterprise":
		return fmt.Sprintf("%s/api/v1/gitlab-hook/%d", baseURL, connectorID)
	case "github", "github-com", "github-enterprise":
		return fmt.Sprintf("%s/api/v1/github-hook/%d", baseURL, connectorID)
	case "bitbucket", "bitbucket-cloud":
		return fmt.Sprintf("%s/api/v1/bitbucket-hook/%d", baseURL, connectorID)
	case "gitea":
		return fmt.Sprintf("%s/api/v1/gitea-hook/%d", baseURL, connectorID)
	default:
		return fmt.Sprintf("%s/api/v1/%s-hook/%d", baseURL, provider, connectorID)
	}
}

// getWebhookEndpointForProvider returns the correct webhook endpoint based on the provider for removal worker
// DEPRECATED: Use getWebhookEndpointForProviderWithConnector instead
func (w *WebhookRemovalWorker) getWebhookEndpointForProvider(provider string) string {
	baseURL := w.config.WebhookConfig.PublicEndpoint

	// Remove any trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Return old-style webhook URLs (without connector_id) with /api/v1 prefix
	switch provider {
	case "gitlab", "gitlab-com", "gitlab-enterprise":
		return baseURL + "/api/v1/gitlab-hook"
	case "github", "github-com", "github-enterprise":
		return baseURL + "/api/v1/github-hook"
	case "bitbucket", "bitbucket-cloud":
		return baseURL + "/api/v1/bitbucket-hook"
	case "gitea":
		return baseURL + "/api/v1/gitea-hook"
	default:
		return baseURL + "/api/v1/" + provider + "-hook"
	}
}

// makeGitLabRequest makes a request to the GitLab API
func (w *WebhookRemovalWorker) makeGitLabRequest(method, endpoint string, payload interface{}, baseURL, pat string) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request payload: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
		log.Printf("GitLab API %s %s with payload: %s", method, endpoint, string(jsonData))
	} else {
		log.Printf("GitLab API %s %s", method, endpoint)
	}

	req, err := http.NewRequest(method, baseURL+endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+pat)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// removeWebhooks removes all LiveReview webhooks from a GitLab project
func (w *WebhookRemovalWorker) removeWebhooks(projectID int, baseURL, pat string, connectorID int, projectPath string) error {
	// Get existing hooks
	resp, err := w.makeGitLabRequest("GET", fmt.Sprintf("/api/v4/projects/%d/hooks", projectID), nil, baseURL, pat)
	if err != nil {
		return fmt.Errorf("failed to fetch existing hooks: %w", err)
	}
	defer resp.Body.Close()

	// Handle 404 gracefully - project or hooks might already be deleted
	if resp.StatusCode == http.StatusNotFound {
		log.Printf("[INFO] Project %d not found or no access - treating as already removed", projectID)
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitLab API error fetching hooks (status %d): %s", resp.StatusCode, string(body))
	}

	var hooks []GitLabHook
	if err := json.NewDecoder(resp.Body).Decode(&hooks); err != nil {
		return fmt.Errorf("failed to decode hooks response: %w", err)
	}

	// Build both old-style (no connector_id) and new-style (with connector_id) webhook URLs
	webhookURLNew := w.getWebhookEndpointForProviderWithConnector("gitlab", connectorID)
	webhookURLOld := w.getWebhookEndpointForProvider("gitlab")
	removedCount := 0

	// Remove hooks that match either old-style or new-style webhook URLs
	for _, hook := range hooks {
		if hook.URL == webhookURLNew || hook.URL == webhookURLOld {
			log.Printf("Removing webhook #%d with URL: %s", hook.ID, hook.URL)
			resp, err := w.makeGitLabRequest("DELETE", fmt.Sprintf("/api/v4/projects/%d/hooks/%d", projectID, hook.ID), nil, baseURL, pat)
			if err != nil {
				log.Printf("Failed to delete webhook #%d: %v", hook.ID, err)
				continue
			}
			resp.Body.Close()

			// Treat 404 as success - webhook already deleted
			if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
				log.Printf("Deleted webhook #%d for project %d (or already deleted)", hook.ID, projectID)
				removedCount++
			} else {
				log.Printf("Failed to delete webhook #%d (status %d)", hook.ID, resp.StatusCode)
			}
		}
	}

	if removedCount == 0 {
		log.Printf("No LiveReview webhooks found for project %d", projectID)
	} else {
		log.Printf("Removed %d webhooks for project %d", removedCount, projectID)
	}

	return nil
}

// updateWebhookRegistryForRemoval updates the webhook registry to mark project as unconnected
func (w *WebhookRemovalWorker) updateWebhookRegistryForRemoval(ctx context.Context, args WebhookRemovalJobArgs, projectID int) error {
	// Check if entry exists in webhook registry
	checkQuery := `
		SELECT id FROM webhook_registry 
		WHERE integration_token_id = $1 AND project_full_name = $2
	`

	var existingID int
	err := w.pool.QueryRow(ctx, checkQuery, args.ConnectorID, args.ProjectPath).Scan(&existingID)

	now := time.Now()

	// Extract project name from full path (last part after /)
	projectName := args.ProjectPath
	if lastSlash := len(args.ProjectPath) - 1; lastSlash >= 0 {
		for i := lastSlash; i >= 0; i-- {
			if args.ProjectPath[i] == '/' {
				projectName = args.ProjectPath[i+1:]
				break
			}
		}
	}

	// Prepare webhook details for removal
	webhookID := ""
	webhookURL := w.getWebhookEndpointForProvider("gitlab")
	webhookName := "LiveReview Webhook"
	events := "merge_requests,notes"
	status := "unconnected" // Mark as unconnected

	if err == pgx.ErrNoRows {
		// No existing record, create a new one marked as unconnected
		insertQuery := `
			INSERT INTO webhook_registry (
				provider,
				provider_project_id,
				project_name,
				project_full_name,
				webhook_id,
				webhook_url,
				webhook_secret,
				webhook_name,
				events,
				status,
				last_verified_at,
				integration_token_id,
				created_at,
				updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		`

		_, err = w.pool.Exec(ctx, insertQuery,
			args.Provider,
			fmt.Sprintf("%d", projectID),
			projectName,
			args.ProjectPath,
			webhookID,
			webhookURL,
			w.config.WebhookConfig.Secret,
			webhookName,
			events,
			status,
			now,
			args.ConnectorID,
			now,
			now,
		)

		if err != nil {
			return fmt.Errorf("failed to create webhook registry entry: %w", err)
		}

		log.Printf("Created webhook_registry entry for project %s with status '%s'", args.ProjectPath, status)
	} else if err != nil {
		return fmt.Errorf("failed to check existing webhook registry entry: %w", err)
	} else {
		// Update existing record
		updateQuery := `
			UPDATE webhook_registry 
			SET 
				webhook_id = $1,
				webhook_url = $2,
				webhook_secret = $3,
				status = $4,
				last_verified_at = $5,
				updated_at = $6
			WHERE id = $7
		`

		_, err = w.pool.Exec(ctx, updateQuery,
			webhookID,
			webhookURL,
			w.config.WebhookConfig.Secret,
			status,
			now,
			now,
			existingID,
		)

		if err != nil {
			return fmt.Errorf("failed to update webhook registry: %w", err)
		}

		log.Printf("Updated webhook_registry entry for project %s with status '%s'", args.ProjectPath, status)
	}

	return nil
}

// GitHub webhook removal methods

// makeGitHubRequestForRemoval makes a request to the GitHub API for webhook removal
func (w *WebhookRemovalWorker) makeGitHubRequestForRemoval(method, endpoint string, payload interface{}, baseURL, pat string) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request payload: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
		log.Printf("GitHub API %s %s with payload: %s", method, endpoint, string(jsonData))
	} else {
		log.Printf("GitHub API %s %s", method, endpoint)
	}

	// Determine API base URL
	apiBaseURL := "https://api.github.com"
	if baseURL != "" && baseURL != "https://github.com" {
		// For GitHub Enterprise, the API is typically at /api/v3
		apiBaseURL = strings.TrimSuffix(baseURL, "/") + "/api/v3"
	}

	req, err := http.NewRequest(method, apiBaseURL+endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+pat)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "LiveReview/1.0")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// removeGitHubWebhooks removes all LiveReview webhooks from a GitHub repository
func (w *WebhookRemovalWorker) removeGitHubWebhooks(owner, repo, baseURL, pat string, connectorID int) error {
	// Get existing hooks
	endpoint := fmt.Sprintf("/repos/%s/%s/hooks", owner, repo)
	resp, err := w.makeGitHubRequestForRemoval("GET", endpoint, nil, baseURL, pat)
	if err != nil {
		return fmt.Errorf("failed to fetch existing hooks: %w", err)
	}
	defer resp.Body.Close()

	// Handle 404 gracefully - repository might be deleted or no access
	if resp.StatusCode == http.StatusNotFound {
		log.Printf("[INFO] Repository %s/%s not found or no access - treating as already removed", owner, repo)
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error fetching hooks (status %d): %s", resp.StatusCode, string(body))
	}

	var hooks []GitHubHook
	if err := json.NewDecoder(resp.Body).Decode(&hooks); err != nil {
		return fmt.Errorf("failed to decode hooks response: %w", err)
	}

	// Build both old-style (no connector_id) and new-style (with connector_id) webhook URLs
	webhookURLNew := w.getWebhookEndpointForProviderWithConnector("github", connectorID)
	webhookURLOld := w.getWebhookEndpointForProvider("github")
	removedCount := 0

	// Remove hooks that match either old-style or new-style webhook URLs
	for _, hook := range hooks {
		if hook.Config.URL == webhookURLNew || hook.Config.URL == webhookURLOld {
			log.Printf("Removing webhook #%d with URL: %s", hook.ID, hook.Config.URL)
			deleteEndpoint := fmt.Sprintf("/repos/%s/%s/hooks/%d", owner, repo, hook.ID)
			resp, err := w.makeGitHubRequestForRemoval("DELETE", deleteEndpoint, nil, baseURL, pat)
			if err != nil {
				log.Printf("Failed to delete webhook #%d: %v", hook.ID, err)
				continue
			}
			resp.Body.Close()

			// Treat 404 as success - webhook already deleted
			if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
				log.Printf("Deleted webhook #%d for repository %s/%s (or already deleted)", hook.ID, owner, repo)
				removedCount++
			} else {
				log.Printf("Failed to delete webhook #%d (status %d)", hook.ID, resp.StatusCode)
			}
		}
	}

	if removedCount == 0 {
		log.Printf("No LiveReview webhooks found for repository %s/%s", owner, repo)
	} else {
		log.Printf("Removed %d webhooks for repository %s/%s", removedCount, owner, repo)
	}

	return nil
}

// updateWebhookRegistryForGitHubRemoval updates the webhook registry to mark GitHub repository as unconnected
func (w *WebhookRemovalWorker) updateWebhookRegistryForGitHubRemoval(ctx context.Context, args WebhookRemovalJobArgs) error {
	// Check if entry exists in webhook registry
	checkQuery := `
		SELECT id FROM webhook_registry 
		WHERE integration_token_id = $1 AND project_full_name = $2
	`

	var existingID int
	err := w.pool.QueryRow(ctx, checkQuery, args.ConnectorID, args.ProjectPath).Scan(&existingID)

	now := time.Now()

	// Extract project name from full path (last part after /)
	projectName := args.ProjectPath
	if lastSlash := strings.LastIndex(args.ProjectPath, "/"); lastSlash >= 0 {
		projectName = args.ProjectPath[lastSlash+1:]
	}

	// Prepare webhook details for removal
	webhookID := ""
	webhookURL := w.getWebhookEndpointForProvider("github")
	webhookName := "LiveReview Webhook"
	events := "pull_request,issue_comment"
	status := "unconnected" // Mark as unconnected

	if err == pgx.ErrNoRows {
		// No existing record, create a new one marked as unconnected
		insertQuery := `
			INSERT INTO webhook_registry (
				provider,
				provider_project_id,
				project_name,
				project_full_name,
				webhook_id,
				webhook_url,
				webhook_secret,
				webhook_name,
				events,
				status,
				last_verified_at,
				integration_token_id,
				created_at,
				updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		`

		_, err = w.pool.Exec(ctx, insertQuery,
			args.Provider,
			args.ProjectPath, // Use full project path as ID for GitHub
			projectName,
			args.ProjectPath,
			webhookID,
			webhookURL,
			w.config.WebhookConfig.Secret,
			webhookName,
			events,
			status,
			now,
			args.ConnectorID,
			now,
			now,
		)

		if err != nil {
			return fmt.Errorf("failed to create webhook registry entry: %w", err)
		}

		log.Printf("Created webhook_registry entry for GitHub repository %s with status '%s'", args.ProjectPath, status)
	} else if err != nil {
		return fmt.Errorf("failed to check existing webhook registry entry: %w", err)
	} else {
		// Update existing record
		updateQuery := `
			UPDATE webhook_registry 
			SET 
				webhook_id = $1,
				webhook_url = $2,
				webhook_secret = $3,
				status = $4,
				last_verified_at = $5,
				updated_at = $6
			WHERE id = $7
		`

		_, err = w.pool.Exec(ctx, updateQuery,
			webhookID,
			webhookURL,
			w.config.WebhookConfig.Secret,
			status,
			now,
			now,
			existingID,
		)

		if err != nil {
			return fmt.Errorf("failed to update webhook registry: %w", err)
		}

		log.Printf("Updated webhook_registry entry for GitHub repository %s with status '%s'", args.ProjectPath, status)
	}

	return nil
}

// Bitbucket webhook removal methods

// handleBitbucketWebhookRemoval handles Bitbucket webhook removal
func (w *WebhookRemovalWorker) handleBitbucketWebhookRemoval(ctx context.Context, args WebhookRemovalJobArgs) error {
	// Parse Bitbucket repository from project path (format: workspace/repo)
	parts := strings.SplitN(args.ProjectPath, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid Bitbucket repository format: %s (expected: workspace/repo)", args.ProjectPath)
	}
	workspace, repo := parts[0], parts[1]

	log.Printf("Removing Bitbucket webhooks for repository: %s/%s", workspace, repo)

	// Get email from connector metadata (needed for Bitbucket authentication)
	email, err := w.getBitbucketEmailFromConnectorForRemoval(args.ConnectorID)
	if err != nil {
		return fmt.Errorf("failed to get Bitbucket email for connector %d: %w", args.ConnectorID, err)
	}

	// Remove webhooks from Bitbucket
	err = w.removeBitbucketWebhooks(workspace, repo, email, args.PAT, args.ConnectorID)
	if err != nil {
		log.Printf("Failed to remove webhooks from Bitbucket repository %s/%s: %v", workspace, repo, err)
		// Don't return error here - we still want to update the registry
	}

	// Update the webhook_registry to mark as removed
	err = w.updateWebhookRegistryForBitbucketRemoval(ctx, args)
	if err != nil {
		return fmt.Errorf("failed to update webhook registry: %w", err)
	}

	log.Printf("Bitbucket webhook removal completed for repository: %s/%s", workspace, repo)
	return nil
}

// getBitbucketEmailFromConnectorForRemoval retrieves the email from connector metadata for removal operations
func (w *WebhookRemovalWorker) getBitbucketEmailFromConnectorForRemoval(connectorID int) (string, error) {
	var metadataBytes []byte
	query := `SELECT COALESCE(metadata, '{}') FROM integration_tokens WHERE id = $1`

	err := w.pool.QueryRow(context.Background(), query, connectorID).Scan(&metadataBytes)
	if err != nil {
		return "", fmt.Errorf("failed to get connector metadata: %w", err)
	}

	var metadata map[string]interface{}
	if len(metadataBytes) > 0 {
		if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
			return "", fmt.Errorf("failed to parse metadata: %w", err)
		}
	}

	email, ok := metadata["email"].(string)
	if !ok || email == "" {
		return "", fmt.Errorf("email not found in connector metadata")
	}

	return email, nil
}

// makeBitbucketRequestForRemoval makes a request to the Bitbucket API for removal operations
func (w *WebhookRemovalWorker) makeBitbucketRequestForRemoval(method, endpoint string, payload interface{}, email, apiToken string) (*http.Response, error) {
	var reqBody io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	// Always use Bitbucket Cloud API
	url := "https://api.bitbucket.org/2.0" + endpoint
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set Basic Auth with email and API token
	req.SetBasicAuth(email, apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LiveReview/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

// removeBitbucketWebhooks removes all LiveReview webhooks from a Bitbucket repository
func (w *WebhookRemovalWorker) removeBitbucketWebhooks(workspace, repo, email, apiToken string, connectorID int) error {
	// Get list of existing webhooks
	endpoint := fmt.Sprintf("/repositories/%s/%s/hooks", workspace, repo)
	resp, err := w.makeBitbucketRequestForRemoval("GET", endpoint, nil, email, apiToken)
	if err != nil {
		return fmt.Errorf("failed to fetch webhooks: %w", err)
	}
	defer resp.Body.Close()

	// Handle 404 gracefully - repository might be deleted or no access
	if resp.StatusCode == http.StatusNotFound {
		log.Printf("[INFO] Repository %s/%s not found or no access - treating as already removed", workspace, repo)
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bitbucket API error fetching webhooks (status %d): %s", resp.StatusCode, string(body))
	}

	var response struct {
		Values []BitbucketHook `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to decode webhook list: %w", err)
	}

	// Build both old-style (no connector_id) and new-style (with connector_id) webhook URLs
	webhookURLNew := w.getWebhookEndpointForProviderWithConnector("bitbucket", connectorID)
	webhookURLOld := w.getWebhookEndpointForProviderRemoval("bitbucket")

	// Remove webhooks that match either old-style or new-style webhook URLs
	removedCount := 0
	for _, hook := range response.Values {
		if hook.URL == webhookURLNew || hook.URL == webhookURLOld {
			log.Printf("Removing webhook %s with URL: %s", hook.UUID, hook.URL)
			deleteEndpoint := fmt.Sprintf("/repositories/%s/%s/hooks/%s", workspace, repo, hook.UUID)
			deleteResp, err := w.makeBitbucketRequestForRemoval("DELETE", deleteEndpoint, nil, email, apiToken)
			if err != nil {
				log.Printf("Failed to delete webhook %s: %v", hook.UUID, err)
				continue
			}
			deleteResp.Body.Close()

			// Treat 404 as success - webhook already deleted
			if deleteResp.StatusCode == http.StatusNoContent || deleteResp.StatusCode == http.StatusNotFound {
				log.Printf("Removed Bitbucket webhook %s from repository %s/%s (or already deleted)", hook.UUID, workspace, repo)
				removedCount++
			} else {
				log.Printf("Failed to delete webhook %s (status %d)", hook.UUID, deleteResp.StatusCode)
			}
		}
	}

	if removedCount == 0 {
		log.Printf("No LiveReview webhooks found for repository %s/%s", workspace, repo)
	} else {
		log.Printf("Removed %d webhooks for repository %s/%s", removedCount, workspace, repo)
	}

	return nil
}

// getWebhookEndpointForProviderRemoval gets the webhook endpoint URL for a specific provider (for removal operations)
func (w *WebhookRemovalWorker) getWebhookEndpointForProviderRemoval(provider string) string {
	// Use the same base URL as the install worker
	baseURL := w.config.WebhookConfig.PublicEndpoint
	if baseURL == "" {
		baseURL = "https://your-domain.com" // fallback
	}

	// Remove any trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Return old-style webhook URL with /api/v1 prefix (consistent with getWebhookEndpointForProvider)
	return baseURL + "/api/v1/" + provider + "-hook"
}

// updateWebhookRegistryForBitbucketRemoval updates the webhook registry to mark Bitbucket repository as unconnected
func (w *WebhookRemovalWorker) updateWebhookRegistryForBitbucketRemoval(ctx context.Context, args WebhookRemovalJobArgs) error {
	// Check if entry exists in webhook registry
	checkQuery := `
		SELECT id FROM webhook_registry 
		WHERE integration_token_id = $1 AND project_full_name = $2
	`

	var existingID int
	err := w.pool.QueryRow(ctx, checkQuery, args.ConnectorID, args.ProjectPath).Scan(&existingID)

	now := time.Now()

	// Extract project name from full path (last part after /)
	projectName := args.ProjectPath
	if lastSlash := strings.LastIndex(args.ProjectPath, "/"); lastSlash >= 0 {
		projectName = args.ProjectPath[lastSlash+1:]
	}

	// Status should be "none" to indicate no webhook is installed
	status := "none"
	webhookID := ""
	webhookURL := w.getWebhookEndpointForProviderRemoval("bitbucket")
	webhookName := ""
	events := ""

	if err == pgx.ErrNoRows {
		// No existing record, create a new one with "none" status
		insertQuery := `
			INSERT INTO webhook_registry (
				provider,
				provider_project_id,
				project_name,
				project_full_name,
				webhook_id,
				webhook_url,
				webhook_secret,
				webhook_name,
				events,
				status,
				last_verified_at,
				integration_token_id,
				created_at,
				updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		`

		_, err = w.pool.Exec(ctx, insertQuery,
			args.Provider,
			args.ProjectPath, // Use full project path as ID for Bitbucket
			projectName,
			args.ProjectPath,
			webhookID,
			webhookURL,
			w.config.WebhookConfig.Secret,
			webhookName,
			events,
			status,
			now,
			args.ConnectorID,
			now,
			now,
		)

		if err != nil {
			return fmt.Errorf("failed to create webhook registry entry: %w", err)
		}

		log.Printf("Created webhook_registry entry for Bitbucket repository %s with status '%s'", args.ProjectPath, status)
	} else if err != nil {
		return fmt.Errorf("failed to check existing webhook registry entry: %w", err)
	} else {
		// Update existing record
		updateQuery := `
			UPDATE webhook_registry 
			SET 
				webhook_id = $1,
				webhook_url = $2,
				webhook_secret = $3,
				status = $4,
				last_verified_at = $5,
				updated_at = $6
			WHERE id = $7
		`

		_, err = w.pool.Exec(ctx, updateQuery,
			webhookID,
			webhookURL,
			w.config.WebhookConfig.Secret,
			status,
			now,
			now,
			existingID,
		)

		if err != nil {
			return fmt.Errorf("failed to update webhook registry: %w", err)
		}

		log.Printf("Updated webhook_registry entry for Bitbucket repository %s with status '%s'", args.ProjectPath, status)
	}

	return nil
}

// Gitea webhook removal methods

// handleGiteaWebhookRemoval handles Gitea webhook removal
func (w *WebhookRemovalWorker) handleGiteaWebhookRemoval(ctx context.Context, args WebhookRemovalJobArgs) error {
	parts := strings.SplitN(args.ProjectPath, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid Gitea repository format: %s (expected: owner/repo)", args.ProjectPath)
	}
	owner, repo := parts[0], parts[1]

	// Unpack PAT if it's in packed format
	pat := gitea.UnpackGiteaPAT(args.PAT)

	log.Printf("Removing Gitea webhooks for repository: %s/%s", owner, repo)

	if err := w.removeGiteaWebhooks(owner, repo, args.BaseURL, pat, args.ConnectorID); err != nil {
		log.Printf("Failed to remove webhooks from Gitea repository %s/%s: %v", owner, repo, err)
		// continue to registry update even on API failure
	}

	if err := w.updateWebhookRegistryForGiteaRemoval(ctx, args); err != nil {
		return fmt.Errorf("failed to update webhook registry: %w", err)
	}

	log.Printf("Gitea webhook removal completed for repository: %s/%s", owner, repo)
	return nil
}

// makeGiteaRequestForRemoval makes a request to the Gitea API for removal operations
func (w *WebhookRemovalWorker) makeGiteaRequestForRemoval(method, endpoint string, payload interface{}, baseURL, pat string) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
	}

	apiBase := gitea.NormalizeGiteaBaseURL(baseURL)
	// Gitea API endpoints always use /api/v1 prefix
	fullURL := fmt.Sprintf("%s/api/v1%s", apiBase, endpoint)

	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+pat)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	return resp, nil
}

// removeGiteaWebhooks removes all LiveReview webhooks from a Gitea repository
func (w *WebhookRemovalWorker) removeGiteaWebhooks(owner, repo, baseURL, pat string, connectorID int) error {
	endpoint := fmt.Sprintf("/repos/%s/%s/hooks", owner, repo)
	resp, err := w.makeGiteaRequestForRemoval(http.MethodGet, endpoint, nil, baseURL, pat)
	if err != nil {
		return fmt.Errorf("failed to fetch webhooks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		log.Printf("[INFO] Gitea repository %s/%s not found or inaccessible - treating as already removed", owner, repo)
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gitea API error fetching webhooks (status %d): %s", resp.StatusCode, string(body))
	}

	var hooks []GiteaHook
	if err := json.NewDecoder(resp.Body).Decode(&hooks); err != nil {
		return fmt.Errorf("failed to decode webhook list: %w", err)
	}

	webhookURLNew := w.getWebhookEndpointForProviderWithConnector("gitea", connectorID)
	webhookURLOld := w.getWebhookEndpointForProvider("gitea")

	removed := 0
	for _, hook := range hooks {
		if hook.Config.URL == webhookURLNew || hook.Config.URL == webhookURLOld {
			deleteEndpoint := fmt.Sprintf("/api/v1/repos/%s/%s/hooks/%d", owner, repo, hook.ID)
			deleteResp, err := w.makeGiteaRequestForRemoval(http.MethodDelete, deleteEndpoint, nil, baseURL, pat)
			if err != nil {
				log.Printf("Failed to delete Gitea webhook %d: %v", hook.ID, err)
				continue
			}
			deleteResp.Body.Close()

			if deleteResp.StatusCode == http.StatusNoContent || deleteResp.StatusCode == http.StatusNotFound || deleteResp.StatusCode == http.StatusOK {
				removed++
				log.Printf("Removed Gitea webhook %d for repository %s/%s", hook.ID, owner, repo)
			} else {
				log.Printf("Failed to delete Gitea webhook %d (status %d)", hook.ID, deleteResp.StatusCode)
			}
		}
	}

	if removed == 0 {
		log.Printf("No LiveReview webhooks found for Gitea repository %s/%s", owner, repo)
	}

	return nil
}

// updateWebhookRegistryForGiteaRemoval marks a Gitea repository as unconnected in the webhook registry
func (w *WebhookRemovalWorker) updateWebhookRegistryForGiteaRemoval(ctx context.Context, args WebhookRemovalJobArgs) error {
	checkQuery := `
		SELECT id FROM webhook_registry 
		WHERE integration_token_id = $1 AND project_full_name = $2
	`

	var existingID int
	err := w.pool.QueryRow(ctx, checkQuery, args.ConnectorID, args.ProjectPath).Scan(&existingID)
	now := time.Now()

	projectName := args.ProjectPath
	if slash := strings.LastIndex(args.ProjectPath, "/"); slash >= 0 {
		projectName = args.ProjectPath[slash+1:]
	}

	status := "unconnected"
	webhookID := ""
	webhookURL := w.getWebhookEndpointForProvider("gitea")
	webhookName := "LiveReview Webhook"
	events := "pull_request,issue_comment"

	if err == pgx.ErrNoRows {
		insertQuery := `
			INSERT INTO webhook_registry (
				provider,
				provider_project_id,
				project_name,
				project_full_name,
				webhook_id,
				webhook_url,
				webhook_secret,
				webhook_name,
				events,
				status,
				last_verified_at,
				integration_token_id,
				created_at,
				updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		`

		_, err = w.pool.Exec(ctx, insertQuery,
			args.Provider,
			args.ProjectPath,
			projectName,
			args.ProjectPath,
			webhookID,
			webhookURL,
			w.config.WebhookConfig.Secret,
			webhookName,
			events,
			status,
			now,
			args.ConnectorID,
			now,
			now,
		)

		if err != nil {
			return fmt.Errorf("failed to create webhook registry entry: %w", err)
		}

		log.Printf("Created webhook_registry entry for Gitea repository %s with status '%s'", args.ProjectPath, status)
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check existing webhook registry entry: %w", err)
	}

	updateQuery := `
		UPDATE webhook_registry 
		SET 
			webhook_id = $1,
			webhook_url = $2,
			webhook_secret = $3,
			status = $4,
			last_verified_at = $5,
			updated_at = $6
		WHERE id = $7
	`

	_, err = w.pool.Exec(ctx, updateQuery,
		webhookID,
		webhookURL,
		w.config.WebhookConfig.Secret,
		status,
		now,
		now,
		existingID,
	)

	if err != nil {
		return fmt.Errorf("failed to update webhook registry: %w", err)
	}

	log.Printf("Updated webhook_registry entry for Gitea repository %s with status '%s'", args.ProjectPath, status)
	return nil
}

// JobQueue manages the River job queue
type JobQueue struct {
	client *river.Client[pgx.Tx]
	pool   *pgxpool.Pool
	config *QueueConfig
}

// NewJobQueue creates a new job queue instance
func NewJobQueue(databaseURL string, db *sql.DB) (*JobQueue, error) {
	// Get configuration with database-sourced webhook endpoint
	config, err := GetQueueConfigWithDB(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue configuration: %w", err)
	}

	// Create a pgx connection pool
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Create River client
	workers := river.NewWorkers()
	river.AddWorker(workers, &WebhookInstallWorker{pool: pool, config: config})
	river.AddWorker(workers, &WebhookRemovalWorker{pool: pool, config: config})

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  config.RiverQueueConfig(),
		Workers: workers,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create River client: %w", err)
	}

	return &JobQueue{
		client: client,
		pool:   pool,
		config: config,
	}, nil
}

// Start starts the job queue workers
func (jq *JobQueue) Start(ctx context.Context) error {
	return jq.client.Start(ctx)
}

// Stop stops the job queue workers
func (jq *JobQueue) Stop(ctx context.Context) error {
	return jq.client.Stop(ctx)
}

// QueueWebhookInstallJob queues a webhook installation job
func (jq *JobQueue) QueueWebhookInstallJob(ctx context.Context, connectorID int, projectPath, provider, baseURL, pat string) error {
	args := WebhookInstallJobArgs{
		ConnectorID: connectorID,
		ProjectPath: projectPath,
		Provider:    provider,
		BaseURL:     baseURL,
		PAT:         pat,
	}

	_, err := jq.client.Insert(ctx, args, nil)
	if err != nil {
		return fmt.Errorf("failed to queue webhook install job: %w", err)
	}

	return nil
}

// QueueWebhookRemovalJob queues a webhook removal job
func (jq *JobQueue) QueueWebhookRemovalJob(ctx context.Context, connectorID int, projectPath, provider, baseURL, pat string) error {
	args := WebhookRemovalJobArgs{
		ConnectorID: connectorID,
		ProjectPath: projectPath,
		Provider:    provider,
		BaseURL:     baseURL,
		PAT:         pat,
	}

	_, err := jq.client.Insert(ctx, args, nil)
	if err != nil {
		return fmt.Errorf("failed to queue webhook removal job: %w", err)
	}

	return nil
}
