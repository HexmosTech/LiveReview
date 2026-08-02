-- migrate:up
CREATE TABLE IF NOT EXISTS public.available_tools (
    id          bigserial PRIMARY KEY,
    name        text NOT NULL UNIQUE,
    description text NOT NULL,
    lambda_arn  text NOT NULL,
    multiplier  numeric(6,2) NOT NULL DEFAULT 1.0,
    use_case    text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- Tools are registered via the lr-tools deployer's `register-tools` command,
-- which resolves real Lambda ARNs from AWS after deployment and calls the
-- LiveReview API (POST /api/v1/admin/tools) to upsert each tool.
-- No hardcoded ARNs belong here.

-- migrate:down
DROP TABLE IF EXISTS public.available_tools;
