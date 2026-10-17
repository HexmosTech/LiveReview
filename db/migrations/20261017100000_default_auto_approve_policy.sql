-- migrate:up

-- Auto-approve is on by default: every org gets an org-level policy with the column defaults.
INSERT INTO auto_approve_policies (org_id)
SELECT o.id FROM orgs o
WHERE NOT EXISTS (
    SELECT 1 FROM auto_approve_policies p WHERE p.org_id = o.id AND p.repository_id IS NULL
);

CREATE FUNCTION create_default_auto_approve_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    INSERT INTO auto_approve_policies (org_id) VALUES (NEW.id);
    RETURN NEW;
END;
$$;

CREATE TRIGGER orgs_default_auto_approve_policy
    AFTER INSERT ON orgs
    FOR EACH ROW EXECUTE FUNCTION create_default_auto_approve_policy();

-- migrate:down

DROP TRIGGER IF EXISTS orgs_default_auto_approve_policy ON orgs;
DROP FUNCTION IF EXISTS create_default_auto_approve_policy();
