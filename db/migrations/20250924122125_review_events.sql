-- migrate:up
CREATE TABLE IF NOT EXISTS public.review_events (
    id         bigserial PRIMARY KEY,
    review_id  bigint NOT NULL REFERENCES public.reviews(id) ON DELETE CASCADE,
    org_id     bigint NOT NULL,
    ts         timestamptz NOT NULL DEFAULT now(),
    event_type text NOT NULL,
    level      text NULL,
    batch_id   text NULL,
    data       jsonb NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_review_events_review_ts
    ON public.review_events (review_id, ts);

CREATE INDEX IF NOT EXISTS idx_review_events_org_ts
    ON public.review_events (org_id, ts);

CREATE INDEX IF NOT EXISTS idx_review_events_type
    ON public.review_events (review_id, event_type, ts DESC);

-- migrate:down
DROP INDEX IF EXISTS idx_review_events_type;
DROP INDEX IF EXISTS idx_review_events_org_ts;
DROP INDEX IF EXISTS idx_review_events_review_ts;
DROP TABLE IF EXISTS public.review_events;

