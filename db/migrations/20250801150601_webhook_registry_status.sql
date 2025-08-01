-- migrate:up
ALTER TABLE webhook_registry ALTER COLUMN status DROP DEFAULT;



-- migrate:down
ALTER TABLE webhook_registry ALTER COLUMN status SET DEFAULT 'active';
