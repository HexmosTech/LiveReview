-- migrate:up
-- Adaptive Review (leader + helper model) is now the default experience for
-- orgs that haven't configured org_review_ai_settings yet. This only changes
-- the column default for new rows — existing orgs' rows (and their explicit
-- choice) are left untouched; see storage/aiconnectors/review_ai_settings_store.go's
-- GetByOrgID zero-value fallback for the matching app-level default.
ALTER TABLE org_review_ai_settings ALTER COLUMN helper_enabled SET DEFAULT true;

-- migrate:down
ALTER TABLE org_review_ai_settings ALTER COLUMN helper_enabled SET DEFAULT false;
