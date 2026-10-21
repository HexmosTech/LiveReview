package jobqueue

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/riverqueue/river"

	"github.com/livereview/internal/githubapp"
	storagegithub "github.com/livereview/storage/providers/github"
)

// githubAppTokenRefreshInterval is how often cached installation tokens are checked; tokens live an hour and
// are renewed when they expire within githubAppTokenRefreshWindow, so a missed run still leaves slack.
const (
	githubAppTokenRefreshInterval = 10 * time.Minute
	githubAppTokenRefreshWindow   = 20 * time.Minute
)

// GitHubAppTokenRefreshJobArgs keeps integration_tokens.pat_token fresh for GitHub App connectors. Hot paths
// resolve tokens on demand (githubapp.ConnectorToken); this job covers the many legacy readers that select
// pat_token straight from the table.
type GitHubAppTokenRefreshJobArgs struct{}

func (GitHubAppTokenRefreshJobArgs) Kind() string { return "github_app_token_refresh" }

type GitHubAppTokenRefreshWorker struct {
	river.WorkerDefaults[GitHubAppTokenRefreshJobArgs]
	db *sql.DB
}

func (w *GitHubAppTokenRefreshWorker) Work(ctx context.Context, job *river.Job[GitHubAppTokenRefreshJobArgs]) error {
	store := storagegithub.NewTokenStore(w.db)
	records, err := store.ListAppConnectorsExpiringBefore(time.Now().Add(githubAppTokenRefreshWindow))
	if err != nil {
		return fmt.Errorf("list expiring github app connectors: %w", err)
	}
	refreshed := 0
	for i := range records {
		if _, err := githubapp.RefreshConnectorToken(ctx, store, &records[i]); err != nil {
			// One broken installation must not stop the others; the next run retries it.
			log.Printf("[GitHubApp] token refresh failed for connector %d: %v", records[i].ID, err)
			continue
		}
		refreshed++
	}
	if len(records) > 0 {
		log.Printf("[GitHubApp] refreshed %d/%d installation tokens", refreshed, len(records))
	}
	return nil
}
