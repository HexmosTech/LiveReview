-- migrate:up

-- Add org tracking and other missing columns to subscriptions
ALTER TABLE subscriptions 
  ADD COLUMN org_id BIGINT REFERENCES orgs(id),
  ADD COLUMN license_expires_at TIMESTAMP WITH TIME ZONE,
  ADD COLUMN notes JSONB,
  ADD COLUMN updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW();

CREATE INDEX idx_subscriptions_org ON subscriptions(org_id);

-- Update license_log to use event_type and description instead of action
ALTER TABLE license_log RENAME COLUMN action TO event_type;
ALTER TABLE license_log ADD COLUMN description TEXT;
ALTER TABLE license_log RENAME COLUMN payload TO metadata;

-- migrate:down

ALTER TABLE license_log
  RENAME COLUMN metadata TO payload,
  DROP COLUMN IF EXISTS description,
  RENAME COLUMN event_type TO action;

DROP INDEX IF EXISTS idx_subscriptions_org;

ALTER TABLE subscriptions
  DROP COLUMN IF EXISTS updated_at,
  DROP COLUMN IF EXISTS notes,
  DROP COLUMN IF EXISTS license_expires_at,
  DROP COLUMN IF EXISTS org_id;

