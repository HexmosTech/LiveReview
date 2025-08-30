-- migrate:up
ALTER TABLE integration_tokens
ADD COLUMN code TEXT;

-- migrate:down
ALTER TABLE integration_tokens
DROP COLUMN IF EXISTS code;
