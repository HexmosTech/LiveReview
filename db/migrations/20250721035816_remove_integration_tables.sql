-- migrate:up

-- First, add gitlab_url to the integration_tokens table if it doesn't exist
-- The column already exists but we'll make it NOT NULL
ALTER TABLE integration_tokens 
  ALTER COLUMN provider_url TYPE text,
  ALTER COLUMN provider_url SET NOT NULL,
  ALTER COLUMN connection_name SET NOT NULL;

-- Update any missing gitlab_url values from integration_tables
UPDATE integration_tokens t
SET provider_url = (
  SELECT i.metadata->>'gitlab_url'
  FROM integration_tables i
  WHERE i.provider = t.provider AND i.provider_app_id = t.provider_app_id
)
WHERE t.provider = 'gitlab' AND (t.provider_url IS NULL OR t.provider_url = '');

-- Drop constraints and sequence
ALTER TABLE integration_tables DROP CONSTRAINT IF EXISTS integration_tables_pkey;

-- Drop the integration_tables table
DROP TABLE IF EXISTS integration_tables;

-- Drop the sequence
DROP SEQUENCE IF EXISTS integration_tables_id_seq;

-- migrate:down

-- Recreate the sequence
CREATE SEQUENCE IF NOT EXISTS integration_tables_id_seq
  START WITH 1
  INCREMENT BY 1
  NO MINVALUE
  NO MAXVALUE
  CACHE 1;

-- Recreate the table
CREATE TABLE IF NOT EXISTS integration_tables (
  id bigint NOT NULL DEFAULT nextval('integration_tables_id_seq'::regclass),
  provider text NOT NULL,
  provider_app_id text NOT NULL,
  connection_name text NOT NULL,
  metadata jsonb DEFAULT '{}'::jsonb,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  updated_at timestamp with time zone DEFAULT now() NOT NULL,
  PRIMARY KEY (id)
);

-- Populate the table based on integration_tokens data
INSERT INTO integration_tables (provider, provider_app_id, connection_name, metadata)
SELECT 
  provider, 
  provider_app_id, 
  connection_name, 
  jsonb_build_object('gitlab_url', provider_url)
FROM integration_tokens
WHERE provider = 'gitlab'
ON CONFLICT DO NOTHING;