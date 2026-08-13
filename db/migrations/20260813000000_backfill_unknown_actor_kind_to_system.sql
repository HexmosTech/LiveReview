-- migrate:up

-- Rows with no user_id were never triggered by a human (scheduled/webhook runs), so
-- they should read as 'system', not 'unknown' - see storage/license/loc_accounting_store.go's
-- AccountSuccess, which used to default to 'unknown' unless an actor_email happened to be
-- set. This backfills the rows written under that old, incorrect default.
UPDATE loc_usage_ledger
SET actor_kind = 'system',
    metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{actor_kind}', '"system"')
WHERE user_id IS NULL
  AND (actor_kind IS NULL OR actor_kind = 'unknown' OR metadata->>'actor_kind' = 'unknown');

-- migrate:down

-- Not reversible: the pre-fix 'unknown' value carried no information beyond
-- "no user_id", which is already recoverable from the user_id column itself.
