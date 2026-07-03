-- migrate:up
-- System-managed helper connectors (see subscription_service.go's
-- ConfirmPurchase) need a lite-tier default resolvable via
-- aidefault.ResolveConnectorOptions, mirroring the existing 'default'
-- (Gemini 2.5 Flash) tier used for leader connectors.
INSERT INTO system_default_ai_configs (tier_name, provider_name, model_name, master_api_key)
VALUES ('default_lite', 'gemini', 'gemini-2.5-flash-lite', '')
ON CONFLICT (tier_name) DO NOTHING;

-- migrate:down
DELETE FROM system_default_ai_configs WHERE tier_name = 'default_lite';
