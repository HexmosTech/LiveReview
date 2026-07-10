package aiconnectors

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const (
	AIConnectorRoleLeader = "leader"
	AIConnectorRoleHelper = "helper"

	HelperModeConciseThenExpand = "concise_then_expand"
	HelperModePolishOnly        = "polish_only"
)

type ReviewAISettings struct {
	OrgID         int64
	HelperEnabled bool
	HelperMode    string
}

type ReviewAISettingsStore struct {
	db *sql.DB
}

func NewReviewAISettingsStore(db *sql.DB) *ReviewAISettingsStore {
	return &ReviewAISettingsStore{db: db}
}

func NormalizeConnectorRole(role string) string {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case "", AIConnectorRoleLeader:
		return AIConnectorRoleLeader
	case AIConnectorRoleHelper:
		return AIConnectorRoleHelper
	default:
		return ""
	}
}

func NormalizeHelperMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "", HelperModeConciseThenExpand:
		return HelperModeConciseThenExpand
	case HelperModePolishOnly:
		return HelperModePolishOnly
	default:
		return ""
	}
}

func (s *ReviewAISettingsStore) GetByOrgID(ctx context.Context, orgID int64) (ReviewAISettings, error) {
	if s == nil || s.db == nil {
		return ReviewAISettings{}, fmt.Errorf("review ai settings store is not initialized")
	}

	// Adaptive Review defaults to on for orgs with no settings row yet
	// (brand-new orgs, or any org that's never touched the AI Providers
	// page). Orgs with an existing row keep whatever they've explicitly
	// set — this only affects the sql.ErrNoRows fallback below.
	settings := ReviewAISettings{
		OrgID:         orgID,
		HelperEnabled: true,
		HelperMode:    HelperModeConciseThenExpand,
	}

	err := s.db.QueryRowContext(ctx, `
		SELECT helper_enabled, helper_mode
		FROM org_review_ai_settings
		WHERE org_id = $1
	`, orgID).Scan(&settings.HelperEnabled, &settings.HelperMode)
	if err != nil {
		if err == sql.ErrNoRows {
			return settings, nil
		}
		return ReviewAISettings{}, fmt.Errorf("get review ai settings: %w", err)
	}

	if normalized := NormalizeHelperMode(settings.HelperMode); normalized != "" {
		settings.HelperMode = normalized
	} else {
		settings.HelperMode = HelperModeConciseThenExpand
	}

	return settings, nil
}

func (s *ReviewAISettingsStore) Upsert(ctx context.Context, settings ReviewAISettings) (ReviewAISettings, error) {
	if s == nil || s.db == nil {
		return ReviewAISettings{}, fmt.Errorf("review ai settings store is not initialized")
	}
	if settings.OrgID <= 0 {
		return ReviewAISettings{}, fmt.Errorf("org id must be > 0")
	}

	normalizedMode := NormalizeHelperMode(settings.HelperMode)
	if normalizedMode == "" {
		return ReviewAISettings{}, fmt.Errorf("invalid helper mode: %s", settings.HelperMode)
	}

	settings.HelperMode = normalizedMode

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO org_review_ai_settings (org_id, helper_enabled, helper_mode, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (org_id)
		DO UPDATE SET helper_enabled = EXCLUDED.helper_enabled,
		              helper_mode = EXCLUDED.helper_mode,
		              updated_at = NOW()
	`, settings.OrgID, settings.HelperEnabled, settings.HelperMode); err != nil {
		return ReviewAISettings{}, fmt.Errorf("upsert review ai settings: %w", err)
	}

	return settings, nil
}
