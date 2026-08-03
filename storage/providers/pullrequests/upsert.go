// Package pullrequests provides the single, shared upsert path for the
// repositories and pull_requests tables, used identically by the webhook-driven
// sync path and the periodic-reconciliation poll path so the two can never
// disagree on how a repo/PR row gets written.
package pullrequests

import (
	"database/sql"
	"encoding/json"
	"time"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// RepositoryUpsert is the input to UpsertRepository / EnsureRepositoryStub.
type RepositoryUpsert struct {
	OrgID          int64
	ConnectorID    int64
	Provider       string
	ProviderRepoID string
	FullName       string
	Name           string
	WebURL         string
	CloneURL       string
	SSHURL         string
	DefaultBranch  string
	IsPrivate      bool
	Description    string
}

// UpsertRepository inserts or updates a repositories row keyed by
// (connector_id, provider_repo_id). Discovery snapshots are always a full,
// self-consistent picture, so no staleness guard is needed here (unlike
// UpsertPullRequest).
func (s *Store) UpsertRepository(r RepositoryUpsert) (int64, error) {
	const q = `
		INSERT INTO repositories (
			org_id, connector_id, provider, provider_repo_id, full_name, name,
			web_url, clone_url, ssh_url, default_branch, is_private, description,
			last_synced_at, last_sync_status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12, now(), 'ok')
		ON CONFLICT (connector_id, provider_repo_id) DO UPDATE SET
			full_name = EXCLUDED.full_name,
			name = EXCLUDED.name,
			web_url = EXCLUDED.web_url,
			clone_url = EXCLUDED.clone_url,
			ssh_url = EXCLUDED.ssh_url,
			default_branch = EXCLUDED.default_branch,
			is_private = EXCLUDED.is_private,
			description = EXCLUDED.description,
			last_synced_at = now(),
			last_sync_status = 'ok',
			updated_at = now()
		RETURNING id
	`
	var id int64
	err := s.db.QueryRow(q, r.OrgID, r.ConnectorID, r.Provider, r.ProviderRepoID, r.FullName, r.Name,
		r.WebURL, r.CloneURL, r.SSHURL, r.DefaultBranch, r.IsPrivate, r.Description).Scan(&id)
	return id, err
}

// EnsureRepositoryStub inserts a minimal repositories row (last_sync_status =
// 'pending') from webhook payload fields if one doesn't already exist for
// (connector_id, provider_repo_id). Used when a PR/MR webhook arrives before
// the connector's discovery/backfill sync has created the repositories row;
// the later backfill's UpsertRepository call fills in the rest. It never
// overwrites an existing row.
func (s *Store) EnsureRepositoryStub(r RepositoryUpsert) (int64, error) {
	const q = `
		INSERT INTO repositories (
			org_id, connector_id, provider, provider_repo_id, full_name, name, web_url, last_sync_status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,'pending')
		ON CONFLICT (connector_id, provider_repo_id) DO UPDATE SET id = repositories.id
		RETURNING id
	`
	var id int64
	err := s.db.QueryRow(q, r.OrgID, r.ConnectorID, r.Provider, r.ProviderRepoID, r.FullName, r.Name, r.WebURL).Scan(&id)
	return id, err
}

// PullRequestUpsert is the input to UpsertPullRequest.
type PullRequestUpsert struct {
	RepositoryID      int64
	OrgID             int64
	Provider          string
	ProviderPRID      string
	Number            int
	Title             string
	Description       string
	State             string // "open" | "closed" | "merged" - already normalized via internal/prsync
	AuthorID          string
	AuthorUsername    string
	AuthorName        string
	AuthorAvatarURL   string
	SourceBranch      string
	TargetBranch      string
	WebURL            string
	ProviderCreatedAt time.Time
	ProviderUpdatedAt time.Time
	LastSyncedSource  string // "webhook" | "poll" | "backfill"
	Metadata          map[string]interface{}
}

// UpsertPullRequest inserts or updates a pull_requests row keyed by
// (repository_id, provider_pr_id). On conflict, the write is only applied if
// the incoming provider_updated_at is at least as new as what's already
// stored, so a stale/delayed webhook delivery can never clobber newer state
// already recorded by a faster delivery or a poll. Postgres's ON CONFLICT
// takes the necessary row lock atomically, so no explicit locking is needed.
func (s *Store) UpsertPullRequest(pr PullRequestUpsert) (int64, error) {
	metadata := pr.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return 0, err
	}

	var providerCreatedAt, providerUpdatedAt interface{}
	if !pr.ProviderCreatedAt.IsZero() {
		providerCreatedAt = pr.ProviderCreatedAt
	}
	if !pr.ProviderUpdatedAt.IsZero() {
		providerUpdatedAt = pr.ProviderUpdatedAt
	}

	lastSyncedSource := pr.LastSyncedSource
	if lastSyncedSource == "" {
		lastSyncedSource = "poll"
	}

	const q = `
		INSERT INTO pull_requests (
			repository_id, org_id, provider, provider_pr_id, number, title, description, state,
			author_id, author_username, author_name, author_avatar_url,
			source_branch, target_branch, web_url,
			provider_created_at, provider_updated_at, last_synced_at, last_synced_source, metadata
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17, now(), $18, $19)
		ON CONFLICT (repository_id, provider_pr_id) DO UPDATE SET
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			state = EXCLUDED.state,
			author_id = EXCLUDED.author_id,
			author_username = EXCLUDED.author_username,
			author_name = EXCLUDED.author_name,
			author_avatar_url = EXCLUDED.author_avatar_url,
			source_branch = EXCLUDED.source_branch,
			target_branch = EXCLUDED.target_branch,
			web_url = EXCLUDED.web_url,
			provider_updated_at = EXCLUDED.provider_updated_at,
			last_synced_at = now(),
			last_synced_source = EXCLUDED.last_synced_source,
			metadata = EXCLUDED.metadata,
			updated_at = now()
		WHERE pull_requests.provider_updated_at IS NULL
		   OR EXCLUDED.provider_updated_at >= pull_requests.provider_updated_at
		RETURNING id
	`
	var id int64
	err = s.db.QueryRow(q,
		pr.RepositoryID, pr.OrgID, pr.Provider, pr.ProviderPRID, pr.Number, pr.Title, pr.Description, pr.State,
		pr.AuthorID, pr.AuthorUsername, pr.AuthorName, pr.AuthorAvatarURL,
		pr.SourceBranch, pr.TargetBranch, pr.WebURL,
		providerCreatedAt, providerUpdatedAt, lastSyncedSource, metadataJSON,
	).Scan(&id)

	if err == sql.ErrNoRows {
		// The WHERE guard rejected this write because the stored row already has
		// a provider_updated_at at least as new as this one - a stale/delayed
		// delivery, not an error. The existing row is authoritative; look up its
		// id so callers (e.g. the webhook handler resolving a repositories stub)
		// can still proceed.
		return s.getExistingPullRequestID(pr.RepositoryID, pr.ProviderPRID)
	}
	return id, err
}

func (s *Store) getExistingPullRequestID(repositoryID int64, providerPRID string) (int64, error) {
	var id int64
	err := s.db.QueryRow(
		`SELECT id FROM pull_requests WHERE repository_id = $1 AND provider_pr_id = $2`,
		repositoryID, providerPRID,
	).Scan(&id)
	return id, err
}
