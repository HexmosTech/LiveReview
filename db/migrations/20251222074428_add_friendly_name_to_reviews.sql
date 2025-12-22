-- migrate:up
ALTER TABLE reviews ADD COLUMN friendly_name TEXT;

-- migrate:down
ALTER TABLE reviews DROP COLUMN IF EXISTS friendly_name

