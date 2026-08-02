-- migrate:up

ALTER TABLE reviews
    ADD COLUMN pull_request_id BIGINT REFERENCES pull_requests(id) ON DELETE SET NULL;

CREATE INDEX idx_reviews_pull_request_id ON reviews(pull_request_id);

COMMENT ON COLUMN reviews.pull_request_id IS 'Canonical pull_requests row this review run belongs to, if known. Nullable for reviews triggered by raw URL or predating this column.';

-- migrate:down

DROP INDEX IF EXISTS idx_reviews_pull_request_id;
ALTER TABLE reviews DROP COLUMN IF EXISTS pull_request_id;
