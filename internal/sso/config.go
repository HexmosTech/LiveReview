package sso

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ProviderType is the SSO protocol a config row speaks.
type ProviderType string

const (
	ProviderOIDC ProviderType = "oidc"
	ProviderSAML ProviderType = "saml"
)

// Config is one org's SSO provider configuration. ClientSecret/metadata XML
// are plaintext here (decrypted on load) - never serialize this struct back
// to an HTTP response without stripping secrets first (see redactedConfig in
// handlers.go).
type Config struct {
	ID                 int64
	OrgID              int64
	ProviderType       ProviderType
	DisplayName        string
	Enabled            bool
	OIDCIssuer         string
	OIDCClientID       string
	OIDCClientSecret   string
	OIDCScopes         string
	SAMLIDPMetadataURL string
	SAMLIDPMetadataXML string
	SAMLIDPEntityID    string

	// Reconciliation fields only apply to provider_type=oidc - see Phase 5
	// of docs/sso.md's deprovisioning section. SAML has no standing
	// credential to silently re-check, so these are always false/zero for
	// SAML configs.
	ReconciliationEnabled  bool
	ReconciliationInterval string // "hourly" | "daily" | "weekly"
	LastReconciledAt       *time.Time
}

// Store persists org_sso_configs, transparently encrypting/decrypting the
// OIDC client secret with box.
type Store struct {
	db  *sql.DB
	box *SecretBox
}

func NewStore(db *sql.DB, box *SecretBox) *Store {
	return &Store{db: db, box: box}
}

// ErrNotFound is returned when no config row exists for the (org, provider).
var ErrNotFound = errors.New("sso: config not found")

func (s *Store) Get(orgID int64, provider ProviderType) (*Config, error) {
	row := s.db.QueryRow(`
		SELECT id, org_id, provider_type, display_name, enabled,
		       COALESCE(oidc_issuer, ''), COALESCE(oidc_client_id, ''), COALESCE(oidc_client_secret_enc, ''), COALESCE(oidc_scopes, ''),
		       COALESCE(saml_idp_metadata_url, ''), COALESCE(saml_idp_metadata_xml, ''), COALESCE(saml_idp_entity_id, ''),
		       reconciliation_enabled, reconciliation_interval, last_reconciled_at
		FROM org_sso_configs
		WHERE org_id = $1 AND provider_type = $2
	`, orgID, string(provider))

	cfg := &Config{}
	var encSecret string
	if err := row.Scan(&cfg.ID, &cfg.OrgID, &cfg.ProviderType, &cfg.DisplayName, &cfg.Enabled,
		&cfg.OIDCIssuer, &cfg.OIDCClientID, &encSecret, &cfg.OIDCScopes,
		&cfg.SAMLIDPMetadataURL, &cfg.SAMLIDPMetadataXML, &cfg.SAMLIDPEntityID,
		&cfg.ReconciliationEnabled, &cfg.ReconciliationInterval, &cfg.LastReconciledAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("sso: loading config: %w", err)
	}
	secret, err := s.box.Decrypt(encSecret)
	if err != nil {
		return nil, fmt.Errorf("sso: decrypting client secret: %w", err)
	}
	cfg.OIDCClientSecret = secret
	return cfg, nil
}

// ListEnabled returns every org's enabled configs, newest first - used by
// the public login-discovery endpoint.
func (s *Store) ListEnabled() ([]*Config, error) {
	rows, err := s.db.Query(`
		SELECT id, org_id, provider_type, display_name, enabled
		FROM org_sso_configs
		WHERE enabled = TRUE
		ORDER BY org_id, provider_type
	`)
	if err != nil {
		return nil, fmt.Errorf("sso: listing enabled configs: %w", err)
	}
	defer rows.Close()

	var out []*Config
	for rows.Next() {
		cfg := &Config{}
		if err := rows.Scan(&cfg.ID, &cfg.OrgID, &cfg.ProviderType, &cfg.DisplayName, &cfg.Enabled); err != nil {
			return nil, fmt.Errorf("sso: scanning config: %w", err)
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

// sqlExecer is satisfied by both *sql.DB and *sql.Tx, so Upsert/Delete can
// run either standalone or as part of a caller's transaction (the latter is
// required for the audited config-change handlers, which must write the
// audit event in the same transaction as the config change per CLAUDE.md's
// "Audit Logging Is Automatic" rule).
type sqlExecer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// Upsert creates or replaces the (org, provider) config. createdByUserID may
// be nil-equivalent (0) only for system-level callers; HTTP handlers must
// always pass the authenticated actor.
func (s *Store) Upsert(cfg *Config, createdByUserID int64) error {
	return s.UpsertTx(s.db, cfg, createdByUserID)
}

// UpsertTx is Upsert run against an explicit executor (typically a *sql.Tx
// an HTTP handler also uses to write its audit event).
func (s *Store) UpsertTx(exec sqlExecer, cfg *Config, createdByUserID int64) error {
	encSecret, err := s.box.Encrypt(cfg.OIDCClientSecret)
	if err != nil {
		return fmt.Errorf("sso: encrypting client secret: %w", err)
	}
	reconciliationInterval := cfg.ReconciliationInterval
	if reconciliationInterval == "" {
		reconciliationInterval = "daily"
	}
	_, err = exec.Exec(`
		INSERT INTO org_sso_configs (
			org_id, provider_type, display_name, enabled,
			oidc_issuer, oidc_client_id, oidc_client_secret_enc, oidc_scopes,
			saml_idp_metadata_url, saml_idp_metadata_xml, saml_idp_entity_id,
			reconciliation_enabled, reconciliation_interval,
			created_by_user_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (org_id, provider_type) DO UPDATE SET
			display_name = EXCLUDED.display_name,
			enabled = EXCLUDED.enabled,
			oidc_issuer = EXCLUDED.oidc_issuer,
			oidc_client_id = EXCLUDED.oidc_client_id,
			oidc_client_secret_enc = EXCLUDED.oidc_client_secret_enc,
			oidc_scopes = EXCLUDED.oidc_scopes,
			saml_idp_metadata_url = EXCLUDED.saml_idp_metadata_url,
			saml_idp_metadata_xml = EXCLUDED.saml_idp_metadata_xml,
			saml_idp_entity_id = EXCLUDED.saml_idp_entity_id,
			reconciliation_enabled = EXCLUDED.reconciliation_enabled,
			reconciliation_interval = EXCLUDED.reconciliation_interval,
			updated_at = NOW()
	`, cfg.OrgID, string(cfg.ProviderType), cfg.DisplayName, cfg.Enabled,
		nullable(cfg.OIDCIssuer), nullable(cfg.OIDCClientID), nullable(encSecret), nullable(cfg.OIDCScopes),
		nullable(cfg.SAMLIDPMetadataURL), nullable(cfg.SAMLIDPMetadataXML), nullable(cfg.SAMLIDPEntityID),
		cfg.ReconciliationEnabled, reconciliationInterval,
		nullableID(createdByUserID))
	if err != nil {
		return fmt.Errorf("sso: upserting config: %w", err)
	}
	return nil
}

func (s *Store) Delete(orgID int64, provider ProviderType) error {
	return s.DeleteTx(s.db, orgID, provider)
}

// DeleteTx is Delete run against an explicit executor; see UpsertTx.
func (s *Store) DeleteTx(exec sqlExecer, orgID int64, provider ProviderType) error {
	_, err := exec.Exec(`DELETE FROM org_sso_configs WHERE org_id = $1 AND provider_type = $2`, orgID, string(provider))
	if err != nil {
		return fmt.Errorf("sso: deleting config: %w", err)
	}
	return nil
}

// ListDueForReconciliation returns every OIDC config with the reconciliation
// poll enabled whose interval has elapsed since it last ran (or that has
// never run). Used by the coordinator job (internal/jobqueue/sso_reconciliation_worker.go)
// on a fixed short tick to decide which orgs to fan out one-shot work to -
// see Phase 5 of docs/sso.md's deprovisioning section for why this is a
// coordinator+fan-out job rather than one River PeriodicJob per org.
func (s *Store) ListDueForReconciliation() ([]*Config, error) {
	rows, err := s.db.Query(`
		SELECT id, org_id, provider_type, reconciliation_interval, last_reconciled_at
		FROM org_sso_configs
		WHERE provider_type = 'oidc' AND reconciliation_enabled = TRUE AND (
			last_reconciled_at IS NULL
			OR (reconciliation_interval = 'hourly' AND last_reconciled_at < NOW() - INTERVAL '1 hour')
			OR (reconciliation_interval = 'daily' AND last_reconciled_at < NOW() - INTERVAL '1 day')
			OR (reconciliation_interval = 'weekly' AND last_reconciled_at < NOW() - INTERVAL '7 days')
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("sso: listing configs due for reconciliation: %w", err)
	}
	defer rows.Close()

	var out []*Config
	for rows.Next() {
		cfg := &Config{}
		if err := rows.Scan(&cfg.ID, &cfg.OrgID, &cfg.ProviderType, &cfg.ReconciliationInterval, &cfg.LastReconciledAt); err != nil {
			return nil, fmt.Errorf("sso: scanning config due for reconciliation: %w", err)
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

// MarkReconciled stamps last_reconciled_at = NOW() once a reconciliation
// sweep finishes for (orgID, provider).
func (s *Store) MarkReconciled(orgID int64, provider ProviderType) error {
	_, err := s.db.Exec(`UPDATE org_sso_configs SET last_reconciled_at = NOW() WHERE org_id = $1 AND provider_type = $2`, orgID, string(provider))
	if err != nil {
		return fmt.Errorf("sso: marking config reconciled: %w", err)
	}
	return nil
}

func nullable(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// nullableID treats 0 as "no actor" (NULL) - callers like the SSO e2e
// harness that seed a config outside any real user session pass 0 rather
// than a fabricated user id.
func nullableID(id int64) interface{} {
	if id == 0 {
		return nil
	}
	return id
}
