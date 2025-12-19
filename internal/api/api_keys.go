package api

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// APIKey represents an API key record
type APIKey struct {
	ID         int64           `json:"id"`
	UserID     int64           `json:"user_id"`
	OrgID      int64           `json:"org_id"`
	KeyHash    string          `json:"-"`
	KeyPrefix  string          `json:"key_prefix"`
	Label      string          `json:"label"`
	Scopes     json.RawMessage `json:"scopes"`
	LastUsedAt *time.Time      `json:"last_used_at"`
	CreatedAt  time.Time       `json:"created_at"`
	ExpiresAt  *time.Time      `json:"expires_at"`
	RevokedAt  *time.Time      `json:"revoked_at"`
}

// APIKeyManager handles API key operations
type APIKeyManager struct {
	db *sql.DB
}

// NewAPIKeyManager creates a new API key manager
func NewAPIKeyManager(db *sql.DB) *APIKeyManager {
	return &APIKeyManager{db: db}
}

// GenerateAPIKey creates a new API key with the format: lr_<base32_random>
func (m *APIKeyManager) GenerateAPIKey() (string, error) {
	// Generate 32 random bytes (256 bits)
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode to base32 for better readability (no ambiguous characters)
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(randomBytes)
	key := "lr_" + strings.ToLower(encoded)

	return key, nil
}

// HashAPIKey creates a SHA-256 hash of the API key
func (m *APIKeyManager) HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// GetKeyPrefix extracts the first 8 characters after "lr_" for display
func (m *APIKeyManager) GetKeyPrefix(key string) string {
	if !strings.HasPrefix(key, "lr_") {
		return ""
	}
	stripped := strings.TrimPrefix(key, "lr_")
	if len(stripped) > 8 {
		return "lr_" + stripped[:8]
	}
	return "lr_" + stripped
}

// CreateAPIKey generates and stores a new API key
func (m *APIKeyManager) CreateAPIKey(userID, orgID int64, label string, scopes []string, expiresAt *time.Time) (*APIKey, string, error) {
	// Generate the key
	key, err := m.GenerateAPIKey()
	if err != nil {
		return nil, "", err
	}

	keyHash := m.HashAPIKey(key)
	keyPrefix := m.GetKeyPrefix(key)

	scopesJSON, _ := json.Marshal(scopes)

	query := `
		INSERT INTO api_keys (user_id, org_id, key_hash, key_prefix, label, scopes, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, org_id, key_hash, key_prefix, label, scopes, last_used_at, created_at, expires_at, revoked_at
	`

	var apiKey APIKey
	err = m.db.QueryRow(query, userID, orgID, keyHash, keyPrefix, label, scopesJSON, expiresAt).Scan(
		&apiKey.ID,
		&apiKey.UserID,
		&apiKey.OrgID,
		&apiKey.KeyHash,
		&apiKey.KeyPrefix,
		&apiKey.Label,
		&apiKey.Scopes,
		&apiKey.LastUsedAt,
		&apiKey.CreatedAt,
		&apiKey.ExpiresAt,
		&apiKey.RevokedAt,
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create API key: %w", err)
	}

	return &apiKey, key, nil
}

// ValidateAPIKey checks if a key is valid and returns the associated key record
func (m *APIKeyManager) ValidateAPIKey(key string) (*APIKey, error) {
	keyHash := m.HashAPIKey(key)

	query := `
		SELECT id, user_id, org_id, key_hash, key_prefix, label, scopes, last_used_at, created_at, expires_at, revoked_at
		FROM api_keys
		WHERE key_hash = $1 AND revoked_at IS NULL
	`

	var apiKey APIKey
	err := m.db.QueryRow(query, keyHash).Scan(
		&apiKey.ID,
		&apiKey.UserID,
		&apiKey.OrgID,
		&apiKey.KeyHash,
		&apiKey.KeyPrefix,
		&apiKey.Label,
		&apiKey.Scopes,
		&apiKey.LastUsedAt,
		&apiKey.CreatedAt,
		&apiKey.ExpiresAt,
		&apiKey.RevokedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid API key")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to validate API key: %w", err)
	}

	// Check if expired
	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("API key expired")
	}

	return &apiKey, nil
}

// UpdateLastUsed updates the last_used_at timestamp for a key
func (m *APIKeyManager) UpdateLastUsed(keyID int64) error {
	query := `UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`
	_, err := m.db.Exec(query, keyID)
	return err
}

// ListAPIKeys returns all API keys for a user in an organization
func (m *APIKeyManager) ListAPIKeys(userID, orgID int64) ([]APIKey, error) {
	query := `
		SELECT id, user_id, org_id, key_hash, key_prefix, label, scopes, last_used_at, created_at, expires_at, revoked_at
		FROM api_keys
		WHERE user_id = $1 AND org_id = $2 AND revoked_at IS NULL
		ORDER BY created_at DESC
	`

	rows, err := m.db.Query(query, userID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var key APIKey
		err := rows.Scan(
			&key.ID,
			&key.UserID,
			&key.OrgID,
			&key.KeyHash,
			&key.KeyPrefix,
			&key.Label,
			&key.Scopes,
			&key.LastUsedAt,
			&key.CreatedAt,
			&key.ExpiresAt,
			&key.RevokedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan API key: %w", err)
		}
		keys = append(keys, key)
	}

	return keys, nil
}

// RevokeAPIKey marks a key as revoked
func (m *APIKeyManager) RevokeAPIKey(keyID, userID, orgID int64) error {
	query := `
		UPDATE api_keys
		SET revoked_at = NOW()
		WHERE id = $1 AND user_id = $2 AND org_id = $3 AND revoked_at IS NULL
	`
	result, err := m.db.Exec(query, keyID, userID, orgID)
	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("API key not found or already revoked")
	}

	return nil
}

// DeleteAPIKey permanently deletes a key
func (m *APIKeyManager) DeleteAPIKey(keyID, userID, orgID int64) error {
	query := `DELETE FROM api_keys WHERE id = $1 AND user_id = $2 AND org_id = $3`
	result, err := m.db.Exec(query, keyID, userID, orgID)
	if err != nil {
		return fmt.Errorf("failed to delete API key: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("API key not found")
	}

	return nil
}
