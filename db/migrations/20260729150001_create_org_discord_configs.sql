-- migrate:up

CREATE TABLE org_discord_configs (
    id            BIGSERIAL PRIMARY KEY,
    org_id        BIGINT NOT NULL UNIQUE REFERENCES orgs(id) ON DELETE CASCADE,
    bot_token     TEXT NOT NULL,
    api_key       TEXT NOT NULL DEFAULT '',
    guild_id      TEXT NOT NULL DEFAULT '',
    enabled       BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_org_discord_configs_org_id ON org_discord_configs(org_id);
CREATE INDEX idx_org_discord_configs_enabled ON org_discord_configs(enabled);
CREATE INDEX idx_org_discord_configs_guild_id ON org_discord_configs(guild_id);

COMMENT ON TABLE org_discord_configs IS 'Per-org Discord bot configuration';
COMMENT ON COLUMN org_discord_configs.bot_token IS 'Discord bot token';
COMMENT ON COLUMN org_discord_configs.guild_id IS 'Discord guild (server) ID, learned after first auth test';

-- migrate:down

DROP TABLE IF EXISTS org_discord_configs;
