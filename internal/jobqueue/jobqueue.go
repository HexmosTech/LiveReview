package jobqueue

import (
	"context"
	"fmt"
	"log"

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
}

// Work performs the webhook installation
func (w *WebhookInstallWorker) Work(ctx context.Context, job *river.Job[WebhookInstallJobArgs]) error {
	args := job.Args

	log.Printf("Processing webhook installation for project: %s (connector: %d)",
		args.ProjectPath, args.ConnectorID)

	// TODO: Implement actual webhook installation logic
	// For now, just simulate the work
	log.Printf("Installing webhook for %s project: %s", args.Provider, args.ProjectPath)
	log.Printf("Base URL: %s", args.BaseURL)

	// Simulate some work
	// In a real implementation, this would:
	// 1. Create a webhook in the Git provider (GitLab, GitHub, etc.)
	// 2. Configure the webhook to point to our LiveReview instance
	// 3. Set up the necessary permissions
	// 4. Store webhook configuration in the database

	log.Printf("Webhook installation completed for project: %s", args.ProjectPath)
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
	river.AddWorker(workers, &WebhookInstallWorker{})

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
