-- migrate:up
-- License state singleton table
CREATE TABLE IF NOT EXISTS license_state (
    id SMALLINT PRIMARY KEY DEFAULT 1, -- enforce singleton row (id=1)
    token TEXT,                        -- raw JWT token (optional: may decide to store hashed later)
    kid VARCHAR(32),                  -- key id from header
    subject VARCHAR(255),             -- derived subject (e.g., email or licence subject)
    app_name VARCHAR(128),            -- app name claim
    seat_count INTEGER,               -- numeric seat count
    unlimited BOOLEAN DEFAULT FALSE,  -- unlimited flag
    issued_at TIMESTAMP WITH TIME ZONE, -- iat from token if needed
    expires_at TIMESTAMP WITH TIME ZONE, -- exp claim
    last_validated_at TIMESTAMP WITH TIME ZONE, -- last successful online validation time
    last_validation_error_code VARCHAR(64), -- last error code from server (if any)
    validation_failures INTEGER DEFAULT 0, -- consecutive network failure count
    status VARCHAR(32) NOT NULL DEFAULT 'missing', -- missing|active|warning|grace|expired|invalid
    grace_started_at TIMESTAMP WITH TIME ZONE, -- when entered grace
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);

-- Indexes
CREATE UNIQUE INDEX IF NOT EXISTS ux_license_state_singleton ON license_state(id);
CREATE INDEX IF NOT EXISTS idx_license_state_expires_at ON license_state(expires_at);
CREATE INDEX IF NOT EXISTS idx_license_state_status ON license_state(status);

-- Trigger (optional) to auto-update updated_at (Postgres example)
CREATE OR REPLACE FUNCTION license_state_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_license_state_updated_at ON license_state;
CREATE TRIGGER trg_license_state_updated_at
BEFORE UPDATE ON license_state
FOR EACH ROW
EXECUTE PROCEDURE license_state_set_updated_at();

-- migrate:down
DROP TRIGGER IF EXISTS trg_license_state_updated_at ON license_state;
DROP FUNCTION IF EXISTS license_state_set_updated_at();
DROP TABLE IF EXISTS license_state;
