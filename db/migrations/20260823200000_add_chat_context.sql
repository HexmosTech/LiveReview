-- migrate:up

-- Adds "context" - who/what a chart or export is scoped to, as a single
-- JSON object {"organization": "...", "repository": [...], "person": [...]}
-- - to the two tables added in 20260817120000 and 20260820141616.

ALTER TABLE chat_charts ADD COLUMN context JSONB;
ALTER TABLE chat_files ADD COLUMN context JSONB;

-- migrate:down

ALTER TABLE chat_charts DROP COLUMN IF EXISTS context;
ALTER TABLE chat_files DROP COLUMN IF EXISTS context;
