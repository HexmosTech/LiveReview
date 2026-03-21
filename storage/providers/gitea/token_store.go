package gitea

import "database/sql"

type IntegrationTokenRecord struct {
	ID           int64
	Provider     string
	ProviderURL  string
	PatToken     string
	OrgID        int64
	MetadataJSON string
}

type TokenStore struct {
	db *sql.DB
}

func NewTokenStore(db *sql.DB) *TokenStore {
	return &TokenStore{db: db}
}

func (s *TokenStore) ListRecentGiteaIntegrationTokens(limit int) ([]IntegrationTokenRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, provider, provider_url, pat_token, org_id, COALESCE(metadata, '{}') as metadata
		FROM integration_tokens
		WHERE provider = 'gitea'
		AND org_id IS NOT NULL
		ORDER BY updated_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]IntegrationTokenRecord, 0)
	for rows.Next() {
		var rec IntegrationTokenRecord
		if err := rows.Scan(&rec.ID, &rec.Provider, &rec.ProviderURL, &rec.PatToken, &rec.OrgID, &rec.MetadataJSON); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *TokenStore) GetGiteaIntegrationTokenByID(connectorID int) (*IntegrationTokenRecord, error) {
	rec := &IntegrationTokenRecord{}
	err := s.db.QueryRow(`
		SELECT id, provider, provider_url, pat_token, org_id, COALESCE(metadata, '{}') as metadata
		FROM integration_tokens
		WHERE id = $1 AND provider = 'gitea'
	`, connectorID).Scan(&rec.ID, &rec.Provider, &rec.ProviderURL, &rec.PatToken, &rec.OrgID, &rec.MetadataJSON)
	if err != nil {
		return nil, err
	}
	return rec, nil
}

func (s *TokenStore) GetLatestWebhookSecret(connectorID int) (sql.NullString, error) {
	var secret sql.NullString
	err := s.db.QueryRow(`
		SELECT webhook_secret
		FROM webhook_registry
		WHERE integration_token_id = $1 AND provider = 'gitea'
		ORDER BY updated_at DESC
		LIMIT 1
	`, connectorID).Scan(&secret)
	if err != nil {
		return sql.NullString{}, err
	}
	return secret, nil
}
