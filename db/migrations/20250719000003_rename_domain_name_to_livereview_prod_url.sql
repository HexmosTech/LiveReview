-- migrate:up
ALTER TABLE instance_details
RENAME COLUMN domain_name TO livereview_prod_url;

-- migrate:down
ALTER TABLE instance_details
RENAME COLUMN livereview_prod_url TO domain_name;
