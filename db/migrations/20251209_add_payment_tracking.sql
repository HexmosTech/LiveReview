-- migrate:up

-- ==================================================================================
-- ADD PAYMENT TRACKING TO SUBSCRIPTIONS TABLE
-- Track Razorpay payment information for each subscription
-- A subscription can have multiple payments (initial + renewals)
-- ==================================================================================

-- Add payment fields to subscriptions table
ALTER TABLE subscriptions 
  ADD COLUMN last_payment_id VARCHAR(255),
  ADD COLUMN last_payment_status VARCHAR(50),
  ADD COLUMN last_payment_received_at TIMESTAMP WITH TIME ZONE,
  ADD COLUMN payment_verified BOOLEAN DEFAULT FALSE NOT NULL;

-- Index for payment status queries
CREATE INDEX idx_subscriptions_payment_verified ON subscriptions(payment_verified);
CREATE INDEX idx_subscriptions_payment_status ON subscriptions(last_payment_status);

COMMENT ON COLUMN subscriptions.last_payment_id IS 'Razorpay payment ID from most recent payment';
COMMENT ON COLUMN subscriptions.last_payment_status IS 'Status of last payment: authorized, captured, failed, refunded';
COMMENT ON COLUMN subscriptions.last_payment_received_at IS 'Timestamp when payment was actually received (captured)';
COMMENT ON COLUMN subscriptions.payment_verified IS 'Whether any payment has been successfully received for this subscription';

-- ==================================================================================
-- CREATE PAYMENTS TABLE
-- Track all payments related to subscriptions (not just the last one)
-- This is for complete audit trail and historical payment tracking
-- ==================================================================================
CREATE TABLE subscription_payments (
    id BIGSERIAL PRIMARY KEY,
    
    -- References (nullable for orphaned payments where we can't find the subscription)
    subscription_id BIGINT REFERENCES subscriptions(id),
    
    -- Razorpay Payment Details
    razorpay_payment_id VARCHAR(255) UNIQUE NOT NULL,
    razorpay_order_id VARCHAR(255),
    razorpay_invoice_id VARCHAR(255),
    
    -- Payment Details
    amount BIGINT NOT NULL,  -- in paise (INR smallest unit)
    currency VARCHAR(10) DEFAULT 'INR' NOT NULL,
    status VARCHAR(50) NOT NULL,  -- authorized, captured, failed, refunded
    method VARCHAR(50),  -- card, netbanking, wallet, upi, etc.
    
    -- Payment Lifecycle
    authorized_at TIMESTAMP WITH TIME ZONE,
    captured_at TIMESTAMP WITH TIME ZONE,
    failed_at TIMESTAMP WITH TIME ZONE,
    refunded_at TIMESTAMP WITH TIME ZONE,
    
    -- Razorpay Metadata
    razorpay_data JSONB,
    
    -- Error tracking
    error_code VARCHAR(100),
    error_description TEXT,
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_subscription_payments_subscription ON subscription_payments(subscription_id);
CREATE INDEX idx_subscription_payments_razorpay ON subscription_payments(razorpay_payment_id);
CREATE INDEX idx_subscription_payments_status ON subscription_payments(status);
CREATE INDEX idx_subscription_payments_captured ON subscription_payments(captured_at) 
  WHERE captured_at IS NOT NULL;

COMMENT ON TABLE subscription_payments IS 'Complete history of all payments for subscriptions';
COMMENT ON COLUMN subscription_payments.amount IS 'Amount in smallest currency unit (paise for INR)';
COMMENT ON COLUMN subscription_payments.status IS 'Payment status: authorized, captured, failed, refunded';

-- migrate:down

DROP TABLE IF EXISTS subscription_payments CASCADE;

DROP INDEX IF EXISTS idx_subscriptions_payment_status;
DROP INDEX IF EXISTS idx_subscriptions_payment_verified;

ALTER TABLE subscriptions 
  DROP COLUMN IF EXISTS payment_verified,
  DROP COLUMN IF EXISTS last_payment_received_at,
  DROP COLUMN IF EXISTS last_payment_status,
  DROP COLUMN IF EXISTS last_payment_id;
