-- migrate:up

-- Add short_url column to subscriptions table
-- This is the Razorpay-provided public link for customers to manage their subscription
ALTER TABLE subscriptions 
  ADD COLUMN short_url VARCHAR(500);

CREATE INDEX idx_subscriptions_short_url ON subscriptions(short_url) 
  WHERE short_url IS NOT NULL;

COMMENT ON COLUMN subscriptions.short_url IS 'Razorpay public link for customers to manage subscription (no login required)';

-- migrate:down

DROP INDEX IF EXISTS idx_subscriptions_short_url;

ALTER TABLE subscriptions 
  DROP COLUMN IF EXISTS short_url;
