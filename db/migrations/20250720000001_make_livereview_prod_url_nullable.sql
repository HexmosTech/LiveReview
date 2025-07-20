-- migrate:up
ALTER TABLE instance_details
ALTER COLUMN livereview_prod_url DROP NOT NULL;

-- migrate:down
ALTER TABLE instance_details
ALTER COLUMN livereview_prod_url SET NOT NULL;
