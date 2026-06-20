-- migrate:up
ALTER TABLE public.reviews ADD COLUMN IF NOT EXISTS diff text;

-- migrate:down
ALTER TABLE public.reviews DROP COLUMN IF EXISTS diff;
