-- migrate:up
-- Prompt application context: targeting scope for variables/chunks
CREATE TABLE IF NOT EXISTS prompt_application_context (
	id                      BIGSERIAL   PRIMARY KEY,
	org_id                  BIGINT      NOT NULL REFERENCES public.orgs(id),
	-- Optional specific targeting (NULL means wildcard "*")
	ai_connector_id         INTEGER              REFERENCES public.ai_connectors(id),
	integration_token_id    BIGINT               REFERENCES public.integration_tokens(id), -- git connector
	group_identifier        TEXT,                -- e.g., GitLab group path, GitHub org/owner; NULL means wildcard
	repository              TEXT,                -- provider-specific repo identifier; NULL means wildcard
	created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pac_org ON prompt_application_context (org_id);
CREATE INDEX IF NOT EXISTS idx_pac_targeting ON prompt_application_context (org_id, ai_connector_id, integration_token_id, group_identifier, repository);

-- Chunks: user or system; ordered within a variable for a given application context
CREATE TABLE IF NOT EXISTS prompt_chunks (
	id                      BIGSERIAL   PRIMARY KEY,
	org_id                  BIGINT      NOT NULL REFERENCES public.orgs(id), -- owner org (redundant but useful for RLS)
	application_context_id  BIGINT      NOT NULL REFERENCES prompt_application_context(id) ON DELETE CASCADE,
	prompt_key              TEXT        NOT NULL,     -- which template this chunk is for
	variable_name           TEXT        NOT NULL,     -- which variable this chunk contributes to
	chunk_type              TEXT        NOT NULL,     -- 'system' | 'user'
	title                   TEXT,                    -- optional descriptive title
	body                    TEXT        NOT NULL,     -- plaintext by default
	sequence_index          INTEGER     NOT NULL DEFAULT 1000, -- order within (prompt_key, variable_name, application_context)
	enabled                 BOOLEAN     NOT NULL DEFAULT TRUE,
	allow_markdown          BOOLEAN     NOT NULL DEFAULT TRUE,
	redact_on_log           BOOLEAN     NOT NULL DEFAULT FALSE,
	created_by              BIGINT,
	updated_by              BIGINT,
	created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE (application_context_id, prompt_key, variable_name, sequence_index)
);

CREATE INDEX IF NOT EXISTS idx_chunks_prompt_var ON prompt_chunks (prompt_key, variable_name);
CREATE INDEX IF NOT EXISTS idx_chunks_appctx ON prompt_chunks (application_context_id);

-- Seed: default application context row per org (org-only; other fields NULL)
INSERT INTO prompt_application_context (org_id)
SELECT id FROM public.orgs;

-- migrate:down
DROP INDEX IF EXISTS idx_chunks_appctx;
DROP INDEX IF EXISTS idx_chunks_prompt_var;
DROP TABLE IF EXISTS prompt_chunks;

DROP INDEX IF EXISTS idx_pac_targeting;
DROP INDEX IF EXISTS idx_pac_org;
DROP TABLE IF EXISTS prompt_application_context;
