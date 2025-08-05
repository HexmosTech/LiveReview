-- migrate:up
CREATE TABLE IF NOT EXISTS dashboard_cache (
    id INTEGER PRIMARY KEY DEFAULT 1,
    data JSONB NOT NULL DEFAULT '{}',
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT single_dashboard_row CHECK (id = 1)
);

-- Insert initial empty record
INSERT INTO dashboard_cache (id, data) VALUES (1, '{}') ON CONFLICT (id) DO NOTHING;

-- migrate:down
DROP TABLE IF EXISTS dashboard_cache;

