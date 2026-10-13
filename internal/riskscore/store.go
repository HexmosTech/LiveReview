package riskscore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// Store bundles the DB access for the trained risk model: seed/version
// management, feature snapshots, and category overrides. Mutating functions
// invalidate the scorer's in-memory cache for the affected org.
type Store struct {
	db *sql.DB
}

// NewStore returns a Store backed by db.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// EnsureSeedModel inserts the single shared seed model (org_id NULL,
// update_kind 'seed') if it doesn't already exist. Idempotent.
func (s *Store) EnsureSeedModel(ctx context.Context) error {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM risk_model_versions WHERE org_id IS NULL AND update_kind = 'seed')`).Scan(&exists)
	if err != nil {
		return fmt.Errorf("checking seed model: %w", err)
	}
	if exists {
		return nil
	}
	weights, err := MarshalWeights(Prior())
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO risk_model_versions (org_id, version, weights_json, training_sample_count, update_kind, is_active)
		VALUES (NULL, 1, $1, 0, 'seed', false)`, weights)
	if err != nil {
		return fmt.Errorf("inserting seed model: %w", err)
	}
	return nil
}

// ActiveWeights returns the org's active weights, falling back to the shared
// seed when the org has no active version yet. Lazily creates the seed model
// if it doesn't exist yet (e.g. no review has ever been scored in this
// deployment), so callers never see a bare "no rows" error.
func (s *Store) ActiveWeights(ctx context.Context, orgID int64) (version int, weights Weights, sampleCount int, err error) {
	var raw []byte
	err = s.db.QueryRowContext(ctx, `
		SELECT version, weights_json, training_sample_count
		FROM risk_model_versions
		WHERE org_id = $1 AND is_active
		ORDER BY id DESC LIMIT 1`, orgID).Scan(&version, &raw, &sampleCount)
	if err == nil {
		weights, err = UnmarshalWeights(raw)
		return version, weights, sampleCount, err
	}
	if err != sql.ErrNoRows {
		return 0, nil, 0, err
	}

	if err := s.EnsureSeedModel(ctx); err != nil {
		return 0, nil, 0, err
	}

	// Fall back to the shared seed.
	err = s.db.QueryRowContext(ctx, `
		SELECT version, weights_json, training_sample_count
		FROM risk_model_versions
		WHERE org_id IS NULL AND update_kind = 'seed'
		ORDER BY id DESC LIMIT 1`).Scan(&version, &raw, &sampleCount)
	if err != nil {
		return 0, nil, 0, err
	}
	weights, err = UnmarshalWeights(raw)
	return version, weights, sampleCount, err
}

// NextVersion returns the next per-org version number (1 if none yet).
func (s *Store) NextVersion(ctx context.Context, orgID int64) (int, error) {
	var v int
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) + 1 FROM risk_model_versions WHERE org_id = $1`, orgID).Scan(&v)
	return v, err
}

// SaveVersion appends a new active version (deactivating the previous one) and
// returns the new row id. Append-only: any version can be reactivated later.
func (s *Store) SaveVersion(ctx context.Context, orgID int64, weights Weights, sampleCount int, auc *float64, kind string) (int64, error) {
	raw, err := MarshalWeights(weights)
	if err != nil {
		return 0, err
	}
	version, err := s.NextVersion(ctx, orgID)
	if err != nil {
		return 0, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `UPDATE risk_model_versions SET is_active = false WHERE org_id = $1 AND is_active`, orgID); err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO risk_model_versions (org_id, version, weights_json, training_sample_count, holdout_auc, update_kind, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, true) RETURNING id`,
		orgID, version, raw, sampleCount, auc, kind).Scan(&id)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}

	Invalidate(orgID)
	return id, nil
}

// SaveFeatureSnapshot records the feature vector and score at evaluation time
// (the training set once review_outcomes attach to the same review_id). One
// snapshot per review: a re-evaluation replaces the prior row.
func (s *Store) SaveFeatureSnapshot(ctx context.Context, reviewID, orgID int64, modelVersion int, features []float64, score float64) error {
	raw, err := json.Marshal(features)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM risk_feature_snapshots WHERE review_id = $1 AND org_id = $2`, reviewID, orgID); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO risk_feature_snapshots (review_id, org_id, model_version, features, score)
		VALUES ($1, $2, $3, $4, $5)`, reviewID, orgID, modelVersion, raw, score)
	return err
}

// LoadOverrides returns the org's category multipliers, defaulting missing
// categories to 1.0.
func (s *Store) LoadOverrides(ctx context.Context, orgID int64) (map[string]float64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT feature_category, multiplier FROM risk_factor_overrides WHERE org_id = $1`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]float64{}
	for rows.Next() {
		var cat string
		var m float64
		if err := rows.Scan(&cat, &m); err != nil {
			return nil, err
		}
		out[cat] = m
	}
	return out, rows.Err()
}

// UpsertOverride sets a category multiplier. Takes effect immediately (the
// scorer cache is invalidated); the next batch refit treats it as an anchor.
func (s *Store) UpsertOverride(ctx context.Context, orgID int64, category string, multiplier float64) error {
	if multiplier <= 0 {
		return fmt.Errorf("multiplier must be > 0")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO risk_factor_overrides (org_id, feature_category, multiplier)
		VALUES ($1, $2, $3)
		ON CONFLICT (org_id, feature_category) DO UPDATE SET multiplier = EXCLUDED.multiplier, updated_at = now()`,
		orgID, category, multiplier)
	if err == nil {
		Invalidate(orgID)
	}
	return err
}
