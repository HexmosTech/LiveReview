-- migrate:up

CREATE TABLE IF NOT EXISTS upgrade_replacement_cutovers (
	id BIGSERIAL PRIMARY KEY,
	upgrade_request_id VARCHAR(36) NOT NULL UNIQUE REFERENCES upgrade_requests(upgrade_request_id) ON DELETE CASCADE,
	org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
	owner_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
	old_local_subscription_id BIGINT NOT NULL REFERENCES subscriptions(id) ON DELETE RESTRICT,
	old_razorpay_subscription_id VARCHAR(255) NOT NULL,
	replacement_local_subscription_id BIGINT REFERENCES subscriptions(id) ON DELETE SET NULL,
	replacement_razorpay_subscription_id VARCHAR(255),
	target_plan_code VARCHAR(64) NOT NULL,
	target_quantity INTEGER NOT NULL,
	currency VARCHAR(16) NOT NULL,
	cutover_at TIMESTAMP WITH TIME ZONE NOT NULL,
	old_cancellation_scheduled BOOLEAN NOT NULL DEFAULT FALSE,
	status VARCHAR(64) NOT NULL DEFAULT 'pending_provisioning',
	retry_count INTEGER NOT NULL DEFAULT 0,
	next_retry_at TIMESTAMP WITH TIME ZONE,
	last_error TEXT,
	last_attempted_at TIMESTAMP WITH TIME ZONE,
	resolved_at TIMESTAMP WITH TIME ZONE,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	CONSTRAINT chk_upgrade_replacement_cutovers_status CHECK (
		status IN (
			'pending_provisioning',
			'replacement_created',
			'old_cancellation_scheduled',
			'retry_pending',
			'manual_review_required',
			'completed'
		)
	),
	CONSTRAINT chk_upgrade_replacement_cutovers_target_quantity_positive CHECK (target_quantity > 0),
	CONSTRAINT chk_upgrade_replacement_cutovers_retry_non_negative CHECK (retry_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_upgrade_replacement_cutovers_org_status
	ON upgrade_replacement_cutovers(org_id, status, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_upgrade_replacement_cutovers_next_retry
	ON upgrade_replacement_cutovers(next_retry_at)
	WHERE next_retry_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_upgrade_replacement_cutovers_cutover_at
	ON upgrade_replacement_cutovers(cutover_at, status);


-- migrate:down

DROP INDEX IF EXISTS idx_upgrade_replacement_cutovers_cutover_at;
DROP INDEX IF EXISTS idx_upgrade_replacement_cutovers_next_retry;
DROP INDEX IF EXISTS idx_upgrade_replacement_cutovers_org_status;

DROP TABLE IF EXISTS upgrade_replacement_cutovers;
