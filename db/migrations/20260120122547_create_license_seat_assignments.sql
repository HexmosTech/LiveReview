-- migrate:up

-- License seat assignments table for self-hosted deployments
-- Tracks which users have been assigned license seats
-- Similar pattern to cloud's subscription-based assignments via user_roles.active_subscription_id
CREATE TABLE IF NOT EXISTS license_seat_assignments (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    assigned_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    assigned_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    revoked_at TIMESTAMP WITH TIME ZONE NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

-- Only one active assignment per user
CREATE UNIQUE INDEX IF NOT EXISTS idx_license_seat_assignments_user_active 
    ON license_seat_assignments(user_id) WHERE is_active = TRUE;

-- For counting active assignments
CREATE INDEX IF NOT EXISTS idx_license_seat_assignments_active 
    ON license_seat_assignments(is_active) WHERE is_active = TRUE;

-- For audit trail
CREATE INDEX IF NOT EXISTS idx_license_seat_assignments_assigned_by 
    ON license_seat_assignments(assigned_by_user_id);

-- Trigger to update updated_at
CREATE OR REPLACE FUNCTION license_seat_assignments_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_license_seat_assignments_updated_at ON license_seat_assignments;
CREATE TRIGGER trg_license_seat_assignments_updated_at
BEFORE UPDATE ON license_seat_assignments
FOR EACH ROW
EXECUTE PROCEDURE license_seat_assignments_set_updated_at();

-- migrate:down
DROP TRIGGER IF EXISTS trg_license_seat_assignments_updated_at ON license_seat_assignments;
DROP FUNCTION IF EXISTS license_seat_assignments_set_updated_at();
DROP TABLE IF EXISTS license_seat_assignments;
