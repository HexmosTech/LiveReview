-- migrate:up

CREATE TABLE IF NOT EXISTS trial_eligibility (
	id BIGSERIAL PRIMARY KEY,
	normalized_email VARCHAR(255) NOT NULL UNIQUE,
	first_user_id BIGINT REFERENCES users(id),
	first_org_id BIGINT REFERENCES orgs(id) ON DELETE SET NULL,
	first_subscription_id BIGINT REFERENCES subscriptions(id) ON DELETE SET NULL,
	first_plan_code VARCHAR(64),
	reservation_token VARCHAR(128),
	reservation_expires_at TIMESTAMP WITH TIME ZONE,
	reserved_user_id BIGINT REFERENCES users(id),
	reserved_org_id BIGINT REFERENCES orgs(id) ON DELETE SET NULL,
	reserved_plan_code VARCHAR(64),
	consumed BOOLEAN NOT NULL DEFAULT FALSE,
	consumed_at TIMESTAMP WITH TIME ZONE,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	CONSTRAINT chk_trial_eligibility_email_lowercase CHECK (normalized_email = lower(normalized_email)),
	CONSTRAINT chk_trial_eligibility_consumed_window CHECK (
		(consumed = TRUE AND consumed_at IS NOT NULL) OR
		(consumed = FALSE)
	),
	CONSTRAINT chk_trial_eligibility_reservation_pair CHECK (
		(reservation_token IS NULL AND reservation_expires_at IS NULL) OR
		(reservation_token IS NOT NULL AND reservation_expires_at IS NOT NULL)
	)
);

CREATE INDEX IF NOT EXISTS idx_trial_eligibility_reservation_expires
	ON trial_eligibility(reservation_expires_at)
	WHERE reservation_expires_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_trial_eligibility_consumed
	ON trial_eligibility(consumed, consumed_at DESC);

CREATE OR REPLACE FUNCTION trial_eligibility_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
	NEW.updated_at = NOW();
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_trial_eligibility_updated_at ON trial_eligibility;
CREATE TRIGGER trg_trial_eligibility_updated_at
BEFORE UPDATE ON trial_eligibility
FOR EACH ROW
EXECUTE PROCEDURE trial_eligibility_set_updated_at();

UPDATE plan_catalog
SET trial_enabled = CASE WHEN monthly_price_usd > 0 THEN TRUE ELSE FALSE END,
	trial_days = CASE WHEN monthly_price_usd > 0 THEN 7 ELSE 0 END,
	updated_at = NOW()
WHERE plan_code IN ('team_32usd', 'loc_200k', 'loc_400k', 'loc_800k', 'loc_1600k', 'loc_3200k', 'free_30k');


-- migrate:down

UPDATE plan_catalog
SET trial_enabled = FALSE,
	trial_days = 0,
	updated_at = NOW()
WHERE plan_code IN ('team_32usd', 'loc_200k', 'loc_400k', 'loc_800k', 'loc_1600k', 'loc_3200k', 'free_30k');

DROP TRIGGER IF EXISTS trg_trial_eligibility_updated_at ON trial_eligibility;
DROP FUNCTION IF EXISTS trial_eligibility_set_updated_at();
DROP INDEX IF EXISTS idx_trial_eligibility_consumed;
DROP INDEX IF EXISTS idx_trial_eligibility_reservation_expires;
DROP TABLE IF EXISTS trial_eligibility;

