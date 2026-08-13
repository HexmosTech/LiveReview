-- migrate:up

ALTER TABLE org_slack_configs ADD COLUMN app_token TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN org_slack_configs.app_token IS 'Slack Socket Mode app-level token (xapp-...) owned by the installing org, used to open the real-time events socket';

-- migrate:down

ALTER TABLE org_slack_configs DROP COLUMN app_token;
