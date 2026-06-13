-- migrate:up
ALTER TABLE ai_connectors
ADD COLUMN gcp_project_id TEXT,
ADD COLUMN gcp_location TEXT;

-- migrate:down
ALTER TABLE ai_connectors
DROP COLUMN IF EXISTS gcp_location,
DROP COLUMN IF EXISTS gcp_project_id;
