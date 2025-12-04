-- migrate:up

-- ==================================================================================
-- SUBSCRIPTIONS TABLE
-- Track Razorpay subscriptions owned by users
-- Licenses can be assigned to any user in any org owned by the subscription owner
-- ==================================================================================
CREATE TABLE subscriptions (
    id BIGSERIAL PRIMARY KEY,
    
    -- Razorpay Integration
    razorpay_subscription_id VARCHAR(255) UNIQUE NOT NULL,
    razorpay_plan_id VARCHAR(255) NOT NULL,
    
    -- Ownership (subscriptions owned by users, not orgs)
    owner_user_id BIGINT NOT NULL REFERENCES users(id),
    
    -- Subscription Details
    plan_type VARCHAR(50) NOT NULL, -- 'team_monthly' | 'team_annual'
    quantity INT NOT NULL, -- number of seats purchased
    assigned_seats INT DEFAULT 0 NOT NULL, -- number of seats currently assigned (denormalized counter)
    status VARCHAR(50) NOT NULL, -- 'created' | 'active' | 'cancelled' | 'expired'
    
    -- Billing Cycle
    current_period_start TIMESTAMP WITH TIME ZONE,
    current_period_end TIMESTAMP WITH TIME ZONE,
    
    -- Lifecycle Timestamps
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    activated_at TIMESTAMP WITH TIME ZONE,
    cancelled_at TIMESTAMP WITH TIME ZONE,
    expired_at TIMESTAMP WITH TIME ZONE,
    
    -- Razorpay Metadata (JSON for flexibility)
    razorpay_data JSONB,
    
    CONSTRAINT valid_quantity CHECK (quantity > 0),
    CONSTRAINT valid_assigned_seats CHECK (assigned_seats >= 0 AND assigned_seats <= quantity)
);

CREATE INDEX idx_subscriptions_owner ON subscriptions(owner_user_id);
CREATE INDEX idx_subscriptions_status ON subscriptions(status);
CREATE INDEX idx_subscriptions_razorpay ON subscriptions(razorpay_subscription_id);

-- ==================================================================================
-- USER_ROLES TABLE EXTENSION
-- Extend existing user_roles junction table to store per-org license info
-- This allows same user to have different plans in different orgs
-- ==================================================================================
ALTER TABLE user_roles 
  ADD COLUMN plan_type VARCHAR(50) DEFAULT 'free' NOT NULL,
  ADD COLUMN license_expires_at TIMESTAMP WITH TIME ZONE,
  ADD COLUMN active_subscription_id BIGINT REFERENCES subscriptions(id);

CREATE INDEX idx_user_roles_plan_type ON user_roles(plan_type);
CREATE INDEX idx_user_roles_license_expires ON user_roles(license_expires_at) 
  WHERE license_expires_at IS NOT NULL;
CREATE INDEX idx_user_roles_subscription ON user_roles(active_subscription_id)
  WHERE active_subscription_id IS NOT NULL;

-- ==================================================================================
-- LICENSE_LOG TABLE
-- Unified audit trail for license actions AND payment events
-- Immutable append-only log for complete subscription history
-- ==================================================================================
CREATE TABLE license_log (
    id BIGSERIAL PRIMARY KEY,
    
    -- Relationships (nullable for payment-only events)
    subscription_id BIGINT REFERENCES subscriptions(id),
    user_id BIGINT REFERENCES users(id),  -- null for payment events
    org_id BIGINT REFERENCES orgs(id),    -- null for payment events
    
    -- Action Details
    action VARCHAR(100) NOT NULL,  -- 'assigned'|'revoked'|'expired'|'subscription.activated'|'payment.captured'|etc.
    actor_id BIGINT REFERENCES users(id),  -- null for webhook events
    
    -- Razorpay Integration (for payment events)
    razorpay_event_id VARCHAR(255) UNIQUE,  -- null for license actions, unique for webhooks
    
    -- Event Data
    payload JSONB,  -- flexible storage for action-specific data
    
    -- Processing Status (for webhook events)
    processed BOOLEAN DEFAULT TRUE,  -- false only for unprocessed webhooks
    processed_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT,
    
    -- Timestamp
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_license_log_subscription ON license_log(subscription_id);
CREATE INDEX idx_license_log_user ON license_log(user_id);
CREATE INDEX idx_license_log_action ON license_log(action);
CREATE INDEX idx_license_log_processed ON license_log(processed) WHERE processed = false;
CREATE INDEX idx_license_log_razorpay ON license_log(razorpay_event_id) WHERE razorpay_event_id IS NOT NULL;

-- migrate:down

DROP TABLE IF EXISTS license_log CASCADE;

DROP INDEX IF EXISTS idx_user_roles_subscription;
DROP INDEX IF EXISTS idx_user_roles_license_expires;
DROP INDEX IF EXISTS idx_user_roles_plan_type;
ALTER TABLE user_roles 
  DROP COLUMN IF EXISTS active_subscription_id,
  DROP COLUMN IF EXISTS license_expires_at,
  DROP COLUMN IF EXISTS plan_type;

DROP TABLE IF EXISTS subscriptions CASCADE;

