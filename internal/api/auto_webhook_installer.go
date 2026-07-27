package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/livereview/internal/providers"
	"github.com/livereview/internal/providers/azuredevops"
	"github.com/livereview/internal/providers/bitbucket"
	"github.com/livereview/internal/providers/gitea"
	"github.com/livereview/internal/providers/github"
	"github.com/livereview/internal/providers/gitlab"
	"github.com/livereview/storage/providers/pullrequests"
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
	QueueRepoPRSyncJob(ctx context.Context, repositoryID int64, initialBackfill bool) error
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

	// Step 3.5: Populate the repositories table and queue an initial PR/MR
	// backfill sync for each repo (GitHub/GitLab only - the unified repo/PR
	// listing feature is not yet built out for other providers). Best-effort:
	// failures here are logged but never fail the connector setup, since the
	// pre-existing webhook installation flow (Step 4) must not regress.
	if strings.HasPrefix(connector.Provider, "github") || strings.HasPrefix(connector.Provider, "gitlab") {
		if err := awi.syncRepositoriesAndQueueBackfill(ctx, connectorID, connector); err != nil {
			log.Printf("Warning: repository/PR backfill setup failed for connector %d: %v", connectorID, err)
		}
	}

	// Step 4: Queue webhook installation jobs for all projects
	return awi.queueWebhookInstallations(ctx, connectorID, projects, connector)
}

// syncRepositoriesAndQueueBackfill discovers repositories with full metadata
// (unlike the name-only discoverAndCacheProjects), upserts them into the
// repositories table, and queues an initial PR/MR backfill sync job for each
// one - so a newly connected org/group is immediately populated instead of
// waiting for the first periodic reconciliation sweep.
func (awi *AutoWebhookInstaller) syncRepositoriesAndQueueBackfill(ctx context.Context, connectorID int, connector *ConnectorDetails) error {
	store := pullrequests.NewStore(awi.db)

	cursor := ""
	total := 0
	for {
		var page *providers.RepositoryPage
		var err error
		switch {
		case strings.HasPrefix(connector.Provider, "github"):
			page, err = github.ListRepositories(ctx, connector.ProviderURL, connector.PATToken, cursor)
		case strings.HasPrefix(connector.Provider, "gitlab"):
			page, err = gitlab.ListRepositories(ctx, connector.ProviderURL, connector.PATToken, cursor)
		default:
			return fmt.Errorf("unsupported provider for repository sync: %s", connector.Provider)
		}
		if err != nil {
			return fmt.Errorf("failed to list repositories: %w", err)
		}

		for _, repo := range page.Repositories {
			repositoryID, err := store.UpsertRepository(pullrequests.RepositoryUpsert{
				OrgID:          connector.OrgID,
				ConnectorID:    int64(connectorID),
				Provider:       connector.Provider,
				ProviderRepoID: repo.ProviderRepoID,
				FullName:       repo.FullName,
				Name:           repo.Name,
				WebURL:         repo.WebURL,
				CloneURL:       repo.CloneURL,
				SSHURL:         repo.SSHURL,
				DefaultBranch:  repo.DefaultBranch,
				IsPrivate:      repo.IsPrivate,
				Description:    repo.Description,
			})
			if err != nil {
				log.Printf("Warning: failed to upsert repository %s for connector %d: %v", repo.FullName, connectorID, err)
				continue
			}
			if err := awi.jobQueue.QueueRepoPRSyncJob(ctx, repositoryID, true); err != nil {
				log.Printf("Warning: failed to queue initial PR backfill for repository %s (id=%d): %v", repo.FullName, repositoryID, err)
				continue
			}
			total++
		}

		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}

	log.Printf("Queued initial PR/MR backfill for %d repositories (connector %d)", total, connectorID)
	return nil
}

// ConnectorDetails holds the essential connector information
type ConnectorDetails struct {
	ID          int
	OrgID       int64
	Provider    string
	ProviderURL string
	PATToken    string
	Metadata    map[string]interface{}
}

// getConnectorDetails retrieves the necessary connector information
func (awi *AutoWebhookInstaller) getConnectorDetails(connectorID int) (*ConnectorDetails, error) {
	var connector ConnectorDetails
	var metadataBytes []byte

	query := `
		SELECT id, org_id, provider, provider_url, pat_token, COALESCE(metadata, '{}')
		FROM integration_tokens
		WHERE id = $1
	`

	err := awi.db.QueryRow(query, connectorID).Scan(
		&connector.ID,
		&connector.OrgID,
		&connector.Provider,
		&connector.ProviderURL,
		&connector.PATToken,
		&metadataBytes,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("connector %d not found", connectorID)
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Parse metadata JSON
	if len(metadataBytes) > 0 {
		var metadata map[string]interface{}
		if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
			log.Printf("Warning: failed to parse metadata for connector %d: %v", connectorID, err)
			connector.Metadata = make(map[string]interface{})
		} else {
			connector.Metadata = metadata
		}
	} else {
		connector.Metadata = make(map[string]interface{})
	}

	return &connector, nil
}

// shouldAutoInstall determines if a connector should have automatic webhook installation
func (awi *AutoWebhookInstaller) shouldAutoInstall(connector *ConnectorDetails) bool {
	// Auto-install for GitLab, GitHub, and Gitea providers
	isGitLab := connector.Provider == "gitlab" ||
		connector.Provider == "gitlab-com" ||
		connector.Provider == "gitlab-self-hosted"

	isGitHub := connector.Provider == "github" ||
		connector.Provider == "github-com" ||
		connector.Provider == "github-enterprise"

	isGitea := connector.Provider == "gitea"

	isAzureDevOps := strings.HasPrefix(connector.Provider, "azuredevops")

	if !isGitLab && !isGitHub && !isGitea && !isAzureDevOps {
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

	var projects []string
	var err error

	// Use the appropriate project discovery function based on provider
	if strings.HasPrefix(connector.Provider, "gitlab") {
		// Use the existing GitLab project discovery function
		projects, err = gitlab.DiscoverProjectsGitlab(connector.ProviderURL, connector.PATToken)
	} else if strings.HasPrefix(connector.Provider, "github") {
		// Use the GitHub project discovery function
		projects, err = github.DiscoverProjectsGitHub(connector.ProviderURL, connector.PATToken)
	} else if strings.HasPrefix(connector.Provider, "gitea") {
		// Use the Gitea project discovery function
		projects, err = gitea.DiscoverProjectsGitea(connector.ProviderURL, connector.PATToken)
	} else if strings.HasPrefix(connector.Provider, "bitbucket") {
		// Use the Bitbucket project discovery function
		// For Bitbucket, we need email from metadata
		email, ok := connector.Metadata["email"].(string)
		if !ok || email == "" {
			return nil, fmt.Errorf("bitbucket connector missing email in metadata")
		}
		projects, err = bitbucket.DiscoverProjectsBitbucket(connector.ProviderURL, email, connector.PATToken)
	} else if strings.HasPrefix(connector.Provider, "azuredevops") {
		// Use the Azure DevOps project discovery function
		projects, err = azuredevops.DiscoverProjectsAzureDevOps(connector.ProviderURL, connector.PATToken)
	} else {
		return nil, fmt.Errorf("unsupported provider: %s", connector.Provider)
	}

	if err != nil {
		return nil, fmt.Errorf("project discovery failed for %s: %w", connector.Provider, err)
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
