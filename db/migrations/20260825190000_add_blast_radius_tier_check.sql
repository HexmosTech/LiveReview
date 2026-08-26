-- migrate:up

-- Livi's SQL generator guessed 'critical'/'high' as tier values (echoing
-- the user's own question wording) instead of the real
-- blast-radius-{none,low,medium,high} strings (internal/blastradius.Tier),
-- producing a WHERE clause that silently matched zero rows - see
-- chat_debug_logs/chat_debug.log. A CHECK constraint puts the exact valid
-- literals directly into the schema (dbctx surfaces CHECK definitions to
-- the model), which a free-text COMMENT ON COLUMN evidently isn't forceful
-- enough to override on its own.

ALTER TABLE blast_radius_hunks
    ADD CONSTRAINT blast_radius_hunks_tier_check
    CHECK (tier IN ('blast-radius-none', 'blast-radius-low', 'blast-radius-medium', 'blast-radius-high'));

-- migrate:down

ALTER TABLE blast_radius_hunks DROP CONSTRAINT IF EXISTS blast_radius_hunks_tier_check;
