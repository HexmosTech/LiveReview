-- migrate:up
ALTER TABLE ai_connectors 
ADD COLUMN base_url TEXT,
ADD COLUMN selected_model TEXT;

-- Add indexes for better query performance
CREATE INDEX idx_ai_connectors_provider_name ON ai_connectors(provider_name);

-- migrate:down
DROP INDEX IF EXISTS idx_ai_connectors_provider_name;
ALTER TABLE ai_connectors 
DROP COLUMN IF EXISTS selected_model,
DROP COLUMN IF EXISTS base_url;
