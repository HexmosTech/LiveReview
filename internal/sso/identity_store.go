package sso

import (
	"database/sql"
	"fmt"
)

// IdentityStore persists sso_identities: the mapping from an IdP's own
// subject identifier (OIDC `sub` / SAML NameID) to a LiveReview user, per
// org and provider. Two things need this mapping, neither of which a
// LiveReview user id alone can satisfy:
//   - An incoming OIDC back-channel logout token or SAML LogoutRequest only
//     ever carries the IdP's subject, never a LiveReview user id.
//   - The reconciliation poll (Phase 5) needs an encrypted refresh token per
//     identity to silently re-check "is this account still valid at the IdP".
type IdentityStore struct {
	db  *sql.DB
	box *SecretBox
}

func NewIdentityStore(db *sql.DB, box *SecretBox) *IdentityStore {
	return &IdentityStore{db: db, box: box}
}

// Upsert records (or refreshes) the subject->user mapping for a successful
// login, and the refresh token if one was issued (empty string clears it -
// callers pass "" when the IdP didn't grant one, e.g. reconciliation isn't
// enabled for this config).
func (s *IdentityStore) Upsert(orgID, userID int64, provider ProviderType, subject, refreshToken string) error {
	encRefresh, err := s.box.Encrypt(refreshToken)
	if err != nil {
		return fmt.Errorf("sso: encrypting refresh token: %w", err)
	}
	_, err = s.db.Exec(`
		INSERT INTO sso_identities (org_id, user_id, provider_type, subject, oidc_refresh_token_enc, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (org_id, provider_type, subject) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			oidc_refresh_token_enc = CASE WHEN EXCLUDED.oidc_refresh_token_enc IS NOT NULL THEN EXCLUDED.oidc_refresh_token_enc ELSE sso_identities.oidc_refresh_token_enc END,
			updated_at = NOW()
	`, orgID, userID, string(provider), subject, nullable(encRefresh))
	if err != nil {
		return fmt.Errorf("sso: upserting identity: %w", err)
	}
	return nil
}

// LookupUserBySubject resolves an incoming logout event's subject to a
// LiveReview user id, for back-channel logout / SAML SLO (Phase 4).
func (s *IdentityStore) LookupUserBySubject(orgID int64, provider ProviderType, subject string) (userID int64, err error) {
	err = s.db.QueryRow(`
		SELECT user_id FROM sso_identities WHERE org_id = $1 AND provider_type = $2 AND subject = $3
	`, orgID, string(provider), subject).Scan(&userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("sso: no identity found for subject")
		}
		return 0, fmt.Errorf("sso: looking up identity: %w", err)
	}
	return userID, nil
}

// IdentityWithRefreshToken is one row the reconciliation worker checks.
type IdentityWithRefreshToken struct {
	ID           int64
	UserID       int64
	Subject      string
	RefreshToken string
}

// ListWithRefreshToken returns every stored identity for orgID that has a
// refresh token on file, for the reconciliation worker
// (internal/jobqueue/sso_reconciliation_worker.go) to silently re-check.
func (s *IdentityStore) ListWithRefreshToken(orgID int64, provider ProviderType) ([]IdentityWithRefreshToken, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, subject, oidc_refresh_token_enc
		FROM sso_identities
		WHERE org_id = $1 AND provider_type = $2 AND oidc_refresh_token_enc IS NOT NULL
	`, orgID, string(provider))
	if err != nil {
		return nil, fmt.Errorf("sso: listing identities with refresh tokens: %w", err)
	}
	defer rows.Close()

	var out []IdentityWithRefreshToken
	for rows.Next() {
		var row IdentityWithRefreshToken
		var encRefresh string
		if err := rows.Scan(&row.ID, &row.UserID, &row.Subject, &encRefresh); err != nil {
			return nil, fmt.Errorf("sso: scanning identity: %w", err)
		}
		refresh, err := s.box.Decrypt(encRefresh)
		if err != nil {
			return nil, fmt.Errorf("sso: decrypting refresh token: %w", err)
		}
		row.RefreshToken = refresh
		out = append(out, row)
	}
	return out, rows.Err()
}

// MarkChecked stamps last_checked_at = NOW() for one identity after a
// successful reconciliation refresh.
func (s *IdentityStore) MarkChecked(id int64) error {
	_, err := s.db.Exec(`UPDATE sso_identities SET last_checked_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("sso: marking identity checked: %w", err)
	}
	return nil
}
