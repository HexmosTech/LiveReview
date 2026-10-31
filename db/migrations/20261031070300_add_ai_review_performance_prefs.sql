-- migrate:up

-- Optional per-org soft preferences for review concurrency/timeout. NULL
-- means "use the system default" (resolved at review-trigger time, see
-- internal/aiselection's review-performance resolver) - these are not hard
-- caps the org can set arbitrarily high; the resolved value is always
-- clamped to the system-wide ceiling in system_settings (row
-- 'ai_review_performance'), set by a super admin / self-hosted owner.
ALTER TABLE org_review_ai_settings
    ADD COLUMN IF NOT EXISTS preferred_max_concurrent_batches INTEGER,
    ADD COLUMN IF NOT EXISTS preferred_review_timeout_seconds INTEGER;

ALTER TABLE org_review_ai_settings
    ADD CONSTRAINT org_review_ai_settings_preferred_max_concurrent_batches_check
        CHECK (preferred_max_concurrent_batches IS NULL OR preferred_max_concurrent_batches > 0),
    ADD CONSTRAINT org_review_ai_settings_preferred_review_timeout_seconds_check
        CHECK (preferred_review_timeout_seconds IS NULL OR preferred_review_timeout_seconds > 0);

-- migrate:down
ALTER TABLE org_review_ai_settings
    DROP CONSTRAINT IF EXISTS org_review_ai_settings_preferred_max_concurrent_batches_check,
    DROP CONSTRAINT IF EXISTS org_review_ai_settings_preferred_review_timeout_seconds_check;

ALTER TABLE org_review_ai_settings
    DROP COLUMN IF EXISTS preferred_max_concurrent_batches,
    DROP COLUMN IF EXISTS preferred_review_timeout_seconds;
