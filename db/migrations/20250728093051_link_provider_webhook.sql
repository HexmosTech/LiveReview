-- migrate:up

-- Add integration_token_id column to webhook_registry table
ALTER TABLE webhook_registry 
ADD COLUMN integration_token_id bigint;

-- Add foreign key constraint linking webhook_registry to integration_tokens
ALTER TABLE webhook_registry 
ADD CONSTRAINT fk_webhook_registry_integration_token 
FOREIGN KEY (integration_token_id) 
REFERENCES integration_tokens(id) 
ON DELETE CASCADE;

-- Add index for better query performance
CREATE INDEX idx_webhook_registry_integration_token_id 
ON webhook_registry(integration_token_id);

-- migrate:down

-- Remove the index
DROP INDEX IF EXISTS idx_webhook_registry_integration_token_id;

-- Remove the foreign key constraint
ALTER TABLE webhook_registry 
DROP CONSTRAINT IF EXISTS fk_webhook_registry_integration_token;

-- Remove the column
ALTER TABLE webhook_registry 
DROP COLUMN IF EXISTS integration_token_id;

