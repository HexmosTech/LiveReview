-- migrate:up

-- Seed canonical pricing catalog rows for free BYOK and paid team auto-model.
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
) VALUES
    (
        'free_30k',
        'Free 30k',
        TRUE,
        0,
        0,
        30000,
        '["basic_review", "byok_required", "usage_envelope_v1"]'::jsonb,
        FALSE,
        0,
        TRUE
    ),
    (
        'team_32usd',
        'Team 32 USD',
        TRUE,
        10,
        32,
        100000,
        '["hosted_auto_model", "byok_optional", "usage_envelope_v1"]'::jsonb,
        FALSE,
        0,
        TRUE
    )
ON CONFLICT (plan_code) DO UPDATE
SET display_name = EXCLUDED.display_name,
    active = EXCLUDED.active,
    rank = EXCLUDED.rank,
    monthly_price_usd = EXCLUDED.monthly_price_usd,
    monthly_loc_limit = EXCLUDED.monthly_loc_limit,
    feature_flags = EXCLUDED.feature_flags,
    trial_enabled = EXCLUDED.trial_enabled,
    trial_days = EXCLUDED.trial_days,
    envelope_show_price = EXCLUDED.envelope_show_price,
    updated_at = NOW();

-- Keep legacy aliases present but inactive to avoid accidental selection.
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
) VALUES
    (
        'starter_100k',
        'Legacy Starter 100k (deprecated)',
        FALSE,
        100,
        32,
        100000,
        '["legacy_alias"]'::jsonb,
        FALSE,
        0,
        FALSE
    ),
    (
        'free',
        'Legacy Free (deprecated)',
        FALSE,
        101,
        0,
        30000,
        '["legacy_alias"]'::jsonb,
        FALSE,
        0,
        FALSE
    )
ON CONFLICT (plan_code) DO UPDATE
SET active = FALSE,
    envelope_show_price = FALSE,
    updated_at = NOW();

-- migrate:down

DELETE FROM plan_catalog
WHERE plan_code IN ('free_30k', 'team_32usd', 'starter_100k', 'free');
