-- migrate:up

CREATE TABLE IF NOT EXISTS upgrade_requests (
	id BIGSERIAL PRIMARY KEY,
	upgrade_request_id VARCHAR(36) NOT NULL UNIQUE,
	org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
	actor_user_id BIGINT NOT NULL REFERENCES users(id),
	from_plan_code VARCHAR(64) NOT NULL,
	to_plan_code VARCHAR(64) NOT NULL,
	expected_amount_cents BIGINT NOT NULL,
	currency VARCHAR(16) NOT NULL,
	preview_token_sha256 CHAR(64) NOT NULL,
	razorpay_mode VARCHAR(16),
	razorpay_order_id VARCHAR(255),
	razorpay_payment_id VARCHAR(255),
	local_subscription_id BIGINT REFERENCES subscriptions(id),
	razorpay_subscription_id VARCHAR(255),
	target_quantity INTEGER,
	payment_capture_confirmed BOOLEAN NOT NULL DEFAULT FALSE,
	payment_capture_confirmed_at TIMESTAMP WITH TIME ZONE,
	subscription_change_confirmed BOOLEAN NOT NULL DEFAULT FALSE,
	subscription_change_confirmed_at TIMESTAMP WITH TIME ZONE,
	plan_grant_applied BOOLEAN NOT NULL DEFAULT FALSE,
	plan_grant_applied_at TIMESTAMP WITH TIME ZONE,
	current_status VARCHAR(64) NOT NULL DEFAULT 'created',
	failure_reason TEXT,
	resolved_at TIMESTAMP WITH TIME ZONE,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	CONSTRAINT chk_upgrade_requests_amount_non_negative CHECK (expected_amount_cents >= 0),
	CONSTRAINT chk_upgrade_requests_status CHECK (
		current_status IN (
			'created',
			'payment_order_created',
			'waiting_for_capture',
			'payment_capture_confirmed',
			'subscription_update_requested',
			'waiting_for_subscription_confirm',
			'subscription_change_confirmed',
			'reconciliation_retrying',
			'manual_review_required',
			'resolved',
			'failed'
		)
	)
);

CREATE INDEX IF NOT EXISTS idx_upgrade_requests_org_created
	ON upgrade_requests(org_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_upgrade_requests_org_status
	ON upgrade_requests(org_id, current_status, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_upgrade_requests_pending_apply
	ON upgrade_requests(current_status, plan_grant_applied, updated_at)
	WHERE current_status = 'resolved' AND plan_grant_applied = FALSE;

CREATE INDEX IF NOT EXISTS idx_upgrade_requests_order
	ON upgrade_requests(razorpay_order_id)
	WHERE razorpay_order_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_upgrade_requests_payment
	ON upgrade_requests(razorpay_payment_id)
	WHERE razorpay_payment_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_upgrade_requests_subscription
	ON upgrade_requests(razorpay_subscription_id)
	WHERE razorpay_subscription_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS upgrade_request_events (
	id BIGSERIAL PRIMARY KEY,
	upgrade_request_id VARCHAR(36) NOT NULL REFERENCES upgrade_requests(upgrade_request_id) ON DELETE CASCADE,
	org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
	event_source VARCHAR(64) NOT NULL,
	event_type VARCHAR(64) NOT NULL,
	from_status VARCHAR(64),
	to_status VARCHAR(64),
	event_payload JSONB,
	event_time TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_upgrade_request_events_request_time
	ON upgrade_request_events(upgrade_request_id, event_time DESC);

CREATE INDEX IF NOT EXISTS idx_upgrade_request_events_org_time
	ON upgrade_request_events(org_id, event_time DESC);

ALTER TABLE upgrade_payment_attempts
	ADD COLUMN IF NOT EXISTS upgrade_request_id VARCHAR(36);

UPDATE upgrade_payment_attempts AS attempts
SET upgrade_request_id = requests.upgrade_request_id
FROM upgrade_requests AS requests
WHERE attempts.upgrade_request_id IS NULL
	AND attempts.org_id = requests.org_id
	AND attempts.preview_token_sha256 = requests.preview_token_sha256
	AND attempts.razorpay_order_id = requests.razorpay_order_id;

ALTER TABLE upgrade_payment_attempts
	ADD CONSTRAINT fk_upgrade_payment_attempts_upgrade_request
	FOREIGN KEY (upgrade_request_id)
	REFERENCES upgrade_requests(upgrade_request_id)
	ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_upgrade_payment_attempts_request
	ON upgrade_payment_attempts(upgrade_request_id)
	WHERE upgrade_request_id IS NOT NULL;


-- migrate:down

DROP INDEX IF EXISTS idx_upgrade_payment_attempts_request;

ALTER TABLE upgrade_payment_attempts
	DROP CONSTRAINT IF EXISTS fk_upgrade_payment_attempts_upgrade_request;

ALTER TABLE upgrade_payment_attempts
	DROP COLUMN IF EXISTS upgrade_request_id;

DROP TABLE IF EXISTS upgrade_request_events;

DROP TABLE IF EXISTS upgrade_requests;

