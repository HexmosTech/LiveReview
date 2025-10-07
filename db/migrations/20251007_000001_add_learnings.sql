-- migrate:up
CREATE TYPE learning_status AS ENUM ('active','archived');
CREATE TYPE learning_scope AS ENUM ('org','repo');

CREATE TABLE learnings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id TEXT NOT NULL UNIQUE,
    org_id BIGINT NOT NULL,
    scope_kind learning_scope NOT NULL,
    repo_id TEXT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    tags TEXT[] NOT NULL DEFAULT '{}',
    status learning_status NOT NULL DEFAULT 'active',
    confidence INT NOT NULL DEFAULT 1,
    simhash BIGINT NOT NULL,
    embedding BYTEA NULL,
    tsv tsvector GENERATED ALWAYS AS (to_tsvector('simple', coalesce(title,'') || ' ' || coalesce(body,''))) STORED,
    source_urls TEXT[] NOT NULL DEFAULT '{}',
    source_context JSONB NULL,
    created_by BIGINT NULL,
    updated_by BIGINT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_learnings_org_simhash ON learnings (org_id, simhash);
CREATE INDEX idx_learnings_tags ON learnings USING GIN (tags);
CREATE INDEX idx_learnings_tsv ON learnings USING GIN (tsv);
CREATE INDEX idx_learnings_active ON learnings (org_id) WHERE status = 'active';

CREATE TABLE learning_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    learning_id UUID NOT NULL REFERENCES learnings(id) ON DELETE CASCADE,
    org_id BIGINT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('add','update','delete','restore')),
    provider TEXT NOT NULL,
    thread_id TEXT NULL,
    comment_id TEXT NULL,
    repository TEXT NULL,
    commit_sha TEXT NULL,
    file_path TEXT NULL,
    line_start INT NULL,
    line_end INT NULL,
    actor_id BIGINT NULL,
    reason_snippet TEXT NULL,
    classifier TEXT NULL,
    context JSONB NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_learning_events_org_created ON learning_events (org_id, created_at DESC);
CREATE INDEX idx_learning_events_learning ON learning_events (learning_id, created_at DESC);

-- migrate:down
DROP TABLE IF EXISTS learning_events;
DROP INDEX IF EXISTS idx_learnings_active;
DROP INDEX IF EXISTS idx_learnings_tsv;
DROP INDEX IF EXISTS idx_learnings_tags;
DROP INDEX IF EXISTS idx_learnings_org_simhash;
DROP TABLE IF EXISTS learnings;
DROP TYPE IF EXISTS learning_scope;
DROP TYPE IF EXISTS learning_status;
