-- migrate:up
ALTER TABLE users ADD COLUMN IF NOT EXISTS time_zone TEXT;

-- migrate:down
ALTER TABLE users DROP COLUMN IF EXISTS time_zone;
