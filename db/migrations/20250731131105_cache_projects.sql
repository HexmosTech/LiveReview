-- migrate:up
ALTER TABLE public.integration_tokens
ADD COLUMN projects_cache jsonb;



-- migrate:down

ALTER TABLE public.integration_tokens
DROP COLUMN projects_cache;
