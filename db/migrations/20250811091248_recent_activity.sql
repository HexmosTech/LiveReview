-- migrate:up

-- Create recent_activity table to track user actions
CREATE TABLE recent_activity (
    id SERIAL PRIMARY KEY,
    activity_type VARCHAR(50) NOT NULL,  -- 'review_triggered', 'connector_created', 'webhook_installed', etc.
    event_data JSONB NOT NULL DEFAULT '{}',  -- All event data stored as JSON
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create indexes for efficient querying
CREATE INDEX idx_recent_activity_type ON recent_activity(activity_type);
CREATE INDEX idx_recent_activity_created_at ON recent_activity(created_at DESC);

-- Create a composite index for dashboard queries
CREATE INDEX idx_recent_activity_dashboard ON recent_activity(created_at DESC, activity_type);

-- migrate:down

-- Drop indexes first
CREATE INDEX idx_recent_activity_dashboard ON recent_activity(created_at DESC, activity_type);
DROP INDEX IF EXISTS idx_recent_activity_created_at;
DROP INDEX IF EXISTS idx_recent_activity_type;

-- Drop table
DROP TABLE IF EXISTS recent_activity;

