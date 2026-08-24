-- migrate:up

-- Precomputed KPI chips for a chart (Total/Avg per period/Peak/Low/Trend for
-- a trend chart, and the analogous shape for band/heatmap/slope/category
-- charts) - see internal/chatstats.ComputeAllStats. Computed once at chart-
-- build time and stored here so the chat UI and PDF/HTML export both read
-- the same numbers instead of the frontend recomputing them from vega_spec's
-- raw row data on every day/week/month toggle click.

ALTER TABLE chat_charts ADD COLUMN stats JSONB;

-- migrate:down

ALTER TABLE chat_charts DROP COLUMN IF EXISTS stats;
