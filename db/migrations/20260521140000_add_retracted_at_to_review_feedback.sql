-- migrate:up
ALTER TABLE review_feedback ADD COLUMN retracted_at TIMESTAMP WITH TIME ZONE;

-- migrate:down
ALTER TABLE review_feedback DROP COLUMN IF EXISTS retracted_at;
