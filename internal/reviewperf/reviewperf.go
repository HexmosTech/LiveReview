// Package reviewperf holds the instance-wide review performance settings:
// the safety ceiling on per-review batch concurrency and the floor/ceiling
// on the per-review timeout. These bound - but never replace - an org's own
// optional soft preference (see storage/aiconnectors.ReviewAISettings'
// PreferredMaxConcurrentBatches/PreferredReviewTimeoutSeconds): an org can
// turn its own concurrency/timeout down, never up past what the instance
// allows. Stored the same way as internal/api/storage_settings.go's
// blob_storage row - a system_settings JSONB row, read fresh on every
// review (no caching, no restart needed to pick up a change).
package reviewperf

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/livereview/internal/batch"
)

const settingsRowName = "ai_review_performance"

// Defaults match internal/batch.DefaultMaxConcurrentBatches and
// internal/review.DefaultReviewConfig's ReviewTimeout - the same conservative
// numbers used when no admin has ever touched this settings page.
const (
	DefaultMaxConcurrentBatchesCeiling = batch.DefaultMaxConcurrentBatches
	DefaultMinReviewTimeoutSeconds     = 60  // 1 minute floor - below this, a review can't realistically do anything useful
	DefaultMaxReviewTimeoutSeconds     = 240 // 4 minutes - matches review.DefaultReviewConfig's ReviewTimeout
)

// Settings is the instance-wide review performance configuration.
type Settings struct {
	// MaxConcurrentBatchesCeiling bounds how high any org's own preference
	// (or the system default) can push per-review batch concurrency.
	MaxConcurrentBatchesCeiling int `json:"max_concurrent_batches_ceiling"`
	// MinReviewTimeoutSeconds/MaxReviewTimeoutSeconds bound the per-review
	// timeout the same way.
	MinReviewTimeoutSeconds int `json:"min_review_timeout_floor_seconds"`
	MaxReviewTimeoutSeconds int `json:"max_review_timeout_ceiling_seconds"`
}

// DefaultSettings returns the settings used when no admin has saved a row yet.
func DefaultSettings() Settings {
	return Settings{
		MaxConcurrentBatchesCeiling: DefaultMaxConcurrentBatchesCeiling,
		MinReviewTimeoutSeconds:     DefaultMinReviewTimeoutSeconds,
		MaxReviewTimeoutSeconds:     DefaultMaxReviewTimeoutSeconds,
	}
}

// Validate checks that the settings are internally consistent.
func (s Settings) Validate() error {
	if s.MaxConcurrentBatchesCeiling <= 0 {
		return fmt.Errorf("max concurrent batches ceiling must be > 0")
	}
	if s.MinReviewTimeoutSeconds <= 0 {
		return fmt.Errorf("min review timeout must be > 0 seconds")
	}
	if s.MaxReviewTimeoutSeconds < s.MinReviewTimeoutSeconds {
		return fmt.Errorf("max review timeout (%ds) must be >= min review timeout (%ds)", s.MaxReviewTimeoutSeconds, s.MinReviewTimeoutSeconds)
	}
	return nil
}

// Load reads the instance-wide settings from system_settings, falling back
// to DefaultSettings when no row has ever been saved.
func Load(ctx context.Context, db *sql.DB) (Settings, error) {
	var data []byte
	err := db.QueryRowContext(ctx, `SELECT data FROM system_settings WHERE name = $1`, settingsRowName).Scan(&data)
	if err != nil {
		if err == sql.ErrNoRows {
			return DefaultSettings(), nil
		}
		return Settings{}, fmt.Errorf("load ai_review_performance settings: %w", err)
	}

	settings := DefaultSettings()
	if err := json.Unmarshal(data, &settings); err != nil {
		return Settings{}, fmt.Errorf("parse ai_review_performance settings: %w", err)
	}
	return settings, nil
}

// Save upserts the instance-wide settings.
func Save(ctx context.Context, db *sql.DB, settings Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal ai_review_performance settings: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO system_settings (name, data)
		VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET data = EXCLUDED.data, updated_at = CURRENT_TIMESTAMP
	`, settingsRowName, data)
	if err != nil {
		return fmt.Errorf("save ai_review_performance settings: %w", err)
	}
	return nil
}

// ResolveMaxConcurrentBatches combines an org's optional preference with the
// system ceiling: NULL preference falls back to the ceiling itself (today's
// best known default - see internal/batch.DefaultMaxConcurrentBatches's
// doc comment on why 16 is a safe, non-guessed starting point), and any
// preference is clamped to [1, ceiling] so an org can only ever turn its own
// concurrency down, never above what the instance allows.
func (s Settings) ResolveMaxConcurrentBatches(orgPreference *int) int {
	ceiling := s.MaxConcurrentBatchesCeiling
	if ceiling <= 0 {
		ceiling = DefaultMaxConcurrentBatchesCeiling
	}
	if orgPreference == nil {
		return ceiling
	}
	return clamp(*orgPreference, 1, ceiling)
}

// ResolveReviewTimeoutSeconds combines an org's optional preference with the
// system floor/ceiling the same way.
func (s Settings) ResolveReviewTimeoutSeconds(orgPreference *int) int {
	floor := s.MinReviewTimeoutSeconds
	if floor <= 0 {
		floor = DefaultMinReviewTimeoutSeconds
	}
	ceiling := s.MaxReviewTimeoutSeconds
	if ceiling < floor {
		ceiling = DefaultMaxReviewTimeoutSeconds
	}
	if orgPreference == nil {
		return ceiling
	}
	return clamp(*orgPreference, floor, ceiling)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
