-- migrate:up

-- Align org billing state with canonical plan codes.
-- Paying orgs (active subscription) are moved to team_32usd, non-paying orgs to free_30k.

UPDATE org_billing_state obs
SET current_plan_code = CASE
        WHEN EXISTS (
            SELECT 1
            FROM subscriptions s
            WHERE s.org_id = obs.org_id
              AND s.status = 'active'
        ) THEN 'team_32usd'
        ELSE 'free_30k'
    END,
    scheduled_plan_code = CASE
        WHEN obs.scheduled_plan_code IN ('starter_100k', 'team') THEN 'team_32usd'
        WHEN obs.scheduled_plan_code = 'free' THEN 'free_30k'
        ELSE obs.scheduled_plan_code
    END,
    updated_at = NOW()
WHERE obs.current_plan_code IN ('starter_100k', 'team', 'free');

-- migrate:down

-- No-op down migration: this data migration is intentionally irreversible.
SELECT 1;
