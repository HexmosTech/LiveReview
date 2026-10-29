// Package scim implements the subset of SCIM 2.0 that enterprise IdPs
// (Okta, Azure AD, Google Workspace) actually use for user lifecycle sync:
// create, read, list-by-filter, and PATCH active:false/true. See
// docs/sso.md for the user-facing setup flow and internal/api/scim_handlers.go
// for the HTTP layer that wires this package into internal/api/users.
package scim

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// tokenPrefix mirrors api_keys.go's "lr_" scheme (see
// internal/api/api_keys.go's GenerateAPIKey) so SCIM tokens are visually
// distinct from regular API keys and session tokens.
const tokenPrefix = "scim_"

var (
	ErrTokenNotFound = errors.New("scim: token not found")
	ErrTokenRevoked  = errors.New("scim: token revoked")
	ErrTokenExpired  = errors.New("scim: token expired")
)

// GenerateToken creates a new SCIM bearer token in the form "scim_<base32>".
func GenerateToken() (string, error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(randomBytes)
	return tokenPrefix + strings.ToLower(encoded), nil
}

// HashToken returns the SHA-256 hash of a SCIM token, which is what's
// actually stored - the plaintext token is only ever shown once, at
// creation time, same as an API key.
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// TokenPrefix extracts a short, non-secret prefix for display in the
// settings UI (e.g. "scim_abcd1234..."), same convention as
// api_keys.go's GetKeyPrefix.
func TokenPrefix(token string) string {
	stripped := strings.TrimPrefix(token, tokenPrefix)
	if len(stripped) > 8 {
		stripped = stripped[:8]
	}
	return tokenPrefix + stripped
}

// Token is a SCIM bearer token record for one org.
type Token struct {
	ID              int64
	OrgID           int64
	TokenPrefix     string
	CreatedAt       time.Time
	ExpiresAt       *time.Time
	RevokedAt       *time.Time
	CreatedByUserID *int64
}

// TokenStore manages the scim_tokens table.
type TokenStore struct {
	db *sql.DB
}

func NewTokenStore(db *sql.DB) *TokenStore {
	return &TokenStore{db: db}
}

// Create generates a new token for orgID, replacing any existing one (an
// org has at most one live SCIM token at a time - rotating means the old
// one stops working, same mental model as rotating an API key). Returns the
// plaintext token, which the caller must show to the user exactly once.
func (s *TokenStore) Create(orgID int64, createdByUserID int64) (plaintext string, tok *Token, err error) {
	plaintext, err = GenerateToken()
	if err != nil {
		return "", nil, err
	}
	hash := HashToken(plaintext)
	prefix := TokenPrefix(plaintext)

	tok = &Token{}
	err = s.db.QueryRow(`
		INSERT INTO scim_tokens (org_id, token_hash, token_prefix, created_by_user_id, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		RETURNING id, org_id, token_prefix, created_at, expires_at, revoked_at, created_by_user_id
	`, orgID, hash, prefix, createdByUserID).Scan(
		&tok.ID, &tok.OrgID, &tok.TokenPrefix, &tok.CreatedAt, &tok.ExpiresAt, &tok.RevokedAt, &tok.CreatedByUserID,
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create scim token: %w", err)
	}
	return plaintext, tok, nil
}

// Authenticate resolves a plaintext bearer token to the org it belongs to.
// Returns ErrTokenNotFound/ErrTokenRevoked/ErrTokenExpired so the HTTP layer
// can give a precise SCIM error response.
func (s *TokenStore) Authenticate(plaintext string) (orgID int64, err error) {
	hash := HashToken(plaintext)
	var revokedAt *time.Time
	var expiresAt *time.Time
	row := s.db.QueryRow(`SELECT org_id, revoked_at, expires_at FROM scim_tokens WHERE token_hash = $1`, hash)
	if scanErr := row.Scan(&orgID, &revokedAt, &expiresAt); scanErr != nil {
		if scanErr == sql.ErrNoRows {
			return 0, ErrTokenNotFound
		}
		return 0, fmt.Errorf("failed to look up scim token: %w", scanErr)
	}
	if revokedAt != nil {
		return 0, ErrTokenRevoked
	}
	if expiresAt != nil && expiresAt.Before(time.Now()) {
		return 0, ErrTokenExpired
	}
	return orgID, nil
}

// Get returns orgID's current live token, if any (nil, nil if none exists).
func (s *TokenStore) Get(orgID int64) (*Token, error) {
	tok := &Token{}
	err := s.db.QueryRow(`
		SELECT id, org_id, token_prefix, created_at, expires_at, revoked_at, created_by_user_id
		FROM scim_tokens WHERE org_id = $1 AND revoked_at IS NULL
		ORDER BY created_at DESC LIMIT 1
	`, orgID).Scan(&tok.ID, &tok.OrgID, &tok.TokenPrefix, &tok.CreatedAt, &tok.ExpiresAt, &tok.RevokedAt, &tok.CreatedByUserID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load scim token: %w", err)
	}
	return tok, nil
}

// Revoke invalidates orgID's current token immediately.
func (s *TokenStore) Revoke(orgID int64) error {
	_, err := s.db.Exec(`UPDATE scim_tokens SET revoked_at = NOW() WHERE org_id = $1 AND revoked_at IS NULL`, orgID)
	if err != nil {
		return fmt.Errorf("failed to revoke scim token: %w", err)
	}
	return nil
}
