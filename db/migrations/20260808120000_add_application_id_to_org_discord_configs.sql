-- migrate:up

ALTER TABLE org_discord_configs
    ADD COLUMN application_id TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN org_discord_configs.application_id IS 'Discord Application ID, used to build the OAuth2 bot invite URL';

-- migrate:down

ALTER TABLE org_discord_configs
    DROP COLUMN IF EXISTS application_id;
