-- migrate:up

CREATE TABLE scheduled_review_configs (
    id                    BIGSERIAL PRIMARY KEY,
    org_id                BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    integration_token_id  BIGINT NOT NULL REFERENCES integration_tokens(id) ON DELETE CASCADE,
    project_full_name     TEXT NOT NULL,
    enabled               BOOLEAN NOT NULL DEFAULT true,
    interval_hours        INT NOT NULL DEFAULT 24,
    default_branch        TEXT,
    last_synced_sha       TEXT,
    last_run_at           TIMESTAMP WITH TIME ZONE,
    next_run_at           TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at            TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE (integration_token_id, project_full_name)
);

CREATE INDEX idx_scheduled_review_configs_org_id ON scheduled_review_configs(org_id);
CREATE INDEX idx_scheduled_review_configs_due ON scheduled_review_configs(next_run_at) WHERE enabled = true;

COMMENT ON TABLE scheduled_review_configs IS 'Per-repo configuration for periodic default-branch reviews';
COMMENT ON COLUMN scheduled_review_configs.last_synced_sha IS 'Checkpoint SHA used as the base for the next scheduled diff';

-- migrate:down

DROP TABLE IF EXISTS scheduled_review_configs;
