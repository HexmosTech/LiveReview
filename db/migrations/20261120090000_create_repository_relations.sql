-- migrate:up

CREATE TABLE repository_relations (
    id                      BIGSERIAL PRIMARY KEY,
    org_id                  BIGINT NOT NULL REFERENCES orgs(id),
    repository_id           BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    related_repository_id   BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    created_by              BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_repository_relations_pair UNIQUE (repository_id, related_repository_id),
    -- Canonical ordering (API always inserts the lower repository id first)
    -- also rules out (A,B) and (B,A) existing as separate rows, which the
    -- UNIQUE constraint alone would allow since it's not symmetric.
    CONSTRAINT ck_repository_relations_ordered CHECK (repository_id < related_repository_id)
);

CREATE INDEX idx_repository_relations_org_id             ON repository_relations(org_id);
CREATE INDEX idx_repository_relations_repository_id      ON repository_relations(repository_id);
CREATE INDEX idx_repository_relations_related_repo_id    ON repository_relations(related_repository_id);

COMMENT ON TABLE repository_relations IS 'Admin-declared "related repo" links used to build cross-repo blast-radius context (see docs/multi-repo-blast-radius-plan.md). Stored directionally (one row per declared pair) but read symmetrically by callers. Both repository_id and related_repository_id must belong to org_id - enforced in the API handler, not just by this schema, since a FK alone cannot check that both repos share the same org.';
COMMENT ON COLUMN repository_relations.created_by IS 'The user who added this relation, for display/audit context; NULL if the user was later deleted.';

-- migrate:down

DROP TABLE IF EXISTS repository_relations;
