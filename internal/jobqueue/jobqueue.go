/*
Package jobqueue provides a River-based job queue system for managing webhook installations.

For configuration options, retry policies, and tuning parameters, see queue_config.go.
All configurable values have been moved there for easier management.
*/
package jobqueue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

func (w *WebhookInstallWorker) installWebhook(projectID int, baseURL, pat string) (*GitLabHook, error) {
	// Check if webhook already exists
	existingHook, err := w.webhookExists(projectID, w.config.WebhookConfig.PublicEndpoint, baseURL, pat)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing webhooks: %w", err)
	}

	payload := WebhookPayload{
		URL:                   w.config.WebhookConfig.PublicEndpoint,
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

	log.Printf("Processing webhook installation for project: %s (connector: %d)",
		args.ProjectPath, args.ConnectorID)

	// Get the numeric project ID from GitLab
	projectID, err := w.getProjectID(args.ProjectPath, args.BaseURL, args.PAT)
	if err != nil {
		log.Printf("Failed to get project ID for %s: %v", args.ProjectPath, err)
		return fmt.Errorf("failed to get project ID: %w", err)
	}

	log.Printf("Resolved project %s to ID: %d", args.ProjectPath, projectID)

	// Install the webhook in GitLab
	webhook, err := w.installWebhook(projectID, args.BaseURL, args.PAT)
	if err != nil {
		log.Printf("Failed to install webhook for project %s (ID: %d): %v", args.ProjectPath, projectID, err)
		return fmt.Errorf("failed to install webhook: %w", err)
	}

	log.Printf("Successfully installed webhook #%d for project %s", webhook.ID, args.ProjectPath)

	// Update the webhook_registry with the actual webhook details
	err = w.updateWebhookRegistry(ctx, args, webhook)
	if err != nil {
		log.Printf("Failed to update webhook registry for project %s: %v", args.ProjectPath, err)
		// Don't return error here since webhook was successfully installed
		// Just log the issue
	}

	log.Printf("Webhook installation completed for project: %s", args.ProjectPath)
	return nil
}

// updateWebhookRegistry creates or updates the webhook registry entry for a project
func (w *WebhookInstallWorker) updateWebhookRegistry(ctx context.Context, args WebhookInstallJobArgs, webhook *GitLabHook) error {
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
	webhookURL := w.config.WebhookConfig.PublicEndpoint
	webhookName := "LiveReview Webhook"
	events := "merge_requests,notes"
	status := "automatic" // Changed from "manual" since we actually installed a webhook

	if webhook != nil {
		webhookID = fmt.Sprintf("%d", webhook.ID)
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

// JobQueue manages the River job queue
type JobQueue struct {
	client *river.Client[pgx.Tx]
	pool   *pgxpool.Pool
	config *QueueConfig
}

// NewJobQueue creates a new job queue instance
func NewJobQueue(databaseURL string) (*JobQueue, error) {
	// Get configuration
	config := GetQueueConfig()

	// Create a pgx connection pool
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Create River client
	workers := river.NewWorkers()
	river.AddWorker(workers, &WebhookInstallWorker{pool: pool, config: config})

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
