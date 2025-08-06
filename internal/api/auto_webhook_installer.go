package api

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/livereview/internal/providers/gitlab"
)

// AutoWebhookInstaller handles automatic webhook installation for new connectors
type AutoWebhookInstaller struct {
	db       *sql.DB
	server   *Server
	jobQueue JobQueueInterface
}

// JobQueueInterface allows for easier testing and decoupling
type JobQueueInterface interface {
	QueueWebhookInstallJob(ctx context.Context, connectorID int, projectPath, provider, baseURL, pat string) error
}

// NewAutoWebhookInstaller creates a new auto webhook installer
func NewAutoWebhookInstaller(db *sql.DB, server *Server, jobQueue JobQueueInterface) *AutoWebhookInstaller {
	return &AutoWebhookInstaller{
		db:       db,
		server:   server,
		jobQueue: jobQueue,
	}
}

// TriggerAutoInstallation starts the background process for automatic webhook installation
// This should be called immediately after a new connector is successfully created
func (awi *AutoWebhookInstaller) TriggerAutoInstallation(connectorID int) {
	log.Printf("Starting background auto-installation for connector %d", connectorID)

	// Start goroutine for background processing
	go func() {
		ctx := context.Background()

		// Add a small delay to ensure the connector creation transaction is fully committed
		time.Sleep(1 * time.Second)

		err := awi.processAutoInstallation(ctx, connectorID)
		if err != nil {
			log.Printf("Auto-installation failed for connector %d: %v", connectorID, err)
			// Store error in database for debugging
			awi.recordAutoInstallationError(connectorID, err)
		} else {
			log.Printf("Auto-installation completed successfully for connector %d", connectorID)
		}
	}()
}

// processAutoInstallation handles the complete auto-installation process
func (awi *AutoWebhookInstaller) processAutoInstallation(ctx context.Context, connectorID int) error {
	log.Printf("Processing auto-installation for connector %d", connectorID)

	// Step 1: Get connector details
	connector, err := awi.getConnectorDetails(connectorID)
	if err != nil {
		return fmt.Errorf("failed to get connector details: %w", err)
	}

	// Step 2: Check if this connector should have auto-installation
	if !awi.shouldAutoInstall(connector) {
		log.Printf("Skipping auto-installation for connector %d (provider: %s)", connectorID, connector.Provider)
		return nil
	}

	// Step 3: Discover projects and cache them
	projects, err := awi.discoverAndCacheProjects(connectorID, connector)
	if err != nil {
		return fmt.Errorf("failed to discover projects: %w", err)
	}

	if len(projects) == 0 {
		log.Printf("No projects found for connector %d, skipping webhook installation", connectorID)
		return nil
	}

	log.Printf("Discovered %d projects for connector %d, starting webhook installation", len(projects), connectorID)

	// Step 4: Queue webhook installation jobs for all projects
	return awi.queueWebhookInstallations(ctx, connectorID, projects, connector)
}

// ConnectorDetails holds the essential connector information
type ConnectorDetails struct {
	ID          int
	Provider    string
	ProviderURL string
	PATToken    string
}

// getConnectorDetails retrieves the necessary connector information
func (awi *AutoWebhookInstaller) getConnectorDetails(connectorID int) (*ConnectorDetails, error) {
	var connector ConnectorDetails

	query := `
		SELECT id, provider, provider_url, pat_token
		FROM integration_tokens
		WHERE id = $1
	`

	err := awi.db.QueryRow(query, connectorID).Scan(
		&connector.ID,
		&connector.Provider,
		&connector.ProviderURL,
		&connector.PATToken,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("connector %d not found", connectorID)
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	return &connector, nil
}

// shouldAutoInstall determines if a connector should have automatic webhook installation
func (awi *AutoWebhookInstaller) shouldAutoInstall(connector *ConnectorDetails) bool {
	// Only auto-install for GitLab providers
	if connector.Provider != "gitlab" &&
		connector.Provider != "gitlab-com" &&
		connector.Provider != "gitlab-self-hosted" {
		return false
	}

	// Only auto-install if we have a PAT token
	if connector.PATToken == "" {
		log.Printf("Connector %d has no PAT token, skipping auto-installation", connector.ID)
		return false
	}

	return true
}

// discoverAndCacheProjects discovers all projects for the connector and caches them
func (awi *AutoWebhookInstaller) discoverAndCacheProjects(connectorID int, connector *ConnectorDetails) ([]string, error) {
	log.Printf("Starting project discovery for connector %d", connectorID)

	// Use the existing GitLab project discovery function
	projects, err := gitlab.DiscoverProjectsGitlab(connector.ProviderURL, connector.PATToken)
	if err != nil {
		return nil, fmt.Errorf("GitLab project discovery failed: %w", err)
	}

	log.Printf("Discovered %d projects for connector %d", len(projects), connectorID)

	// Cache the projects using the existing server method
	if awi.server != nil {
		response := &RepositoryAccessResponse{
			ConnectorID:  connectorID,
			Provider:     connector.Provider,
			BaseURL:      connector.ProviderURL,
			Projects:     projects,
			ProjectCount: len(projects),
			UpdatedAt:    time.Now(),
		}

		awi.server.updateProjectsCache(connectorID, response)
		log.Printf("Cached %d projects for connector %d", len(projects), connectorID)
	}

	return projects, nil
}

// queueWebhookInstallations queues webhook installation jobs for all projects
func (awi *AutoWebhookInstaller) queueWebhookInstallations(ctx context.Context, connectorID int, projects []string, connector *ConnectorDetails) error {
	log.Printf("Queueing webhook installation jobs for %d projects (connector %d)", len(projects), connectorID)

	successCount := 0
	var lastError error

	for _, projectPath := range projects {
		err := awi.jobQueue.QueueWebhookInstallJob(
			ctx,
			connectorID,
			projectPath,
			connector.Provider,
			connector.ProviderURL,
			connector.PATToken,
		)

		if err != nil {
			log.Printf("Failed to queue webhook job for project %s (connector %d): %v", projectPath, connectorID, err)
			lastError = err
		} else {
			successCount++
		}
	}

	log.Printf("Successfully queued %d/%d webhook installation jobs for connector %d", successCount, len(projects), connectorID)

	// If we couldn't queue any jobs, return the last error
	if successCount == 0 && lastError != nil {
		return fmt.Errorf("failed to queue any webhook installation jobs: %w", lastError)
	}

	// If we queued some but not all, log a warning but don't fail
	if successCount < len(projects) {
		log.Printf("Warning: Only queued %d/%d webhook jobs for connector %d", successCount, len(projects), connectorID)
	}

	return nil
}

// recordAutoInstallationError records auto-installation errors for debugging
func (awi *AutoWebhookInstaller) recordAutoInstallationError(connectorID int, err error) {
	// We could add an auto_installation_log table, but for now just log to application logs
	log.Printf("Auto-installation error for connector %d: %v", connectorID, err)

	// Update metadata if possible (non-critical operation)
	// This would help with debugging and showing status in the UI
	query := `
		UPDATE integration_tokens 
		SET metadata = COALESCE(metadata, '{}') || $1
		WHERE id = $2
	`

	errorJSON := fmt.Sprintf(`{"auto_installation_error": "%s", "auto_installation_time": "%s"}`,
		err.Error(), time.Now().Format(time.RFC3339))

	_, updateErr := awi.db.Exec(query, errorJSON, connectorID)
	if updateErr != nil {
		log.Printf("Failed to record auto-installation error in metadata for connector %d: %v", connectorID, updateErr)
	}
}
