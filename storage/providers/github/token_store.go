package github

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// TokenTypeGitHubApp marks an integration_tokens row that represents a GitHub App installation.
const TokenTypeGitHubApp = "GITHUB_APP"

// ErrInstallationBoundToOtherOrg is returned when an installation already belongs to a different org.
var ErrInstallationBoundToOtherOrg = errors.New("github app installation is connected to another LiveReview organization")

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
}

// UpsertAppInstallationConnector inserts or refreshes the GITHUB_APP row for (BaseURL, InstallationID).
// An installation belongs to exactly one org: if the row exists for a different org it returns
// ErrInstallationBoundToOtherOrg and changes nothing.
func (s *TokenStore) UpsertAppInstallationConnector(in AppConnectorUpsert) (connectorID int64, created bool, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()

	appID := fmt.Sprintf("%d", in.InstallationID)
	meta, err := json.Marshal(map[string]interface{}{
		"github_app":           true,
		"installation_id":      in.InstallationID,
		"account_login":        in.AccountLogin,
		"account_type":         in.AccountType,
		"repository_selection": in.RepositorySelection,
		"app_slug":             in.AppSlug,
		"removed":              false,
		"suspended":            false,
	})
	if err != nil {
		return 0, false, err
	}

	var existingOrg int64
	err = tx.QueryRow(`
		SELECT id, org_id FROM integration_tokens
		WHERE token_type = $1 AND provider_url = $2 AND provider_app_id = $3
		FOR UPDATE
	`, TokenTypeGitHubApp, in.BaseURL, appID).Scan(&connectorID, &existingOrg)
	switch {
	case err == nil:
		if existingOrg != in.OrgID {
			return 0, false, ErrInstallationBoundToOtherOrg
		}
		_, err = tx.Exec(`
			UPDATE integration_tokens
			SET pat_token = $1, expires_at = $2, connection_name = $3,
			    metadata = COALESCE(metadata, '{}'::jsonb) || $4::jsonb, updated_at = NOW()
			WHERE id = $5
		`, in.Token, in.ExpiresAt, "GitHub App · "+in.AccountLogin, string(meta), connectorID)
		if err != nil {
			return 0, false, err
		}
	case errors.Is(err, sql.ErrNoRows):
		// The unique index uq_integration_tokens_github_app_installation turns a racing insert into an error.
		err = tx.QueryRow(`
			INSERT INTO integration_tokens
				(provider, provider_app_id, access_token, token_type, pat_token, expires_at, connection_name,
				 provider_url, org_id, metadata)
			VALUES ('github', $1, '', $2, $3, $4, $5, $6, $7, $8::jsonb)
			RETURNING id
		`, appID, TokenTypeGitHubApp, in.Token, in.ExpiresAt, "GitHub App · "+in.AccountLogin, in.BaseURL, in.OrgID, string(meta)).Scan(&connectorID)
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
	return connectorID, created, nil
}

// GetAppConnectorByInstallation finds the connector for an installation id on a GitHub host.
func (s *TokenStore) GetAppConnectorByInstallation(baseURL string, installationID int64) (*IntegrationTokenRecord, error) {
	return scanRecord(s.db.QueryRow(`
		SELECT `+appConnectorColumns+` FROM integration_tokens
		WHERE token_type = $1 AND provider_url = $2 AND provider_app_id = $3
	`, TokenTypeGitHubApp, baseURL, fmt.Sprintf("%d", installationID)))
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
// cached token so nothing can keep using it.
func (s *TokenStore) SetAppConnectorFlags(connectorID int64, removed, suspended *bool) error {
	flags := map[string]bool{}
	if removed != nil {
		flags["removed"] = *removed
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

// HasAppConnectorForOwnerAllRepos reports whether the org has a live GitHub App connector installed on the account
// with access to ALL of its repositories. The repositories table is filled by a background sync after install, so
// this answers right away, before the sync lists a repository.
func (s *TokenStore) HasAppConnectorForOwnerAllRepos(orgID int64, owner string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM integration_tokens it
			WHERE it.org_id = $1 AND it.token_type = $2
			  AND LOWER(it.metadata->>'account_login') = LOWER($3)
			  AND it.metadata->>'repository_selection' = 'all'
			  AND COALESCE(it.metadata->>'removed', 'false') <> 'true'
			  AND COALESCE(it.metadata->>'suspended', 'false') <> 'true'
		)
	`, orgID, TokenTypeGitHubApp, owner).Scan(&exists)
	return exists, err
}
