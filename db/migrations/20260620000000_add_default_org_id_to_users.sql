-- migrate:up
ALTER TABLE users ADD COLUMN default_org_id BIGINT REFERENCES orgs(id);

UPDATE users u
SET default_org_id = (
    SELECT org_id 
    FROM user_roles ur 
    WHERE ur.user_id = u.id 
    ORDER BY created_at ASC 
    LIMIT 1
);

-- migrate:down
ALTER TABLE users DROP COLUMN default_org_id;
