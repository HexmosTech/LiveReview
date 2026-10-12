-- migrate:up

-- Set once the "Start a review / View history" links comment is posted on a new PR/MR,
-- so duplicate webhook deliveries don't post it twice.
ALTER TABLE pull_requests ADD COLUMN links_comment_posted_at TIMESTAMPTZ;

-- migrate:down

ALTER TABLE pull_requests DROP COLUMN IF EXISTS links_comment_posted_at;
