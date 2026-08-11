-- migrate:up

DROP TABLE IF EXISTS scheduled_review_configs;

CREATE TABLE scheduled_review_configs (
    id                    BIGSERIAL PRIMARY KEY,
    org_id                BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    repository_id         BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    enabled               BOOLEAN NOT NULL DEFAULT true,
    cron_expression       TEXT NOT NULL DEFAULT '0 9 * * *',
    default_branch        TEXT,
    last_synced_sha       TEXT,
    last_run_at           TIMESTAMP WITH TIME ZONE,
    next_run_at           TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at            TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE (repository_id)
);

CREATE INDEX IF NOT EXISTS idx_scheduled_review_configs_org_id ON scheduled_review_configs(org_id);
CREATE INDEX IF NOT EXISTS idx_scheduled_review_configs_due ON scheduled_review_configs(next_run_at) WHERE enabled = true;

COMMENT ON TABLE scheduled_review_configs IS 'Per-repo configuration for periodic default-branch reviews';
COMMENT ON COLUMN scheduled_review_configs.repository_id IS 'One config per repo - the repositories row already carries full_name/connector_id/org_id, so those are joined rather than duplicated here';
COMMENT ON COLUMN scheduled_review_configs.cron_expression IS 'Standard 5-field cron expression (UTC) - the frontend converts the user''s local time before sending';
COMMENT ON COLUMN scheduled_review_configs.last_synced_sha IS 'Checkpoint SHA used as the base for the next scheduled diff';

-- migrate:down

DROP TABLE IF EXISTS scheduled_review_configs;
