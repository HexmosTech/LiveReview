package github

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// TokenTypeGitHubApp marks an integration_tokens row that represents a GitHub App installation.
const TokenTypeGitHubApp = "GITHUB_APP"

// Repository scope of a GitHub App connector (metadata "repo_scope"). One installation can be connected to several
// orgs, one connector each.
const (
	// RepoScopeInstaller syncs every repository the installation covers. The install/configure callback creates it
	// (only someone allowed to install the App gets there), and connectors without a scope are installer ones.
	RepoScopeInstaller = "installer"
	// RepoScopeUser syncs only "allowed_repo_ids": the repositories the GitHub user who connected it can access.
	// Plain sign-ins that merely see an installation create it.
	RepoScopeUser = "user"
)

// ErrAppConnectorDisconnected is returned when an org disconnected this installation itself and the upsert asked
// not to reconnect it (plain sign-ins). Its repository scope is still refreshed, so the org's owner can connect it
// again in one click (ConnectAppConnector); installing or configuring the App again also reconnects it.
var ErrAppConnectorDisconnected = errors.New("github app installation was disconnected from this organization")

// ErrAppConnectorOffered is returned when the upsert asked to offer an installation another org already uses
// (OfferIfShared) instead of connecting it: the org gets a connector that is available but not connected (no token,
// no repositories) until its owner connects it (ConnectAppConnector).
var ErrAppConnectorOffered = errors.New("github app installation is used by another organization; offered, not connected")

type IntegrationTokenRecord struct {
	ID          int64
	Provider    string
	ProviderURL string
	PatToken    string
	Metadata    []byte
	OrgID       int64
	TokenType   string
	ExpiresAt   sql.NullTime
}

type TokenStore struct {
	db *sql.DB
}

func NewTokenStore(db *sql.DB) *TokenStore {
	return &TokenStore{db: db}
}

// GetGitHubTokenByConnectorID loads one GitHub connector by primary key. Callers must already know which
// connector (and so which org) a request belongs to - there is deliberately no "latest token" lookup.
func (s *TokenStore) GetGitHubTokenByConnectorID(connectorID int64) (*IntegrationTokenRecord, error) {
	const query = `
		SELECT id, provider, provider_url, COALESCE(pat_token, ''), COALESCE(metadata, '{}'), org_id,
		       COALESCE(token_type, ''), expires_at
		FROM integration_tokens
		WHERE id = $1 AND provider LIKE 'github%'
	`

	var rec IntegrationTokenRecord
	err := s.db.QueryRow(query, connectorID).Scan(
		&rec.ID, &rec.Provider, &rec.ProviderURL, &rec.PatToken, &rec.Metadata, &rec.OrgID,
		&rec.TokenType, &rec.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

const appConnectorColumns = `id, provider, provider_url, COALESCE(pat_token, ''), COALESCE(metadata, '{}'), org_id,
	COALESCE(token_type, ''), expires_at`

func scanRecord(row interface{ Scan(...interface{}) error }) (*IntegrationTokenRecord, error) {
	var rec IntegrationTokenRecord
	if err := row.Scan(&rec.ID, &rec.Provider, &rec.ProviderURL, &rec.PatToken, &rec.Metadata, &rec.OrgID,
		&rec.TokenType, &rec.ExpiresAt); err != nil {
		return nil, err
	}
	return &rec, nil
}

// AppConnectorUpsert describes one GitHub App installation to record for an org.
type AppConnectorUpsert struct {
	OrgID               int64
	BaseURL             string
	InstallationID      int64
	AccountLogin        string
	AccountType         string
	RepositorySelection string
	AppSlug             string
	Token               string
	ExpiresAt           time.Time
	// RepoScope is RepoScopeInstaller ("" means the same) or RepoScopeUser with AllowedRepoIDs. An installer
	// connector is never narrowed to a user one; a user connector becomes installer when the org installs or
	// configures the App.
	RepoScope      string
	AllowedRepoIDs []int64
	// KeepDisconnected leaves a connector the org disconnected (or was offered) not connected, only refreshing its
	// scope (ErrAppConnectorDisconnected / ErrAppConnectorOffered).
	KeepDisconnected bool
	// OfferIfShared records an installation the org has no live connector for, and another org already uses, as
	// available instead of connecting it (ErrAppConnectorOffered).
	OfferIfShared bool
}

// UpsertAppInstallationConnector inserts or refreshes the org's GITHUB_APP row for (BaseURL, InstallationID).
// Other orgs may have their own connector for the same installation. With KeepDisconnected or OfferIfShared the row
// may be written without being connected; the returned error then says which (the write is still committed).
func (s *TokenStore) UpsertAppInstallationConnector(in AppConnectorUpsert) (connectorID int64, created bool, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()

	appID := fmt.Sprintf("%d", in.InstallationID)
	fields := map[string]interface{}{
		"github_app":           true,
		"installation_id":      in.InstallationID,
		"account_login":        in.AccountLogin,
		"account_type":         in.AccountType,
		"repository_selection": in.RepositorySelection,
		"app_slug":             in.AppSlug,
		"removed":              false,
		"suspended":            false,
		"disconnected":         false,
		"offered":              false,
	}
	// offer turns fields into an available-but-not-connected connector: no token is stored.
	offer := func() {
		fields["removed"], fields["disconnected"], fields["offered"] = true, true, true
		in.Token, in.ExpiresAt = "", time.Time{}
	}
	sharedElsewhere := func() (bool, error) {
		var shared bool
		err := tx.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM integration_tokens
				WHERE token_type = $1 AND provider_url = $2 AND provider_app_id = $3 AND org_id <> $4
				  AND COALESCE(metadata->>'removed', 'false') <> 'true'
			)
		`, TokenTypeGitHubApp, in.BaseURL, appID, in.OrgID).Scan(&shared)
		return shared, err
	}
	var notConnected error // ErrAppConnectorDisconnected / ErrAppConnectorOffered once the row is written

	var existingMeta []byte
	err = tx.QueryRow(`
		SELECT id, COALESCE(metadata, '{}') FROM integration_tokens
		WHERE token_type = $1 AND provider_url = $2 AND provider_app_id = $3 AND org_id = $4
		FOR UPDATE
	`, TokenTypeGitHubApp, in.BaseURL, appID, in.OrgID).Scan(&connectorID, &existingMeta)
	switch {
	case err == nil:
		var prev struct {
			RepoScope    string `json:"repo_scope"`
			Removed      bool   `json:"removed"`
			Disconnected bool   `json:"disconnected"`
			Offered      bool   `json:"offered"`
		}
		_ = json.Unmarshal(existingMeta, &prev)
		switch {
		case in.KeepDisconnected && prev.Removed && prev.Disconnected:
			// Stays not connected (and offered stays as it was); only the scope below is refreshed.
			offer()
			fields["offered"] = prev.Offered
			notConnected = ErrAppConnectorDisconnected
			if prev.Offered {
				notConnected = ErrAppConnectorOffered
			}
		case in.OfferIfShared && prev.Removed:
			// Uninstalled before; installed again since, by someone in another org.
			shared, err := sharedElsewhere()
			if err != nil {
				return 0, false, err
			}
			if shared {
				offer()
				notConnected = ErrAppConnectorOffered
			}
		}
		// A user-scoped refresh of a live installer connector keeps it installer. A removed one starts over, so
		// a scope it had before an uninstall doesn't come back.
		scope := in.RepoScope
		if scope == RepoScopeUser && !prev.Removed && (prev.RepoScope == "" || prev.RepoScope == RepoScopeInstaller) {
			scope = RepoScopeInstaller
		}
		setScope(fields, scope, in.AllowedRepoIDs)
		meta, err := json.Marshal(fields)
		if err != nil {
			return 0, false, err
		}
		_, err = tx.Exec(`
			UPDATE integration_tokens
			SET pat_token = $1, expires_at = $2, connection_name = $3,
			    metadata = (COALESCE(metadata, '{}'::jsonb) - 'allowed_repo_ids') || $4::jsonb, updated_at = NOW()
			WHERE id = $5
		`, in.Token, nullTime(in.ExpiresAt), "GitHub App · "+in.AccountLogin, string(meta), connectorID)
		if err != nil {
			return 0, false, err
		}
	case errors.Is(err, sql.ErrNoRows):
		if in.OfferIfShared {
			shared, err := sharedElsewhere()
			if err != nil {
				return 0, false, err
			}
			if shared {
				offer()
				notConnected = ErrAppConnectorOffered
			}
		}
		setScope(fields, in.RepoScope, in.AllowedRepoIDs)
		meta, err := json.Marshal(fields)
		if err != nil {
			return 0, false, err
		}
		// The unique index uq_integration_tokens_github_app_installation_org turns a racing insert into an error.
		err = tx.QueryRow(`
			INSERT INTO integration_tokens
				(provider, provider_app_id, access_token, token_type, pat_token, expires_at, connection_name,
				 provider_url, org_id, metadata)
			VALUES ('github', $1, '', $2, $3, $4, $5, $6, $7, $8::jsonb)
			RETURNING id
		`, appID, TokenTypeGitHubApp, in.Token, nullTime(in.ExpiresAt), "GitHub App · "+in.AccountLogin, in.BaseURL, in.OrgID, string(meta)).Scan(&connectorID)
		if err != nil {
			return 0, false, err
		}
		created = true
	default:
		return 0, false, err
	}

	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return connectorID, created, notConnected
}

// nullTime stores a zero time as NULL (a connector that isn't connected has no token, so no expiry).
func nullTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

// setScope writes repo_scope (and allowed_repo_ids for a user scope) into the metadata fields.
func setScope(fields map[string]interface{}, scope string, allowed []int64) {
	if scope != RepoScopeUser {
		fields["repo_scope"] = RepoScopeInstaller
		return
	}
	if allowed == nil {
		allowed = []int64{}
	}
	fields["repo_scope"] = RepoScopeUser
	fields["allowed_repo_ids"] = allowed
}

// GetAppConnectorByInstallation finds the org's connector for an installation id on a GitHub host.
func (s *TokenStore) GetAppConnectorByInstallation(baseURL string, installationID, orgID int64) (*IntegrationTokenRecord, error) {
	return scanRecord(s.db.QueryRow(`
		SELECT `+appConnectorColumns+` FROM integration_tokens
		WHERE token_type = $1 AND provider_url = $2 AND provider_app_id = $3 AND org_id = $4
	`, TokenTypeGitHubApp, baseURL, fmt.Sprintf("%d", installationID), orgID))
}

// ListAppConnectorsByInstallation returns every org's connector for an installation id on a GitHub host, oldest
// first (removed and suspended ones included; callers filter).
func (s *TokenStore) ListAppConnectorsByInstallation(baseURL string, installationID int64) ([]IntegrationTokenRecord, error) {
	return s.listRecords(`
		SELECT `+appConnectorColumns+` FROM integration_tokens
		WHERE token_type = $1 AND provider_url = $2 AND provider_app_id = $3
		ORDER BY id
	`, TokenTypeGitHubApp, baseURL, fmt.Sprintf("%d", installationID))
}

// CountOtherOrgsUsingInstallation counts the other orgs with a live (not removed) connector for the installation.
func (s *TokenStore) CountOtherOrgsUsingInstallation(baseURL string, installationID, orgID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM integration_tokens
		WHERE token_type = $1 AND provider_url = $2 AND provider_app_id = $3 AND org_id <> $4
		  AND COALESCE(metadata->>'removed', 'false') <> 'true'
	`, TokenTypeGitHubApp, baseURL, fmt.Sprintf("%d", installationID), orgID).Scan(&n)
	return n, err
}

// OtherOrgOwnerEmailsForInstallation returns the emails of the active owners of the other orgs with a live connector
// for the installation, sorted and without duplicates. Callers show them masked only.
func (s *TokenStore) OtherOrgOwnerEmailsForInstallation(baseURL string, installationID, orgID int64) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT LOWER(u.email)
		FROM integration_tokens it
		JOIN user_roles ur ON ur.org_id = it.org_id
		JOIN roles r ON r.id = ur.role_id AND r.name = 'owner'
		JOIN users u ON u.id = ur.user_id AND u.is_active
		WHERE it.token_type = $1 AND it.provider_url = $2 AND it.provider_app_id = $3 AND it.org_id <> $4
		  AND COALESCE(it.metadata->>'removed', 'false') <> 'true'
		ORDER BY 1
	`, TokenTypeGitHubApp, baseURL, fmt.Sprintf("%d", installationID), orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	emails := []string{}
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		emails = append(emails, e)
	}
	return emails, rows.Err()
}

// LatestReviewConnector returns which of the connectors ran the most recent review of the PR/MR at prURL;
// sql.ErrNoRows when none of them reviewed it.
func (s *TokenStore) LatestReviewConnector(connectorIDs []int64, prURL string) (int64, error) {
	var id int64
	err := s.db.QueryRow(`
		SELECT connector_id FROM reviews
		WHERE connector_id = ANY($1) AND (pr_mr_url = $2 OR starts_with(pr_mr_url, $2 || '/'))
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, pq.Array(connectorIDs), prURL).Scan(&id)
	return id, err
}

// PreferredSiblingConnector says which connector should answer an event for a repo that several orgs connected with
// a token from the same provider account. Each org has its own webhook, so every one of them sees the event.
// It is the connector that reviewed prURL most recently, else the oldest. It returns connectorID itself when it has
// no such siblings (or no saved account profile), so a different answer only ever means "defer to another org".
func (s *TokenStore) PreferredSiblingConnector(connectorID, orgID int64, repoProviderID, repoFullName, prURL string) (int64, error) {
	// Deliberately spans orgs, like LatestReviewConnector: the caller only compares the returned id with its own.
	rows, err := s.db.Query(`
		WITH accounts AS (
			SELECT id, org_id, provider, provider_url,
			       COALESCE(metadata->'githubProfile'->>'id', metadata->'gitlabProfile'->>'id',
			                metadata->'giteaProfile'->>'id', metadata->'bitbucketProfile'->>'account_id') AS account_id
			FROM integration_tokens
			WHERE token_type = 'PAT'
		)
		SELECT DISTINCT other.id
		FROM accounts me
		JOIN accounts other ON other.provider = me.provider AND other.provider_url = me.provider_url
		                   AND other.account_id = me.account_id
		JOIN repositories r ON r.connector_id = other.id
		WHERE me.id = $1 AND me.org_id = $2 AND me.account_id IS NOT NULL
		  AND (($3 <> '' AND r.provider_repo_id = $3) OR ($4 <> '' AND LOWER(r.full_name) = LOWER($4)))
		ORDER BY other.id
	`, connectorID, orgID, repoProviderID, repoFullName)
	if err != nil {
		return connectorID, err
	}
	defer rows.Close()
	var ids []int64
	includesSelf := false
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return connectorID, err
		}
		ids = append(ids, id)
		includesSelf = includesSelf || id == connectorID
	}
	if err := rows.Err(); err != nil {
		return connectorID, err
	}
	// Fail open: if this connector doesn't list the repo itself, don't let a sibling silence it.
	if len(ids) < 2 || !includesSelf {
		return connectorID, nil
	}
	if prURL != "" {
		id, err := s.LatestReviewConnector(ids, prURL)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return connectorID, err
		}
	}
	return ids[0], nil
}

// UpdateCachedInstallationToken stores a freshly minted installation token on its connector.
func (s *TokenStore) UpdateCachedInstallationToken(connectorID int64, token string, expiresAt time.Time) error {
	_, err := s.db.Exec(`
		UPDATE integration_tokens SET pat_token = $1, expires_at = $2, updated_at = NOW()
		WHERE id = $3 AND token_type = $4
	`, token, expiresAt, connectorID, TokenTypeGitHubApp)
	return err
}

// ListAppConnectorsExpiringBefore returns live (not removed/suspended) App connectors whose cached token
// expires before t, or has never been set.
func (s *TokenStore) ListAppConnectorsExpiringBefore(t time.Time) ([]IntegrationTokenRecord, error) {
	return s.listRecords(`
		SELECT `+appConnectorColumns+` FROM integration_tokens
		WHERE token_type = $1
		  AND COALESCE(metadata->>'removed', 'false') <> 'true'
		  AND COALESCE(metadata->>'suspended', 'false') <> 'true'
		  AND (expires_at IS NULL OR expires_at < $2)
	`, TokenTypeGitHubApp, t)
}

// ListAppConnectorsForOrg returns the org's App connectors that have not been removed.
func (s *TokenStore) ListAppConnectorsForOrg(orgID int64) ([]IntegrationTokenRecord, error) {
	return s.listRecords(`
		SELECT `+appConnectorColumns+` FROM integration_tokens
		WHERE token_type = $1 AND org_id = $2 AND COALESCE(metadata->>'removed', 'false') <> 'true'
		ORDER BY id
	`, TokenTypeGitHubApp, orgID)
}

func (s *TokenStore) listRecords(query string, args ...interface{}) ([]IntegrationTokenRecord, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IntegrationTokenRecord
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

// SetAppConnectorFlags records that an installation was removed and/or suspended. Removing it also clears the
// cached token so nothing can keep using it, and ends a disconnected or offered state: the installation itself is
// gone, so there is nothing left to connect.
func (s *TokenStore) SetAppConnectorFlags(connectorID int64, removed, suspended *bool) error {
	flags := map[string]bool{}
	if removed != nil {
		flags["removed"] = *removed
		if *removed {
			flags["disconnected"], flags["offered"] = false, false
		}
	}
	if suspended != nil {
		flags["suspended"] = *suspended
	}
	if len(flags) == 0 {
		return nil
	}
	b, err := json.Marshal(flags)
	if err != nil {
		return err
	}
	clearToken := removed != nil && *removed
	_, err = s.db.Exec(`
		UPDATE integration_tokens
		SET metadata = COALESCE(metadata, '{}'::jsonb) || $1::jsonb,
		    pat_token = CASE WHEN $2 THEN '' ELSE pat_token END,
		    updated_at = NOW()
		WHERE id = $3 AND token_type = $4
	`, string(b), clearToken, connectorID, TokenTypeGitHubApp)
	return err
}

// DisconnectAppConnector stops one org using an installation without uninstalling it: the connector is marked
// removed and disconnected (plain sign-ins then leave it alone) and its cached token is cleared.
func (s *TokenStore) DisconnectAppConnector(connectorID int64) error {
	_, err := s.db.Exec(`
		UPDATE integration_tokens
		SET metadata = COALESCE(metadata, '{}'::jsonb) || '{"removed": true, "disconnected": true, "offered": false}'::jsonb,
		    pat_token = '', updated_at = NOW()
		WHERE id = $1 AND token_type = $2
	`, connectorID, TokenTypeGitHubApp)
	return err
}

// ListNotConnectedAppConnectorsForOrg returns the org's App connectors that are available but not connected: ones
// it disconnected, and ones it was offered because another org already uses the installation. Suspended ones are
// left out (they can't be connected until GitHub unsuspends them).
func (s *TokenStore) ListNotConnectedAppConnectorsForOrg(orgID int64) ([]IntegrationTokenRecord, error) {
	return s.listRecords(`
		SELECT `+appConnectorColumns+` FROM integration_tokens
		WHERE token_type = $1 AND org_id = $2
		  AND COALESCE(metadata->>'removed', 'false') = 'true'
		  AND COALESCE(metadata->>'disconnected', 'false') = 'true'
		  AND COALESCE(metadata->>'suspended', 'false') <> 'true'
		ORDER BY id
	`, TokenTypeGitHubApp, orgID)
}

// ConnectAppConnector connects an available connector (disconnected or offered) with a freshly minted installation
// token, keeping the repository scope recorded at the org's last GitHub sign-in. sql.ErrNoRows when the connector is
// not in that state (already connected, uninstalled, or not a GitHub App connector).
func (s *TokenStore) ConnectAppConnector(connectorID int64, token string, expiresAt time.Time) error {
	var id int64
	return s.db.QueryRow(`
		UPDATE integration_tokens
		SET metadata = COALESCE(metadata, '{}'::jsonb) || '{"removed": false, "disconnected": false, "offered": false}'::jsonb,
		    pat_token = $1, expires_at = $2, updated_at = NOW()
		WHERE id = $3 AND token_type = $4
		  AND COALESCE(metadata->>'removed', 'false') = 'true'
		  AND COALESCE(metadata->>'disconnected', 'false') = 'true'
		RETURNING id
	`, token, expiresAt, connectorID, TokenTypeGitHubApp).Scan(&id)
}

// FindPreferredConnectorForRepo returns the org's GitHub connector that has the repository, preferring a
// GitHub App connector over a PAT one (the App wins on overlap). sql.ErrNoRows means no connector lists it.
func (s *TokenStore) FindPreferredConnectorForRepo(orgID int64, repoFullName string) (int64, error) {
	var id int64
	err := s.db.QueryRow(`
		SELECT it.id
		FROM integration_tokens it
		JOIN repositories r ON r.connector_id = it.id
		WHERE it.org_id = $1 AND it.provider LIKE 'github%' AND LOWER(r.full_name) = LOWER($2)
		  AND COALESCE(it.metadata->>'removed', 'false') <> 'true'
		ORDER BY (it.token_type = $3) DESC, it.created_at DESC
		LIMIT 1
	`, orgID, repoFullName, TokenTypeGitHubApp).Scan(&id)
	return id, err
}

// HasAppConnectorForRepo reports whether the org has a live GitHub App connector that lists the repository.
func (s *TokenStore) HasAppConnectorForRepo(orgID int64, repoFullName string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM integration_tokens it
			JOIN repositories r ON r.connector_id = it.id
			WHERE it.org_id = $1 AND it.token_type = $2 AND LOWER(r.full_name) = LOWER($3)
			  AND COALESCE(it.metadata->>'removed', 'false') <> 'true'
			  AND COALESCE(it.metadata->>'suspended', 'false') <> 'true'
		)
	`, orgID, TokenTypeGitHubApp, repoFullName).Scan(&exists)
	return exists, err
}

// HasAppConnectorForOwnerAllRepos reports whether the org has a live installer GitHub App connector installed on the
// account with access to ALL of its repositories (a user-scoped one only covers what its user can access). The
// repositories table is filled by a background sync after install, so this answers right away, before the sync
// lists a repository.
func (s *TokenStore) HasAppConnectorForOwnerAllRepos(orgID int64, owner string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM integration_tokens it
			WHERE it.org_id = $1 AND it.token_type = $2
			  AND LOWER(it.metadata->>'account_login') = LOWER($3)
			  AND it.metadata->>'repository_selection' = 'all'
			  AND COALESCE(it.metadata->>'repo_scope', 'installer') <> 'user'
			  AND COALESCE(it.metadata->>'removed', 'false') <> 'true'
			  AND COALESCE(it.metadata->>'suspended', 'false') <> 'true'
		)
	`, orgID, TokenTypeGitHubApp, owner).Scan(&exists)
	return exists, err
}
