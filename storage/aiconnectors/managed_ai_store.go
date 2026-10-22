package aiconnectors

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ManagedAIConfig is one configuration of the LiveReview-managed model (a row of system_default_ai_configs).
// The master key is never part of it; HasKey says whether one is stored.
type ManagedAIConfig struct {
	ID              int64     `json:"id"`
	TierName        string    `json:"tier_name"`
	ProviderName    string    `json:"provider_name"`
	ModelName       string    `json:"model_name"`
	GCPLocation     string    `json:"gcp_location"`
	Label           string    `json:"label"`
	IsActive        bool      `json:"is_active"`
	HasKey          bool      `json:"has_key"`
	UpdatedByUserID *int64    `json:"updated_by_user_id,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ManagedAIConfigInput is what the admin page creates or edits. An empty APIKey on update keeps the stored key.
type ManagedAIConfigInput struct {
	TierName     string
	ProviderName string
	ModelName    string
	APIKey       string
	GCPLocation  string
	Label        string
}

// ErrManagedAIConfigNotFound is returned when no configuration has the given id.
var ErrManagedAIConfigNotFound = errors.New("managed AI configuration not found")

type ManagedAIStore struct {
	db *sql.DB
}

func NewManagedAIStore(db *sql.DB) *ManagedAIStore {
	return &ManagedAIStore{db: db}
}

// ListManagedAIConfigs returns every configuration of every tier, active first.
func (s *ManagedAIStore) ListManagedAIConfigs(ctx context.Context) ([]ManagedAIConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tier_name, provider_name, model_name, COALESCE(gcp_location, ''), COALESCE(label, ''),
		       COALESCE(is_active, false), master_api_key <> '', updated_by_user_id, COALESCE(updated_at, created_at, NOW())
		FROM system_default_ai_configs
		ORDER BY tier_name, is_active DESC, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ManagedAIConfig{}
	for rows.Next() {
		var c ManagedAIConfig
		var updatedBy sql.NullInt64
		if err := rows.Scan(&c.ID, &c.TierName, &c.ProviderName, &c.ModelName, &c.GCPLocation, &c.Label,
			&c.IsActive, &c.HasKey, &updatedBy, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if updatedBy.Valid {
			c.UpdatedByUserID = &updatedBy.Int64
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetManagedAIConfig returns one configuration and its stored key (for testing it before activation).
func (s *ManagedAIStore) GetManagedAIConfig(ctx context.Context, id int64) (ManagedAIConfig, string, error) {
	var c ManagedAIConfig
	var key string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, tier_name, provider_name, model_name, COALESCE(gcp_location, ''), COALESCE(label, ''),
		       COALESCE(is_active, false), master_api_key
		FROM system_default_ai_configs WHERE id = $1
	`, id).Scan(&c.ID, &c.TierName, &c.ProviderName, &c.ModelName, &c.GCPLocation, &c.Label, &c.IsActive, &key)
	if errors.Is(err, sql.ErrNoRows) {
		return c, "", ErrManagedAIConfigNotFound
	}
	c.HasKey = key != ""
	return c, key, err
}

// CreateManagedAIConfig adds an inactive configuration to a tier.
func (s *ManagedAIStore) CreateManagedAIConfig(ctx context.Context, in ManagedAIConfigInput, actorUserID int64) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO system_default_ai_configs
			(tier_name, provider_name, model_name, master_api_key, gcp_location, label, is_active, updated_by_user_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), false, $7, NOW(), NOW())
		RETURNING id
	`, in.TierName, in.ProviderName, in.ModelName, in.APIKey, in.GCPLocation, in.Label, actorUserID).Scan(&id)
	return id, err
}

// UpdateManagedAIConfig edits a configuration in place; its tier and active state don't change. An empty
// in.APIKey keeps the stored key.
func (s *ManagedAIStore) UpdateManagedAIConfig(ctx context.Context, id int64, in ManagedAIConfigInput, actorUserID int64) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE system_default_ai_configs
		SET provider_name = $2, model_name = $3,
		    master_api_key = CASE WHEN $4 = '' THEN master_api_key ELSE $4 END,
		    gcp_location = NULLIF($5, ''), label = NULLIF($6, ''),
		    updated_by_user_id = $7, updated_at = NOW()
		WHERE id = $1
	`, id, in.ProviderName, in.ModelName, in.APIKey, in.GCPLocation, in.Label, actorUserID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrManagedAIConfigNotFound
	}
	return nil
}

// ActivateManagedAIConfig makes id the only active configuration of its tier, in one transaction.
func (s *ManagedAIStore) ActivateManagedAIConfig(ctx context.Context, id int64, actorUserID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var tier string
	if err := tx.QueryRowContext(ctx, `SELECT tier_name FROM system_default_ai_configs WHERE id = $1 FOR UPDATE`, id).Scan(&tier); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrManagedAIConfigNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE system_default_ai_configs SET is_active = false, updated_by_user_id = $2, updated_at = NOW()
		WHERE tier_name = $1 AND is_active AND id <> $3
	`, tier, actorUserID, id); err != nil {
		return fmt.Errorf("deactivate current configuration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE system_default_ai_configs SET is_active = true, updated_by_user_id = $2, updated_at = NOW()
		WHERE id = $1
	`, id, actorUserID); err != nil {
		return fmt.Errorf("activate configuration: %w", err)
	}
	return tx.Commit()
}

// EnsureManagedConnector adds the org's "LiveReview AI Model" connector for role ("leader" or "helper") at the
// top of that role's list, unless it already has one. providerName is aidefault.ProviderName (passed in because
// that package depends on this one).
func EnsureManagedConnector(tx *sql.Tx, orgID int64, providerName, role string) error {
	name, tier := "LiveReview AI Model", "default"
	if role == "helper" {
		name, tier = "LiveReview AI Model (Helper)", "default_lite"
	}
	var exists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM ai_connectors WHERE org_id = $1 AND provider_name = $2 AND role = $3)`,
		orgID, providerName, role).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check managed %s AI connector existence: %w", role, err)
	}
	if exists {
		return nil
	}
	// Shift the role's existing connectors down to make room at display_order = 1.
	if _, err := tx.Exec(`UPDATE ai_connectors SET display_order = display_order + 1 WHERE org_id = $1 AND role = $2`, orgID, role); err != nil {
		return fmt.Errorf("failed to shift existing %s AI connectors: %w", role, err)
	}
	if _, err := tx.Exec(`
		INSERT INTO ai_connectors (
			provider_name, api_key, connector_name, selected_model, display_order, role, org_id,
			created_at, updated_at
		) VALUES ($1, 'system_managed', $2, $3, 1, $4, $5, NOW(), NOW())
	`, providerName, name, tier, role, orgID); err != nil {
		return fmt.Errorf("failed to provision managed %s AI connector: %w", role, err)
	}
	return nil
}

// GrantFreeManagedAI lets a Free org run reviews on the LiveReview model (orgs created through
// /gh-login?src=launch-trial) and adds its managed leader connector, in one transaction.
func (s *ManagedAIStore) GrantFreeManagedAI(ctx context.Context, orgID int64, providerName string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		UPDATE org_billing_state SET free_managed_ai_granted_at = COALESCE(free_managed_ai_granted_at, NOW()), updated_at = NOW()
		WHERE org_id = $1
	`, orgID)
	if err != nil {
		return fmt.Errorf("record free managed AI grant: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("org %d has no billing state", orgID)
	}
	if err := EnsureManagedConnector(tx, orgID, providerName, "leader"); err != nil {
		return err
	}
	return tx.Commit()
}

// HasFreeManagedAI reports whether the org was granted the LiveReview model on the Free plan.
func (s *ManagedAIStore) HasFreeManagedAI(ctx context.Context, orgID int64) (bool, error) {
	var granted bool
	err := s.db.QueryRowContext(ctx, `
		SELECT free_managed_ai_granted_at IS NOT NULL FROM org_billing_state WHERE org_id = $1
	`, orgID).Scan(&granted)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return granted, err
}
