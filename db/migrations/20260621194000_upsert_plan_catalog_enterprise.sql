-- migrate:up

ALTER TABLE plan_catalog
DROP CONSTRAINT chk_plan_catalog_loc_non_negative;

ALTER TABLE plan_catalog
ADD CONSTRAINT chk_plan_catalog_loc_non_negative
CHECK (monthly_loc_limit >= 0 OR monthly_loc_limit = -1);

INSERT INTO plan_catalog (
    plan_code,
    display_name,
    active,
    rank,
    monthly_price_usd,
    monthly_loc_limit,
    feature_flags,
    trial_enabled,
    trial_days,
    envelope_show_price
)
VALUES (
    'enterprise-selfhosted',
    'Enterprise - Self Hosted',
    TRUE,
    999,
    0,
    -1,
    '["unlimited_reviews"]'::jsonb,
    FALSE,
    0,
    FALSE
)
ON CONFLICT (plan_code) DO UPDATE
SET
    display_name = EXCLUDED.display_name,
    active = EXCLUDED.active,
    rank = EXCLUDED.rank,
    monthly_price_usd = EXCLUDED.monthly_price_usd,
    monthly_loc_limit = EXCLUDED.monthly_loc_limit,
    feature_flags = EXCLUDED.feature_flags,
    trial_enabled = EXCLUDED.trial_enabled,
    trial_days = EXCLUDED.trial_days,
    envelope_show_price = EXCLUDED.envelope_show_price,
    updated_at = NOW();


-- migrate:down

DELETE FROM plan_catalog
WHERE plan_code = 'enterprise-selfhosted';

ALTER TABLE plan_catalog
DROP CONSTRAINT chk_plan_catalog_loc_non_negative;

ALTER TABLE plan_catalog
ADD CONSTRAINT chk_plan_catalog_loc_non_negative
CHECK (monthly_loc_limit >= 0);