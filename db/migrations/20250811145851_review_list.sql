-- migrate:up
CREATE TABLE reviews (
    id BIGSERIAL PRIMARY KEY,
    repository VARCHAR(255) NOT NULL,
    branch VARCHAR(255),
    commit_hash VARCHAR(255),
    pr_mr_url TEXT,
    connector_id BIGINT,
    status VARCHAR(50) NOT NULL DEFAULT 'created',
    trigger_type VARCHAR(50) NOT NULL DEFAULT 'manual',
    user_email VARCHAR(255),
    provider VARCHAR(100),
    
    -- Timing fields to track AI efficiency
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    
    -- Additional metadata in JSON format for flexibility
    metadata JSONB DEFAULT '{}'::jsonb,
    
    -- Indexes for common queries
    CONSTRAINT reviews_status_check CHECK (status IN ('created', 'in_progress', 'completed', 'failed'))
);

-- Create indexes for efficient querying
CREATE INDEX idx_reviews_status ON reviews(status);
CREATE INDEX idx_reviews_created_at ON reviews(created_at DESC);
CREATE INDEX idx_reviews_connector_id ON reviews(connector_id);
CREATE INDEX idx_reviews_repository ON reviews(repository);
CREATE INDEX idx_reviews_provider ON reviews(provider);

-- Create AI comments table (depends on reviews table)
CREATE TABLE ai_comments (
    id BIGSERIAL PRIMARY KEY,
    review_id BIGINT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    comment_type VARCHAR(50) NOT NULL,
    content JSONB NOT NULL,
    
    -- For line comments - file path and line number
    file_path TEXT,
    line_number INTEGER,
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT ai_comments_type_check CHECK (comment_type IN ('summary', 'line_comment', 'suggestion', 'general', 'file_comment'))
);

-- Create indexes for AI comments
CREATE INDEX idx_ai_comments_review_id ON ai_comments(review_id);
CREATE INDEX idx_ai_comments_type ON ai_comments(comment_type);
CREATE INDEX idx_ai_comments_created_at ON ai_comments(created_at DESC);
CREATE INDEX idx_ai_comments_file_path ON ai_comments(file_path) WHERE file_path IS NOT NULL;

-- Add review_id to recent_activity table for better tracking
ALTER TABLE recent_activity ADD COLUMN review_id BIGINT REFERENCES reviews(id) ON DELETE SET NULL;
CREATE INDEX idx_recent_activity_review_id ON recent_activity(review_id);

-- migrate:down
-- Remove review_id column from recent_activity
DROP INDEX IF EXISTS idx_recent_activity_review_id;
ALTER TABLE recent_activity DROP COLUMN IF EXISTS review_id;

-- Drop the ai_comments table
DROP TABLE IF EXISTS ai_comments;

-- Drop the reviews table
DROP TABLE IF EXISTS reviews;

