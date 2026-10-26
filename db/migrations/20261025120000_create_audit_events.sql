-- migrate:up

-- Unified, append-only audit trail for enterprise compliance.
--
-- Partitioned by month so retention is a cheap DROP PARTITION rather than a
-- mass DELETE. A DEFAULT partition is kept as a safety net: audit inserts must
-- never fail because a futuristic partition was not created yet. Monthly
-- partitions are created ahead of time by AuditMaintenanceManager.
CREATE TABLE audit_events (
    id              BIGSERIAL,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    org_id          BIGINT NOT NULL,
    actor_user_id   BIGINT NULL,
    actor_email     TEXT NULL,
    actor_name      TEXT NULL,
    actor_kind      TEXT NOT NULL DEFAULT 'system',   -- member | api_key | system | webhook | super_admin | legacy
    actor_capacity  TEXT NOT NULL DEFAULT 'member',   -- member | admin | system
    auth_method     TEXT NULL,                        -- session | api_key | none
    api_key_id      BIGINT NULL,

    action          TEXT NOT NULL,                    -- e.g. user.role_changed
    category        TEXT NOT NULL,                    -- auth | user | org | ...
    resource_type   TEXT NULL,
    resource_id     TEXT NULL,

    outcome         TEXT NOT NULL DEFAULT 'success',  -- success | failure | denied | error
    status_code     INT NULL,
    severity        TEXT NOT NULL DEFAULT 'info',     -- info | warning | critical

    ip              INET NULL,
    user_agent      TEXT NULL,
    request_id      TEXT NULL,
    method          TEXT NULL,
    path            TEXT NULL,                        -- route template, never a raw URL

    details         JSONB NOT NULL DEFAULT '{}',
    source          TEXT NOT NULL DEFAULT 'api',      -- api | webhook | system | legacy
    legacy_id       BIGINT NULL,

    -- Reserved for the per-org hash chain (enabled in phase B).
    seq             BIGINT NULL,
    prev_hash       TEXT NULL,
    hash            TEXT NULL,

    PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);

-- Safety net so an insert can never fail for lack of a monthly partition.
CREATE TABLE audit_events_default PARTITION OF audit_events DEFAULT;

-- Query indexes. Creating them on the partitioned parent propagates to every
-- existing and future partition.
CREATE INDEX idx_audit_events_id
    ON audit_events (id);
CREATE INDEX idx_audit_events_org_time
    ON audit_events (org_id, occurred_at DESC, id DESC);
CREATE INDEX idx_audit_events_org_category_time
    ON audit_events (org_id, category, occurred_at DESC);
CREATE INDEX idx_audit_events_org_actor_time
    ON audit_events (org_id, actor_user_id, occurred_at DESC);
CREATE INDEX idx_audit_events_org_action_time
    ON audit_events (org_id, action, occurred_at DESC);
CREATE INDEX idx_audit_events_org_outcome_time
    ON audit_events (org_id, outcome, occurred_at DESC);
CREATE INDEX idx_audit_events_org_severity_time
    ON audit_events (org_id, severity, occurred_at DESC);
CREATE INDEX idx_audit_events_org_seq
    ON audit_events (org_id, seq) WHERE seq IS NOT NULL;
CREATE UNIQUE INDEX uq_audit_events_legacy
    ON audit_events (source, legacy_id, occurred_at) WHERE legacy_id IS NOT NULL;

-- Append-only guard. UPDATE is never allowed. DELETE is allowed only when the
-- retention job explicitly opts in for the current transaction, so normal
-- application code (and a compromised handler) cannot rewrite history.
CREATE OR REPLACE FUNCTION audit_events_append_only() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION 'audit_events is append-only: UPDATE is not permitted';
    END IF;
    IF TG_OP = 'DELETE'
       AND COALESCE(current_setting('app.audit_purge', true), '') <> 'on' THEN
        RAISE EXCEPTION 'audit_events is append-only: DELETE is not permitted';
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER audit_events_append_only_trigger
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION audit_events_append_only();

-- One row per archived (month, org). The object lives in the configured blob
-- store; this table is the index and the tamper-evidence digest.
CREATE TABLE audit_archives (
    org_id      BIGINT NOT NULL,
    month       TEXT NOT NULL,                 -- YYYY-MM
    object_key  TEXT NOT NULL,
    row_count   BIGINT NOT NULL,
    sha256      TEXT NOT NULL,
    archived_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, month)
);

-- Runtime policy. Code also has safe defaults if this row is absent.
INSERT INTO system_settings (name, data)
VALUES ('audit_settings', '{
  "enabled": true,
  "hot_days": 90,
  "retention_days": 365,
  "archive_enabled": true,
  "anchor_enabled": true,
  "hash_chain_enabled": true,
  "cron_expression": "15 3 * * *"
}'::jsonb)
ON CONFLICT (name) DO NOTHING;

-- Import the pre-existing user-management audit trail as legacy history, so
-- the new page is populated from day one. These rows predate IP/device
-- capture, so those columns stay NULL and source='legacy' marks them.
INSERT INTO audit_events (
    occurred_at, org_id,
    actor_user_id, actor_email, actor_name, actor_kind, actor_capacity,
    action, category, resource_type, resource_id,
    outcome, severity, details, source, legacy_id
)
SELECT
    uma.created_at AT TIME ZONE 'UTC',
    uma.org_id,
    uma.performed_by_user_id,
    p.email,
    COALESCE(NULLIF(TRIM(CONCAT_WS(' ', p.first_name, p.last_name)), ''), p.email),
    CASE WHEN EXISTS (
        SELECT 1 FROM user_roles ur
        JOIN roles r ON r.id = ur.role_id
        WHERE ur.user_id = uma.performed_by_user_id AND r.name = 'super_admin'
    ) THEN 'super_admin' ELSE 'legacy' END,
    CASE WHEN EXISTS (
        SELECT 1 FROM user_roles ur
        JOIN roles r ON r.id = ur.role_id
        WHERE ur.user_id = uma.performed_by_user_id AND r.name = 'super_admin'
    ) THEN 'admin' ELSE 'member' END,
    CASE uma.action
        WHEN 'created' THEN 'user.create'
        WHEN 'updated' THEN 'user.update'
        WHEN 'deactivated' THEN 'user.deactivate'
        WHEN 'reactivated' THEN 'user.reactivate'
        WHEN 'role_changed' THEN 'user.role.changed'
        WHEN 'password_reset' THEN 'user.password.reset.force'
        WHEN 'transferred' THEN 'user.transfer'
        ELSE 'user.' || uma.action
    END,
    'user',
    'user',
    uma.target_user_id::text,
    'success',
    CASE WHEN uma.action IN ('role_changed', 'deactivated') THEN 'warning' ELSE 'info' END,
    COALESCE(uma.details, '{}'::jsonb),
    'legacy',
    uma.id
FROM user_management_audit uma
LEFT JOIN users p ON p.id = uma.performed_by_user_id;

-- migrate:down

DROP INDEX IF EXISTS uq_audit_events_legacy;
DROP INDEX IF EXISTS idx_audit_events_org_seq;
DROP INDEX IF EXISTS idx_audit_events_org_severity_time;
DROP INDEX IF EXISTS idx_audit_events_org_outcome_time;
DROP INDEX IF EXISTS idx_audit_events_org_action_time;
DROP INDEX IF EXISTS idx_audit_events_org_actor_time;
DROP INDEX IF EXISTS idx_audit_events_org_category_time;
DROP INDEX IF EXISTS idx_audit_events_org_time;
DROP INDEX IF EXISTS idx_audit_events_id;

DROP TRIGGER IF EXISTS audit_events_append_only_trigger ON audit_events;
DROP FUNCTION IF EXISTS audit_events_append_only();

DROP TABLE IF EXISTS audit_archives;
DROP TABLE IF EXISTS audit_events;

DELETE FROM system_settings WHERE name = 'audit_settings';
