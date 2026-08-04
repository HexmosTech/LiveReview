-- migrate:up

-- dashboard_cache already exists (20250805104629_dashboard_details.sql) but was
-- constrained to a single global row (id = 1) and is unused by any live code —
-- superseded at some point by the in-memory DashboardManager.cache map. This
-- repurposes it into a per-org table for precomputed dashboard widget data
-- instead of creating a second, colliding table.

ALTER TABLE dashboard_cache DROP CONSTRAINT single_dashboard_row;
ALTER TABLE dashboard_cache DROP CONSTRAINT dashboard_cache_pkey;
ALTER TABLE dashboard_cache ADD CONSTRAINT dashboard_cache_pkey PRIMARY KEY (org_id);

-- Redundant now that org_id is the primary key (Postgres already maintains a
-- unique index for the PK) — the multi-column one below stays, it's not redundant.
DROP INDEX IF EXISTS idx_dashboard_cache_org_id;

ALTER TABLE dashboard_cache ADD COLUMN json_raw JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE dashboard_cache ADD COLUMN final_json JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON TABLE dashboard_cache IS 'Precomputed per-org dashboard widget data. json_raw = one entry per calendar day (source of truth, unbounded retention). final_json = derived day/week/month/all summaries read by GET /api/v1/dashboard. Refreshed by DashboardCacheManager every 5 min. Legacy id/data/created_at columns predate this repurposing (20250805104629) and are unused by current code.';

-- migrate:down

ALTER TABLE dashboard_cache DROP COLUMN final_json;
ALTER TABLE dashboard_cache DROP COLUMN json_raw;

CREATE INDEX idx_dashboard_cache_org_id ON dashboard_cache(org_id);

ALTER TABLE dashboard_cache DROP CONSTRAINT dashboard_cache_pkey;
ALTER TABLE dashboard_cache ADD CONSTRAINT dashboard_cache_pkey PRIMARY KEY (id);
ALTER TABLE dashboard_cache ADD CONSTRAINT single_dashboard_row CHECK (id = 1);
