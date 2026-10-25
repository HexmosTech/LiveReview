-- migrate:up

-- Normalize historical rows created before the super-admin visibility rule was
-- introduced. Every super-admin action is visible only to super admins; org
-- owners see their org and all member actions. This is a one-time metadata
-- correction, so the append-only trigger is disabled for the duration of this
-- transactional migration and re-enabled immediately afterwards.
--
-- This takes an ACCESS EXCLUSIVE lock on the audit table/partitions. Run it in
-- a maintenance window on a busy production instance.
ALTER TABLE audit_events DISABLE TRIGGER audit_events_append_only_trigger;

UPDATE audit_events ae
SET actor_capacity = 'admin',
    actor_kind = CASE
        WHEN ae.actor_kind = 'member' THEN 'super_admin'
        ELSE ae.actor_kind
    END
WHERE ae.actor_capacity <> 'admin'
  AND ae.actor_kind NOT IN ('system', 'webhook')
  AND ae.actor_user_id IN (
      SELECT ur.user_id
      FROM user_roles ur
      JOIN roles r ON r.id = ur.role_id
      WHERE r.name = 'super_admin'
  );

ALTER TABLE audit_events ENABLE TRIGGER audit_events_append_only_trigger;

-- migrate:down

-- No-op by design. Reverting would re-expose support actions to customers.
