-- migrate:up

-- Deleting a connector cascades to its repositories; repo override policies blocked that.
-- A repo's override policy is meaningless without the repo, so delete it too.
ALTER TABLE auto_approve_policies DROP CONSTRAINT auto_approve_policies_repository_id_fkey;
ALTER TABLE auto_approve_policies ADD CONSTRAINT auto_approve_policies_repository_id_fkey
    FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE;

-- migrate:down

ALTER TABLE auto_approve_policies DROP CONSTRAINT auto_approve_policies_repository_id_fkey;
ALTER TABLE auto_approve_policies ADD CONSTRAINT auto_approve_policies_repository_id_fkey
    FOREIGN KEY (repository_id) REFERENCES repositories(id);
