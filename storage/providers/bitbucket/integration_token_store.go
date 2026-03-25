package bitbucket

import "database/sql"

type IntegrationTokenRecord struct {
	ID          int64
	Provider    string
	ProviderURL string
	PatToken    string
	OrgID       int64
	Metadata    []byte
}

type IntegrationTokenStore struct {
	db *sql.DB
}

func NewIntegrationTokenStore(db *sql.DB) *IntegrationTokenStore {
	return &IntegrationTokenStore{db: db}
}

func (s *IntegrationTokenStore) GetTokenByRepoFullName(repoFullName string) (*IntegrationTokenRecord, error) {
	query := `
		SELECT it.id, it.provider, it.provider_url, it.pat_token, it.org_id, COALESCE(it.metadata, '{}')
		FROM integration_tokens it
		JOIN webhook_registry wr ON wr.integration_token_id = it.id
		WHERE wr.project_full_name = $1
		LIMIT 1
	`

	var rec IntegrationTokenRecord
	err := s.db.QueryRow(query, repoFullName).Scan(
		&rec.ID, &rec.Provider, &rec.ProviderURL, &rec.PatToken, &rec.OrgID, &rec.Metadata,
	)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (s *IntegrationTokenStore) GetLatestBitbucketToken() (*IntegrationTokenRecord, error) {
	fallbackQuery := `
		SELECT id, provider, provider_url, pat_token, org_id, COALESCE(metadata, '{}')
		FROM integration_tokens
		WHERE provider LIKE 'bitbucket%'
		ORDER BY created_at DESC
		LIMIT 1
	`

	var rec IntegrationTokenRecord
	err := s.db.QueryRow(fallbackQuery).Scan(
		&rec.ID, &rec.Provider, &rec.ProviderURL, &rec.PatToken, &rec.OrgID, &rec.Metadata,
	)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}
