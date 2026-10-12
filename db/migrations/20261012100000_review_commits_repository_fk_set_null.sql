-- migrate:up

-- Deleting a connector cascades to its repositories; review_commits rows blocked that.
-- Keep the review's commit history and just clear the repository link instead.
ALTER TABLE review_commits DROP CONSTRAINT review_commits_repository_id_fkey;
ALTER TABLE review_commits ADD CONSTRAINT review_commits_repository_id_fkey
    FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE SET NULL;

-- migrate:down

ALTER TABLE review_commits DROP CONSTRAINT review_commits_repository_id_fkey;
ALTER TABLE review_commits ADD CONSTRAINT review_commits_repository_id_fkey
    FOREIGN KEY (repository_id) REFERENCES repositories(id);
