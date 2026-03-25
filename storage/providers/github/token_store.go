package github

import "database/sql"

type IntegrationTokenRecord struct {
	ID          int64
	Provider    string
	ProviderURL string
	PatToken    string
	Metadata    []byte
}

type TokenStore struct {
	db *sql.DB
}

func NewTokenStore(db *sql.DB) *TokenStore {
	return &TokenStore{db: db}
}

func (s *TokenStore) GetLatestGitHubToken() (*IntegrationTokenRecord, error) {
	query := `
		SELECT id, provider, provider_url, pat_token, COALESCE(metadata, '{}')
		FROM integration_tokens
		WHERE provider = 'github'
		  AND (provider_url = 'https://github.com' OR provider_url = 'https://api.github.com')
		ORDER BY created_at DESC
		LIMIT 1
	`

	var rec IntegrationTokenRecord
	err := s.db.QueryRow(query).Scan(&rec.ID, &rec.Provider, &rec.ProviderURL, &rec.PatToken, &rec.Metadata)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}
