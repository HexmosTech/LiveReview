package jobqueue

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

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
	pool *pgxpool.Pool
}

// Work performs the webhook installation
func (w *WebhookInstallWorker) Work(ctx context.Context, job *river.Job[WebhookInstallJobArgs]) error {
	args := job.Args

	log.Printf("Processing webhook installation for project: %s (connector: %d)",
		args.ProjectPath, args.ConnectorID)

	// First, update the webhook_registry to mark this project as "manual" status
	err := w.updateWebhookRegistry(ctx, args)
	if err != nil {
		log.Printf("Failed to update webhook registry for project %s: %v", args.ProjectPath, err)
		return fmt.Errorf("failed to update webhook registry: %w", err)
	}

	// TODO: Implement actual webhook installation logic
	// For now, just simulate the work
	log.Printf("Installing webhook for %s project: %s", args.Provider, args.ProjectPath)
	log.Printf("Base URL: %s", args.BaseURL)

	// Simulate some work
	// In a real implementation, this would:
	// 1. Create a webhook in the Git provider (GitLab, GitHub, etc.)
	// 2. Configure the webhook to point to our LiveReview instance
	// 3. Set up the necessary permissions
	// 4. Update webhook_registry with actual webhook details

	log.Printf("Webhook installation completed for project: %s", args.ProjectPath)
	return nil
}

// updateWebhookRegistry creates or updates the webhook registry entry for a project
func (w *WebhookInstallWorker) updateWebhookRegistry(ctx context.Context, args WebhookInstallJobArgs) error {
	// First check if a record already exists
	var existingID int
	checkQuery := `
		SELECT id FROM webhook_registry 
		WHERE integration_token_id = $1 AND project_full_name = $2
	`
	err := w.pool.QueryRow(ctx, checkQuery, args.ConnectorID, args.ProjectPath).Scan(&existingID)

	now := time.Now()

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

		_, err = w.pool.Exec(ctx, insertQuery,
			args.Provider,               // provider
			args.ProjectPath,            // provider_project_id (using project path as ID)
			projectName,                 // project_name
			args.ProjectPath,            // project_full_name
			"",                          // webhook_id (empty for manual trigger)
			"",                          // webhook_url (empty for manual trigger)
			"",                          // webhook_secret (empty for manual trigger)
			"LiveReview Manual Trigger", // webhook_name
			"manual_trigger",            // events
			"manual",                    // status (manual trigger enabled)
			now,                         // last_verified_at
			now,                         // created_at
			now,                         // updated_at
			args.ConnectorID,            // integration_token_id
		)

		if err != nil {
			return fmt.Errorf("failed to insert webhook registry: %w", err)
		}

		log.Printf("Created webhook_registry entry for project %s with status 'manual'", args.ProjectPath)
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
			    events = $5
			WHERE id = $6
		`

		_, err = w.pool.Exec(ctx, updateQuery,
			"manual",                    // status
			now,                         // updated_at
			now,                         // last_verified_at
			"LiveReview Manual Trigger", // webhook_name
			"manual_trigger",            // events
			existingID,                  // id
		)

		if err != nil {
			return fmt.Errorf("failed to update webhook registry: %w", err)
		}

		log.Printf("Updated webhook_registry entry for project %s with status 'manual'", args.ProjectPath)
	}

	return nil
}

// JobQueue manages the River job queue
type JobQueue struct {
	client *river.Client[pgx.Tx]
	pool   *pgxpool.Pool
}

// NewJobQueue creates a new job queue instance
func NewJobQueue(databaseURL string) (*JobQueue, error) {
	// Create a pgx connection pool
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Create River client
	workers := river.NewWorkers()
	river.AddWorker(workers, &WebhookInstallWorker{pool: pool})

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
		Workers: workers,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create River client: %w", err)
	}

	return &JobQueue{
		client: client,
		pool:   pool,
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
