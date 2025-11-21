/*
Package jobqueue configuration - All tunable parameters for the River job queue system.

# River Job Queue Configuration Guide

This file contains all configurable parameters for the webhook installation job queue.
Modify these values to tune performance, reliability, and behavior according to your needs.

## Quick Configuration Reference:

### Performance Tuning:
- Increase MaxWorkers for higher throughput (more concurrent webhook installations)
- Adjust MaxRetries for different reliability vs. speed tradeoffs
- Modify retry intervals for faster/slower retry cycles

### Reliability Tuning:
- Increase MaxRetries for better reliability on unstable networks
- Adjust RetryPolicy intervals for network conditions
- Configure job timeouts based on GitLab API response times

### Resource Management:
- Lower MaxWorkers to reduce database connection usage
- Adjust timeouts to prevent resource leaks
- Configure queue priorities if multiple job types are added

## Monitoring and Debugging:
- Enable verbose logging by modifying log levels in the worker
- Job status can be monitored via River's built-in job tracking
- Failed jobs retain error information in the River jobs table
- Webhook installation results are stored in the webhook_registry table

## Security Configuration:
- GitLab credentials are currently hardcoded (will be made dynamic later)
- Webhook secret token is configured below
- All API requests use HTTPS with SSL verification enabled

## Database Requirements:
- PostgreSQL with River schema migrations applied
- Connection pool configured for concurrent workers
- webhook_registry table for storing installation results
*/
package jobqueue

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/riverqueue/river"
)

// getWebhookPublicEndpoint returns the base webhook endpoint URL (without provider-specific path) based on deployment mode.
// In production mode, it queries the database for livereview_prod_url.
// If not set, returns empty string (will be validated when actually installing webhooks).
// The returned base URL does NOT include provider path or connector_id - those are appended by getWebhookEndpointForProviderWithCustomEndpoint.
func getWebhookPublicEndpoint(db *sql.DB) (string, error) {
	reverseProxy := os.Getenv("LIVEREVIEW_REVERSE_PROXY") == "true"

	if reverseProxy {
		// Production mode: Query DB for livereview_prod_url
		var prodURL sql.NullString
		err := db.QueryRow(`SELECT livereview_prod_url FROM instance_details ORDER BY id DESC LIMIT 1`).Scan(&prodURL)
		if err != nil {
			if err == sql.ErrNoRows {
				// No production URL set - return empty string (will be validated during webhook installation)
				return "", nil
			}
			return "", fmt.Errorf("Error querying production URL: %v", err)
		}
		if !prodURL.Valid || strings.TrimSpace(prodURL.String) == "" {
			// Production URL is empty - return empty string (will be validated during webhook installation)
			return "", nil
		}
		// Return just the base URL without any provider-specific path
		return strings.TrimSuffix(strings.TrimSpace(prodURL.String), "/"), nil
	} else {
		// Demo mode: Use localhost with backend port
		backendPort := os.Getenv("LIVEREVIEW_BACKEND_PORT")
		if backendPort == "" {
			backendPort = "8888"
		}
		// Return just the base URL without any provider-specific path
		return "http://localhost:" + backendPort, nil
	}
}

// QueueConfig holds all configurable parameters for the job queue
type QueueConfig struct {
	// Worker Configuration
	MaxWorkers int // Number of concurrent workers processing jobs (default: 10)

	// Retry Configuration
	MaxRetries   int           // Maximum retry attempts per job (default: 25)
	RetryPolicy  RetryPolicy   // Retry timing and backoff configuration
	JobTimeout   time.Duration // Maximum time a single job can run (default: 5 minutes)
	QueueTimeout time.Duration // Maximum time jobs can stay in queue (default: 24 hours)

	// GitLab API Configuration (hardcoded for now, will be dynamic later)
	GitLabConfig GitLabConfig

	// Webhook Configuration
	WebhookConfig WebhookConfig
}

// RetryPolicy defines how failed jobs are retried
type RetryPolicy struct {
	// InitialInterval is the time to wait before the first retry
	InitialInterval time.Duration // default: 1 second

	// MaxInterval is the maximum time to wait between retries
	MaxInterval time.Duration // default: 1 hour

	// Multiplier is the factor by which the interval increases after each retry
	Multiplier float64 // default: 2.0 (exponential backoff)

	// MaxElapsedTime is the total time after which retries stop
	MaxElapsedTime time.Duration // default: 72 hours (3 days)
}

// GitLabConfig holds GitLab API configuration
type GitLabConfig struct {
	BaseURL string // GitLab instance base URL
	PAT     string // Personal Access Token with API scope
}

// WebhookConfig holds webhook-specific configuration
type WebhookConfig struct {
	Secret         string // Secret token for webhook verification
	PublicEndpoint string // Public URL where GitLab can reach the webhook handler
	EnableSSL      bool   // Whether to enable SSL verification for webhooks

	// Event types to subscribe to
	Events WebhookEvents
}

// WebhookEvents defines which GitLab events to subscribe to
type WebhookEvents struct {
	PushEvents          bool // Git push events
	IssuesEvents        bool // Issue creation/updates
	MergeRequestsEvents bool // Merge request events (recommended: true)
	TagPushEvents       bool // Git tag push events
	NoteEvents          bool // Comments on issues/MRs (recommended: true)
	JobEvents           bool // CI/CD job events
	PipelineEvents      bool // CI/CD pipeline events
}

// DefaultQueueConfig returns the default configuration
// For production webhook URL, use DefaultQueueConfigWithDB instead
func DefaultQueueConfig() *QueueConfig {
	return &QueueConfig{
		// Worker settings - tune based on your server capacity and GitLab API rate limits
		MaxWorkers: 10, // Start with 10, increase if you have many projects and good network

		// Retry settings - River default is 25 retries over ~3 days
		MaxRetries: 25,
		RetryPolicy: RetryPolicy{
			InitialInterval: 1 * time.Second, // Start retrying quickly
			MaxInterval:     1 * time.Hour,   // Don't wait more than 1 hour between retries
			Multiplier:      2.0,             // Double the wait time each retry
			MaxElapsedTime:  72 * time.Hour,  // Give up after 3 days
		},

		// Timeout settings
		JobTimeout:   5 * time.Minute, // Each webhook installation should complete within 5 minutes
		QueueTimeout: 24 * time.Hour,  // Jobs expire from queue after 24 hours

		// GitLab API configuration (hardcoded for now)
		GitLabConfig: GitLabConfig{
			BaseURL: "https://git.apps.hexmos.com",
			PAT:     "REDACTED_GITLAB_PAT_5", // TODO: Make this dynamic/configurable
		},

		// Webhook configuration
		WebhookConfig: WebhookConfig{
			Secret:         "super-secret-string", // TODO: Make this dynamic/configurable
			PublicEndpoint: "",                    // Queried directly from DB during webhook install
			EnableSSL:      true,                  // Always verify SSL in production
			// Event configuration - only subscribe to events we need
			Events: WebhookEvents{
				PushEvents:          false, // Not needed for code review triggers
				IssuesEvents:        false, // Not needed for code review triggers
				MergeRequestsEvents: true,  // REQUIRED: For detecting MR events
				TagPushEvents:       false, // Not needed for code review triggers
				NoteEvents:          true,  // REQUIRED: For detecting @livereviewbot mentions
				JobEvents:           false, // Not needed for code review triggers
				PipelineEvents:      false, // Not needed for code review triggers
			},
		},
	}
}

// ProductionQueueConfig returns a configuration optimized for production use
func ProductionQueueConfig() *QueueConfig {
	config := DefaultQueueConfig()

	// Production optimizations
	config.MaxWorkers = 20                              // More workers for higher throughput
	config.JobTimeout = 10 * time.Minute                // Longer timeout for network issues
	config.RetryPolicy.MaxElapsedTime = 168 * time.Hour // Retry for a full week

	return config
}

// DevelopmentQueueConfig returns a configuration optimized for development
func DevelopmentQueueConfig() *QueueConfig {
	config := DefaultQueueConfig()

	// Development optimizations
	config.MaxWorkers = 3                             // Fewer workers to reduce resource usage
	config.MaxRetries = 5                             // Fail faster in development
	config.RetryPolicy.MaxElapsedTime = 1 * time.Hour // Give up quickly
	config.JobTimeout = 2 * time.Minute               // Shorter timeout for faster feedback

	return config
}

// GetQueueConfigWithDB returns the appropriate configuration based on environment,
// with the PublicEndpoint populated from the database
func GetQueueConfigWithDB(db *sql.DB) (*QueueConfig, error) {
	config := DefaultQueueConfig()

	// Set the public endpoint from database (may be empty if not configured yet)
	endpoint, err := getWebhookPublicEndpoint(db)
	if err != nil {
		return nil, err
	}
	config.WebhookConfig.PublicEndpoint = endpoint

	return config, nil
}

// ValidateWebhookEndpoint checks if the production URL is properly configured for webhook installation
// This should be called before attempting to install webhooks, not during queue initialization
func ValidateWebhookEndpoint(db *sql.DB) error {
	reverseProxy := os.Getenv("LIVEREVIEW_REVERSE_PROXY") == "true"

	if reverseProxy {
		// Production mode: Require livereview_prod_url to be set
		var prodURL sql.NullString
		err := db.QueryRow(`SELECT livereview_prod_url FROM instance_details ORDER BY id DESC LIMIT 1`).Scan(&prodURL)
		if err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("Production URL not set: please configure livereview_prod_url in settings before installing webhooks")
			}
			return fmt.Errorf("Error querying production URL: %v", err)
		}
		if !prodURL.Valid || strings.TrimSpace(prodURL.String) == "" {
			return fmt.Errorf("Production URL is empty: please configure livereview_prod_url in settings before installing webhooks")
		}
		// Validate URL format
		url := strings.TrimSpace(prodURL.String)
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			return fmt.Errorf("Production URL must start with http:// or https://")
		}
	}

	return nil
}

// GetQueueConfig returns the appropriate configuration based on environment
// TODO: Read from environment variable or config file
func GetQueueConfig() *QueueConfig {
	// For now, return default config
	// Later this can read from environment:
	// if os.Getenv("APP_ENV") == "production" {
	//     return ProductionQueueConfig()
	// } else if os.Getenv("APP_ENV") == "development" {
	//     return DevelopmentQueueConfig()
	// }
	return DefaultQueueConfig()
}

// RiverQueueConfig converts our config to River's queue configuration format
func (c *QueueConfig) RiverQueueConfig() map[string]river.QueueConfig {
	return map[string]river.QueueConfig{
		river.QueueDefault: {
			MaxWorkers: c.MaxWorkers,
		},
		// Future: Add more queues here for different job types
		// "priority": {MaxWorkers: c.MaxWorkers / 2}, // High priority queue
		// "batch": {MaxWorkers: c.MaxWorkers * 2},    // Batch processing queue
	}
}
