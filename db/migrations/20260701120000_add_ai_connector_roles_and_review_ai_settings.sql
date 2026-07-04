-- migrate:up
ALTER TABLE ai_connectors
ADD COLUMN role VARCHAR(32) NOT NULL DEFAULT 'leader';

ALTER TABLE ai_connectors
ADD CONSTRAINT ai_connectors_role_check CHECK (role IN ('leader', 'helper'));

CREATE INDEX idx_ai_connectors_org_role_order ON ai_connectors(org_id, role, display_order);

CREATE TABLE org_review_ai_settings (
	org_id BIGINT PRIMARY KEY REFERENCES orgs(id) ON DELETE CASCADE,
	helper_enabled BOOLEAN NOT NULL DEFAULT false,
	helper_mode VARCHAR(32) NOT NULL DEFAULT 'concise_then_expand',
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	CONSTRAINT org_review_ai_settings_helper_mode_check CHECK (helper_mode IN ('concise_then_expand', 'polish_only'))
);

-- migrate:down
DROP TABLE IF EXISTS org_review_ai_settings;

DROP INDEX IF EXISTS idx_ai_connectors_org_role_order;

ALTER TABLE ai_connectors
DROP CONSTRAINT IF EXISTS ai_connectors_role_check;

ALTER TABLE ai_connectors
DROP COLUMN IF EXISTS role;