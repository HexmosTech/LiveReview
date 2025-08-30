-- migrate:up
ALTER TABLE integration_tokens
ADD COLUMN connection_name TEXT;

-- migrate:down
ALTER TABLE integration_tokens
DROP COLUMN IF EXISTS connection_name;

