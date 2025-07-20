-- migrate:up

ALTER TABLE public.integration_tokens ADD COLUMN provider_url text;

-- migrate:down

ALTER TABLE public.integration_tokens DROP COLUMN provider_url;
