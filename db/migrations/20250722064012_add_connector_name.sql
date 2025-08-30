-- migrate:up
ALTER TABLE ai_connectors
ADD COLUMN connector_name VARCHAR(128);

-- Add a comment to the column
COMMENT ON COLUMN ai_connectors.connector_name IS 'A user-friendly name for the connector';

-- Update existing records to have a default name based on provider_name
UPDATE ai_connectors
SET connector_name = CONCAT(provider_name, '-', id)
WHERE connector_name IS NULL;

-- migrate:down
ALTER TABLE ai_connectors
DROP COLUMN connector_name;

