-- migrate:up

CREATE TABLE scheduled_review_runs (
    id             BIGSERIAL PRIMARY KEY,
    config_id      BIGINT NOT NULL REFERENCES scheduled_review_configs(id) ON DELETE CASCADE,
    review_id      BIGINT REFERENCES reviews(id) ON DELETE SET NULL,
    outcome        TEXT NOT NULL,
    branch         TEXT,
    base_sha       TEXT,
    head_sha       TEXT,
    commit_count   INT NOT NULL DEFAULT 0,
    error_message  TEXT,
    started_at     TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    completed_at   TIMESTAMP WITH TIME ZONE,
    CONSTRAINT scheduled_review_runs_outcome_check CHECK (outcome IN ('reviewed', 'no_changes', 'failed', 'skipped_unsupported_provider', 'quota_blocked'))
);

CREATE INDEX idx_scheduled_review_runs_config_id ON scheduled_review_runs(config_id, started_at DESC);

COMMENT ON TABLE scheduled_review_runs IS 'One row per scheduler attempt (not per successful review) - lets the UI show "did it run, and what happened" even for cron ticks that found nothing to review. repository_id/org_id are deliberately not stored here - derive via config_id, same pattern as scheduled_review_configs itself not duplicating repositories.full_name/connector_id.';
COMMENT ON COLUMN scheduled_review_runs.review_id IS 'Set only when outcome = reviewed; the actual AI review record this run produced. Issue/severity counts live on the review itself, not duplicated here.';

-- migrate:down

DROP TABLE IF EXISTS scheduled_review_runs;
