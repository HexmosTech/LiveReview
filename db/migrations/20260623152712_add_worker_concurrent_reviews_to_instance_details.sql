-- migrate:up
ALTER TABLE instance_details ADD COLUMN worker_concurrent_reviews integer DEFAULT 10 NOT NULL;

-- migrate:down
ALTER TABLE instance_details DROP COLUMN worker_concurrent_reviews;
