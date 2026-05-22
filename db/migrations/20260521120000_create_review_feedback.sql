-- migrate:up
CREATE TABLE review_feedback (
    id               BIGSERIAL PRIMARY KEY,
    org_id           BIGINT NOT NULL DEFAULT 1 REFERENCES orgs(id),
    review_id        BIGINT REFERENCES reviews(id) ON DELETE SET NULL,
    ai_comment_id    BIGINT REFERENCES ai_comments(id) ON DELETE SET NULL,
    vote_type        VARCHAR(10) NOT NULL,
    tags             TEXT[],
    feedback_text    TEXT,
    comment_content  TEXT,
    code_excerpt     TEXT,
    file_path        TEXT,
    severity         VARCHAR(50),
    source_type      VARCHAR(20) NOT NULL DEFAULT 'comment',
    lrc_version      VARCHAR(50),
    created_at       TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT review_feedback_vote_check   CHECK (vote_type   IN ('up', 'down')),
    CONSTRAINT review_feedback_source_check CHECK (source_type IN ('comment', 'pr_level', 'slideshow'))
);

CREATE INDEX idx_review_feedback_org_id     ON review_feedback(org_id);
CREATE INDEX idx_review_feedback_review_id  ON review_feedback(review_id) WHERE review_id IS NOT NULL;
CREATE INDEX idx_review_feedback_vote_type  ON review_feedback(vote_type);
CREATE INDEX idx_review_feedback_created_at ON review_feedback(created_at DESC);

-- migrate:down
DROP TABLE IF EXISTS review_feedback;
