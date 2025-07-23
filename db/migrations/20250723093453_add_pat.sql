-- migrate:up
ALTER TABLE integration_tokens ADD COLUMN pat_token TEXT;


-- migrate:down
ALTER TABLE integration_tokens DROP COLUMN pat_token;

