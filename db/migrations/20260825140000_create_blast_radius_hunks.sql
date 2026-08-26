-- migrate:up

-- Per-hunk blast radius scoring, ported from git-lrc's blast-radius.json
-- artifact (previously opaque, S3-only - internal/api/diff_review.go's
-- PutDiffReviewArtifact/GetDiffReviewArtifact). combined + math_mode are
-- computed once at artifact-upload time by internal/blastradius, so Livi's
-- chat can query blast radius via plain SQL instead of it living only in a
-- JSON blob no SQL generator can see. org_id is denormalized (not just
-- reachable via review_id -> reviews.org_id) per AGENTS.md's "Direct Context
-- Filtering" rule - every query filters org_id straight from the
-- authenticated request context, no join required.
--
-- One row per hunk, not per review: a review's overall risk is
-- MAX(combined) across its hunks (see the blast_radius_reviews view below),
-- but hunk-level granularity is what the diff viewer's per-hunk risk badges
-- need, and is cheap to keep.

CREATE TABLE blast_radius_hunks (
    id         BIGSERIAL PRIMARY KEY,
    review_id  BIGINT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    org_id     BIGINT NOT NULL REFERENCES orgs(id),
    file_path  TEXT NOT NULL,
    new_start  INTEGER NOT NULL,
    new_lines  INTEGER NOT NULL,
    combined   NUMERIC(5,2) NOT NULL,
    tier       VARCHAR(32) NOT NULL,
    math_mode  JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_blast_radius_hunks_review_hunk UNIQUE (review_id, file_path, new_start, new_lines)
);

CREATE INDEX idx_blast_radius_hunks_review_id ON blast_radius_hunks (review_id);
CREATE INDEX idx_blast_radius_hunks_org_combined ON blast_radius_hunks (org_id, combined DESC);

COMMENT ON TABLE blast_radius_hunks IS 'Per-hunk blast radius scores, computed server-side from git-lrc''s S3 artifact at upload time. combined is the 0-100 score (see internal/blastradius.ComputeMathMode); math_mode is the full step-by-step derivation (signal-level detail included) that powers the diff viewer''s Math Mode tab without recomputing anything client-side.';
COMMENT ON COLUMN blast_radius_hunks.tier IS 'One of blast-radius-{none,low,medium,high} - see internal/blastradius.Tier. Thresholds: >=66 high, >=33 medium, >0 low, else none.';

-- One row per review (MAX(combined) across its hunks), for the common case
-- Livi actually needs to answer ("which reviews had high blast radius this
-- month") without every generated query having to know to GROUP BY/MAX
-- itself - a flat SELECT against this view is a much more reliable shape
-- for an LLM's SQL generator than an ad-hoc subquery.
CREATE VIEW blast_radius_reviews AS
SELECT
    review_id,
    org_id,
    MAX(combined) AS combined,
    (ARRAY_AGG(tier ORDER BY combined DESC))[1] AS tier
FROM blast_radius_hunks
GROUP BY review_id, org_id;

COMMENT ON VIEW blast_radius_reviews IS 'One row per review: its highest-scoring hunk''s combined score and tier. The shape Livi should query for "how many reviews had high blast radius" style questions.';

-- migrate:down

DROP VIEW IF EXISTS blast_radius_reviews;
DROP TABLE IF EXISTS blast_radius_hunks;
