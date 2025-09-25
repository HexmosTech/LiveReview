-- migrate:up
ALTER TABLE reviews
    ADD COLUMN mr_title TEXT,
    ADD COLUMN author_name TEXT,
    ADD COLUMN author_username TEXT;

-- migrate:down
ALTER TABLE reviews
    DROP COLUMN IF EXISTS mr_title,
    DROP COLUMN IF EXISTS author_name,
    DROP COLUMN IF EXISTS author_username;
