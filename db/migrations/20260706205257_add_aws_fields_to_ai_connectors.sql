-- migrate:up
ALTER TABLE ai_connectors
ADD COLUMN aws_access_key_id TEXT,
ADD COLUMN aws_region TEXT;

-- migrate:down
ALTER TABLE ai_connectors
DROP COLUMN IF EXISTS aws_region,
DROP COLUMN IF EXISTS aws_access_key_id;
