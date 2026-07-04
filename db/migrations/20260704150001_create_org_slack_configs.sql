-- migrate:up

CREATE TABLE org_slack_configs (
    id         BIGSERIAL PRIMARY KEY,
    org_id     BIGINT NOT NULL UNIQUE REFERENCES orgs(id) ON DELETE CASCADE,
    bot_token  TEXT NOT NULL,
    api_key    TEXT NOT NULL,
    team_id    TEXT NOT NULL DEFAULT '',
    enabled    BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_org_slack_configs_org_id ON org_slack_configs(org_id);
CREATE INDEX idx_org_slack_configs_team_id ON org_slack_configs(team_id);
CREATE INDEX idx_org_slack_configs_enabled ON org_slack_configs(enabled);

COMMENT ON TABLE org_slack_configs IS 'Per-org Slack bot configuration';
COMMENT ON COLUMN org_slack_configs.team_id IS 'Slack workspace team ID, learned after first auth test';

-- migrate:down

DROP TABLE IF EXISTS org_slack_configs;
