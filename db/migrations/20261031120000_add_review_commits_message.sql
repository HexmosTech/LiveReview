-- migrate:up

-- Commit message, captured best-effort from whichever provider API call already
-- fetched the commit SHA (GitHub/GitLab PR-commits endpoints both return it). Null
-- for older rows and for sources that never had one available (e.g. local staged
-- commits recorded before a message was ever sent).
ALTER TABLE review_commits ADD COLUMN IF NOT EXISTS message TEXT;

-- migrate:down
ALTER TABLE review_commits DROP COLUMN IF EXISTS message;
