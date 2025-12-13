-- migrate:up
-- Add cancel_at_period_end column to subscriptions table
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS cancel_at_period_end BOOLEAN DEFAULT FALSE;

-- Update existing cancelled subscriptions (best effort guess based on status)
UPDATE subscriptions 
SET cancel_at_period_end = TRUE 
WHERE status = 'cancelled' AND current_period_end > NOW();

-- migrate:down
ALTER TABLE subscriptions DROP COLUMN IF EXISTS cancel_at_period_end;
