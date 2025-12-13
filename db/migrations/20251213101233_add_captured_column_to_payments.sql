-- migrate:up

-- Add the 'captured' boolean column to subscription_payments table
-- This is used by webhook handlers to track whether a payment has been captured
ALTER TABLE subscription_payments 
  ADD COLUMN captured BOOLEAN DEFAULT FALSE NOT NULL;

-- Create an index for faster queries on captured status
CREATE INDEX idx_subscription_payments_captured_bool ON subscription_payments(captured);

COMMENT ON COLUMN subscription_payments.captured IS 'Whether the payment has been captured (true) or just authorized (false)';

-- migrate:down

DROP INDEX IF EXISTS idx_subscription_payments_captured_bool;

ALTER TABLE subscription_payments 
  DROP COLUMN IF EXISTS captured;
