-- migrate:up

CREATE TABLE repositories (
    id                  BIGSERIAL PRIMARY KEY,
    org_id              BIGINT NOT NULL REFERENCES orgs(id),
    connector_id        BIGINT NOT NULL REFERENCES integration_tokens(id) ON DELETE CASCADE,
    provider            TEXT NOT NULL,               -- 'github' | 'gitlab'
    provider_repo_id    TEXT NOT NULL,                -- stable external id as string
    full_name           TEXT NOT NULL,                -- 'owner/repo' or 'group/subgroup/project'
    name                TEXT NOT NULL,
    web_url             TEXT NOT NULL,
    clone_url           TEXT,
    ssh_url             TEXT,
    default_branch      TEXT,
    is_private          BOOLEAN NOT NULL DEFAULT true,
    description         TEXT,
    last_synced_at      TIMESTAMPTZ,
    last_sync_status    TEXT NOT NULL DEFAULT 'pending' CHECK (last_sync_status IN ('pending', 'ok', 'error')),
    last_sync_error     TEXT,
    metadata            JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_repositories_connector_provider_repo UNIQUE (connector_id, provider_repo_id)
);

CREATE INDEX idx_repositories_org_id         ON repositories(org_id);
CREATE INDEX idx_repositories_connector_id   ON repositories(connector_id);
CREATE INDEX idx_repositories_org_provider   ON repositories(org_id, provider);
CREATE INDEX idx_repositories_last_synced_at ON repositories(last_synced_at);

COMMENT ON TABLE repositories IS 'Canonical repository per connector, discovered from a provider (GitHub/GitLab) and kept in sync via webhook + periodic reconciliation.';
COMMENT ON COLUMN repositories.provider_repo_id IS 'Stable external repository id as reported by the provider API (string form).';
COMMENT ON COLUMN repositories.last_sync_status IS 'Outcome of the most recent sync attempt for this repository.';

CREATE TABLE pull_requests (
    id                   BIGSERIAL PRIMARY KEY,
    repository_id        BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    org_id               BIGINT NOT NULL REFERENCES orgs(id),
    provider             TEXT NOT NULL,
    provider_pr_id       TEXT NOT NULL,               -- stable external id
    number               INTEGER NOT NULL,            -- human-facing #123 / !45
    title                TEXT NOT NULL DEFAULT '',
    description          TEXT,
    state                TEXT NOT NULL CHECK (state IN ('open', 'closed', 'merged')),
    author_id            TEXT,
    author_username      TEXT,
    author_name          TEXT,
    author_avatar_url    TEXT,
    source_branch        TEXT,
    target_branch        TEXT,
    web_url              TEXT NOT NULL,
    provider_created_at  TIMESTAMPTZ,
    provider_updated_at  TIMESTAMPTZ,
    last_synced_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_synced_source   TEXT NOT NULL DEFAULT 'poll' CHECK (last_synced_source IN ('webhook', 'poll', 'backfill')),
    metadata             JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_pull_requests_repo_provider_pr UNIQUE (repository_id, provider_pr_id)
);

CREATE UNIQUE INDEX uq_pull_requests_repo_number  ON pull_requests(repository_id, number);
CREATE INDEX idx_pull_requests_org_id             ON pull_requests(org_id);
CREATE INDEX idx_pull_requests_repository_state   ON pull_requests(repository_id, state);
CREATE INDEX idx_pull_requests_org_state          ON pull_requests(org_id, state);
CREATE INDEX idx_pull_requests_last_synced_at     ON pull_requests(last_synced_at);

COMMENT ON TABLE pull_requests IS 'Canonical pull/merge request per repository, normalized across providers and kept in sync via webhook + periodic reconciliation.';
COMMENT ON COLUMN pull_requests.state IS 'Normalized state: open | closed | merged. Mapping is provider-specific (see internal/prsync).';
COMMENT ON COLUMN pull_requests.last_synced_source IS 'How last_synced_at was set: webhook | poll | backfill.';

-- migrate:down

DROP TABLE IF EXISTS pull_requests;
DROP TABLE IF EXISTS repositories;
