-- migrate:up
CREATE TABLE IF NOT EXISTS public.org_tools (
    org_id      bigint NOT NULL REFERENCES public.orgs(id) ON DELETE CASCADE,
    tool_id     bigint NOT NULL REFERENCES public.available_tools(id) ON DELETE CASCADE,
    enabled     boolean NOT NULL DEFAULT false,
    config_json jsonb   NOT NULL DEFAULT '{}',
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, tool_id)
);

CREATE INDEX IF NOT EXISTS idx_org_tools_org_id ON public.org_tools (org_id);

-- migrate:down
DROP INDEX  IF EXISTS idx_org_tools_org_id;
DROP TABLE  IF EXISTS public.org_tools;
