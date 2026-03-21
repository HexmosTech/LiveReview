package gitlab

import (
	"database/sql"
	"encoding/json"
	"time"
)

type TokenRecord struct {
	RefreshToken       string
	ProviderURL        string
	StoredClientSecret string
}

type TokenUpsertInput struct {
	ProviderAppID  string
	AccessToken    string
	RefreshToken   string
	TokenType      string
	Scope          string
	ExpiresAt      *time.Time
	MetadataJSON   []byte
	Code           string
	ConnectionName string
	ProviderURL    string
	ClientSecret   string
}

type TokenRefreshUpdateInput struct {
	IntegrationID int64
	AccessToken   string
	RefreshToken  string
	TokenType     string
	Scope         string
	ExpiresAt     *time.Time
}

type AuthTokenStore struct {
	db *sql.DB
}

func NewAuthTokenStore(db *sql.DB) *AuthTokenStore {
	return &AuthTokenStore{db: db}
}

func (s *AuthTokenStore) UpsertGitLabIntegrationToken(input TokenUpsertInput) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}

	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()

	var existingID int64
	err = tx.QueryRow(`
		SELECT id FROM integration_tokens
		WHERE provider = 'gitlab' AND provider_app_id = $1 AND connection_name = $2
	`, input.ProviderAppID, input.ConnectionName).Scan(&existingID)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}

	var integrationTokenID int64
	if err == sql.ErrNoRows {
		err = tx.QueryRow(`
			INSERT INTO integration_tokens
			(provider, provider_app_id, access_token, refresh_token, token_type, scope,
			 expires_at, metadata, code, connection_name, provider_url, client_secret)
			VALUES ('gitlab', $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id
		`, input.ProviderAppID, input.AccessToken, input.RefreshToken,
			input.TokenType, input.Scope, input.ExpiresAt, input.MetadataJSON,
			input.Code, input.ConnectionName, input.ProviderURL, input.ClientSecret).Scan(&integrationTokenID)
	} else {
		err = tx.QueryRow(`
			UPDATE integration_tokens
			SET access_token = $1, refresh_token = $2, token_type = $3, scope = $4,
			    expires_at = $5, metadata = $6, code = $7, updated_at = CURRENT_TIMESTAMP,
				provider_url = $8, client_secret = $9
			WHERE id = $10
			RETURNING id
		`, input.AccessToken, input.RefreshToken, input.TokenType,
			input.Scope, input.ExpiresAt, input.MetadataJSON, input.Code, input.ProviderURL,
			input.ClientSecret, existingID).Scan(&integrationTokenID)
		if err == nil {
			integrationTokenID = existingID
		}
	}

	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	rollback = false

	return integrationTokenID, nil
}

func (s *AuthTokenStore) GetGitLabTokenRecord(integrationID int64) (*TokenRecord, error) {
	var rec TokenRecord
	err := s.db.QueryRow(`
		SELECT refresh_token, provider_url, client_secret
		FROM integration_tokens
		WHERE id = $1 AND provider = 'gitlab'
	`, integrationID).Scan(&rec.RefreshToken, &rec.ProviderURL, &rec.StoredClientSecret)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (s *AuthTokenStore) GetProviderAppID(integrationID int64) (string, error) {
	var providerAppID string
	err := s.db.QueryRow(`
		SELECT provider_app_id FROM integration_tokens WHERE id = $1
	`, integrationID).Scan(&providerAppID)
	if err != nil {
		return "", err
	}
	return providerAppID, nil
}

func (s *AuthTokenStore) UpdateRefreshedToken(input TokenRefreshUpdateInput) error {
	_, err := s.db.Exec(`
		UPDATE integration_tokens
		SET access_token = $1, refresh_token = $2, token_type = $3, scope = $4,
		    expires_at = $5, updated_at = CURRENT_TIMESTAMP
		WHERE id = $6
	`, input.AccessToken, input.RefreshToken, input.TokenType,
		input.Scope, input.ExpiresAt, input.IntegrationID)
	return err
}

func BuildGitLabTokenMetadata(tokenType, scope string, createdAtUnixSeconds int, userID int, username, email, name, avatarURL string) ([]byte, error) {
	metadata := map[string]interface{}{
		"token_type":              tokenType,
		"scope":                   scope,
		"created_at":              createdAtUnixSeconds,
		"created_at_unix_seconds": createdAtUnixSeconds,
	}
	if userID > 0 {
		metadata["user_id"] = userID
		metadata["username"] = username
		metadata["email"] = email
		metadata["name"] = name
		metadata["avatar_url"] = avatarURL
	}
	return json.Marshal(metadata)
}
