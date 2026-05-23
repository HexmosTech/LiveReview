-- migrate:up
ALTER TABLE review_feedback DROP CONSTRAINT review_feedback_source_check;
ALTER TABLE review_feedback ADD CONSTRAINT review_feedback_source_check
    CHECK (source_type IN ('comment', 'pr_level', 'slideshow', 'general'));

-- migrate:down
ALTER TABLE review_feedback DROP CONSTRAINT review_feedback_source_check;
ALTER TABLE review_feedback ADD CONSTRAINT review_feedback_source_check
    CHECK (source_type IN ('comment', 'pr_level', 'slideshow'));
