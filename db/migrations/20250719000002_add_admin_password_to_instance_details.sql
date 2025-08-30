-- migrate:up
ALTER TABLE instance_details
ADD COLUMN admin_password TEXT NOT NULL;

-- migrate:down
ALTER TABLE instance_details
DROP COLUMN admin_password;
