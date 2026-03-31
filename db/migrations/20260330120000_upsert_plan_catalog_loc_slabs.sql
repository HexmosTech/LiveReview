-- migrate:up

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
        'loc_200k',
        'LOC 200k',
        TRUE,
        20,
        64,
        200000,
        '["hosted_auto_model", "byok_optional", "usage_envelope_v1"]'::jsonb,
        FALSE,
        0,
        TRUE
    ),
    (
        'loc_400k',
        'LOC 400k',
        TRUE,
        30,
        128,
        400000,
        '["hosted_auto_model", "byok_optional", "usage_envelope_v1"]'::jsonb,
        FALSE,
        0,
        TRUE
    ),
    (
        'loc_800k',
        'LOC 800k',
        TRUE,
        40,
        256,
        800000,
        '["hosted_auto_model", "byok_optional", "usage_envelope_v1"]'::jsonb,
        FALSE,
        0,
        TRUE
    ),
    (
        'loc_1600k',
        'LOC 1.6M',
        TRUE,
        50,
        512,
        1600000,
        '["hosted_auto_model", "byok_optional", "usage_envelope_v1"]'::jsonb,
        FALSE,
        0,
        TRUE
    ),
    (
        'loc_3200k',
        'LOC 3.2M',
        TRUE,
        60,
        1024,
        3200000,
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

-- migrate:down

DELETE FROM plan_catalog
WHERE plan_code IN ('loc_200k', 'loc_400k', 'loc_800k', 'loc_1600k', 'loc_3200k');
