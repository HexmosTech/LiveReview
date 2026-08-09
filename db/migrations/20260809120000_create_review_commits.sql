-- migrate:up

CREATE TABLE review_commits (
    id            BIGSERIAL PRIMARY KEY,
    review_id     BIGINT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    org_id        BIGINT NOT NULL REFERENCES orgs(id),
    repository_id BIGINT REFERENCES repositories(id),
    ref           VARCHAR(160) NOT NULL,
    ref_type      TEXT NOT NULL DEFAULT 'commit' CHECK (ref_type IN ('commit', 'range')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_review_commits_review_ref UNIQUE (review_id, ref)
);

CREATE INDEX idx_review_commits_org_ref ON review_commits (org_id, ref);
CREATE INDEX idx_review_commits_review_id ON review_commits (review_id);

COMMENT ON TABLE review_commits IS 'Commit identifiers covered by a given review run. A review may cover zero (staged/working diff), one, or many commits. When a --range review is submitted, both the expanded individual commit SHAs and the literal range expression are stored, so later lookups match whichever identifier form the caller supplies. Matched for CI lookups by exact (org_id, ref) string equality only -- no ancestry computation.';
COMMENT ON COLUMN review_commits.ref IS 'A full commit SHA, or a literal "<from>..<to>" range expression as submitted by the caller.';

-- migrate:down

DROP TABLE review_commits;
