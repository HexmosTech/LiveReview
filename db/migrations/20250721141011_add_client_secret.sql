-- migrate:up
ALTER TABLE integration_tokens ADD COLUMN client_secret text;

-- migrate:down
ALTER TABLE integration_tokens DROP COLUMN client_secret;

