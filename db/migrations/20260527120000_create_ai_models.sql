-- migrate:up
CREATE TABLE ai_models (
    id SERIAL PRIMARY KEY,
    model_id VARCHAR(255) UNIQUE NOT NULL,
    provider VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    is_default BOOLEAN DEFAULT FALSE,
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_ai_models_provider ON ai_models(provider);

-- Trigger to auto-update updated_at
CREATE OR REPLACE FUNCTION ai_models_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ai_models_updated_at ON ai_models;
CREATE TRIGGER trg_ai_models_updated_at
BEFORE UPDATE ON ai_models
FOR EACH ROW
EXECUTE PROCEDURE ai_models_set_updated_at();

-- migrate:down
DROP TRIGGER IF EXISTS trg_ai_models_updated_at ON ai_models;
DROP FUNCTION IF EXISTS ai_models_set_updated_at();
DROP INDEX IF EXISTS idx_ai_models_provider;
DROP TABLE IF EXISTS ai_models;
