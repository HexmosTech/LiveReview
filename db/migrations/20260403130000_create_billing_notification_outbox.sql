-- migrate:up

CREATE TABLE IF NOT EXISTS billing_notification_outbox (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    event_type VARCHAR(80) NOT NULL,
    channel VARCHAR(24) NOT NULL,
    dedupe_key VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    recipient_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    recipient_email VARCHAR(320),
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    retry_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    send_after TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_billing_notification_outbox_dedupe UNIQUE (channel, dedupe_key),
    CONSTRAINT chk_billing_notification_outbox_channel CHECK (channel IN ('in_app', 'email')),
    CONSTRAINT chk_billing_notification_outbox_status CHECK (status IN ('pending', 'processing', 'sent', 'failed', 'cancelled'))
);

CREATE INDEX IF NOT EXISTS idx_billing_notification_outbox_pending
    ON billing_notification_outbox(status, send_after, created_at)
    WHERE status IN ('pending', 'failed');

CREATE INDEX IF NOT EXISTS idx_billing_notification_outbox_org_created
    ON billing_notification_outbox(org_id, created_at DESC);

-- migrate:down

DROP INDEX IF EXISTS idx_billing_notification_outbox_org_created;
DROP INDEX IF EXISTS idx_billing_notification_outbox_pending;
DROP TABLE IF EXISTS billing_notification_outbox;
