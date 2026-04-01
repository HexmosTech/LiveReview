-- migrate:up

CREATE TABLE IF NOT EXISTS upgrade_payment_attempts (
	id BIGSERIAL PRIMARY KEY,
	org_id BIGINT NOT NULL,
	preview_token_sha256 CHAR(64) NOT NULL,
	from_plan_code VARCHAR(64) NOT NULL,
	to_plan_code VARCHAR(64) NOT NULL,
	amount_cents BIGINT NOT NULL,
	currency VARCHAR(16) NOT NULL,
	razorpay_mode VARCHAR(16) NOT NULL,
	razorpay_order_id VARCHAR(255) NOT NULL UNIQUE,
	razorpay_payment_id VARCHAR(255),
	status VARCHAR(64) NOT NULL DEFAULT 'prepared',
	execute_idempotency_key VARCHAR(255),
	execute_response JSONB,
	error_code VARCHAR(128),
	error_reason VARCHAR(255),
	error_description TEXT,
	error_source VARCHAR(128),
	error_step VARCHAR(128),
	prepared_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	payment_failed_at TIMESTAMP WITH TIME ZONE,
	payment_captured_at TIMESTAMP WITH TIME ZONE,
	executed_at TIMESTAMP WITH TIME ZONE,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	CONSTRAINT chk_upgrade_payment_attempts_amount_non_negative CHECK (amount_cents >= 0),
	CONSTRAINT chk_upgrade_payment_attempts_status CHECK (
		status IN ('prepared', 'payment_failed', 'payment_captured', 'execute_applied')
	)
);

CREATE INDEX IF NOT EXISTS idx_upgrade_payment_attempts_org_preview
	ON upgrade_payment_attempts(org_id, preview_token_sha256);

CREATE INDEX IF NOT EXISTS idx_upgrade_payment_attempts_order
	ON upgrade_payment_attempts(razorpay_order_id);

CREATE INDEX IF NOT EXISTS idx_upgrade_payment_attempts_payment
	ON upgrade_payment_attempts(razorpay_payment_id)
	WHERE razorpay_payment_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_upgrade_payment_attempts_status
	ON upgrade_payment_attempts(status);

CREATE INDEX IF NOT EXISTS idx_upgrade_payment_attempts_execute_key
	ON upgrade_payment_attempts(execute_idempotency_key)
	WHERE execute_idempotency_key IS NOT NULL;


-- migrate:down

DROP TABLE IF EXISTS upgrade_payment_attempts CASCADE;

