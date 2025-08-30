-- migrate:up
INSERT INTO roles (name) VALUES
('super_admin'),
('owner'),
('member');

-- migrate:down
DELETE FROM roles WHERE name IN ('super_admin', 'owner', 'member');