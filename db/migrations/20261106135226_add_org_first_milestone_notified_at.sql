-- migrate:up

-- One-time Discord milestone pings (first GitHub connect, first review
-- submitted) per org. NULL means "not yet notified"; set exactly once via
-- an atomic UPDATE ... WHERE ... IS NULL claim, same idiom as
-- reviews.first_finding_notified_at.
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS first_github_connect_notified_at TIMESTAMPTZ;
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS first_review_notified_at TIMESTAMPTZ;

-- migrate:down

ALTER TABLE orgs DROP COLUMN IF EXISTS first_github_connect_notified_at;
ALTER TABLE orgs DROP COLUMN IF EXISTS first_review_notified_at;
