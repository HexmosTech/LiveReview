-- migrate:up

CREATE TABLE org_teams_configs (
    id           BIGSERIAL PRIMARY KEY,
    org_id       BIGINT NOT NULL UNIQUE REFERENCES orgs(id) ON DELETE CASCADE,
    bot_app_id   TEXT NOT NULL,
    bot_password TEXT NOT NULL,
    api_key      TEXT NOT NULL DEFAULT '',
    tenant_id    TEXT NOT NULL DEFAULT '',
    enabled      BOOLEAN NOT NULL DEFAULT false,
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_org_teams_configs_org_id ON org_teams_configs(org_id);
CREATE INDEX idx_org_teams_configs_enabled ON org_teams_configs(enabled);

COMMENT ON TABLE org_teams_configs IS 'Per-org Microsoft Teams bot configuration';
COMMENT ON COLUMN org_teams_configs.bot_app_id IS 'Microsoft App ID for the Teams bot';
COMMENT ON COLUMN org_teams_configs.bot_password IS 'Microsoft App Password (client secret) for the Teams bot';

-- migrate:down

DROP TABLE IF EXISTS org_teams_configs;
