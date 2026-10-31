-- migrate:up

-- One row per browser/device a user has granted push permission on (a user
-- has no single device, so this is 1:many, not a column on users).
CREATE TABLE IF NOT EXISTS push_subscriptions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint TEXT NOT NULL UNIQUE,
    p256dh TEXT NOT NULL,
    auth_key TEXT NOT NULL,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_push_subscriptions_user_id ON push_subscriptions(user_id);

-- A missing row means "both notification types enabled" (the agreed default),
-- so there is no backfill to do for existing users.
CREATE TABLE IF NOT EXISTS user_notification_preferences (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    notify_first_finding BOOLEAN NOT NULL DEFAULT TRUE,
    notify_completion BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Claim-once marker so concurrent batch-completion goroutines (see
-- internal/batch/task_queue.go's worker pool) can't double-send the
-- "first finding available" push for the same review.
ALTER TABLE reviews ADD COLUMN IF NOT EXISTS first_finding_notified_at TIMESTAMPTZ;

-- migrate:down
ALTER TABLE reviews DROP COLUMN IF EXISTS first_finding_notified_at;
DROP TABLE IF EXISTS user_notification_preferences;
DROP TABLE IF EXISTS push_subscriptions;
