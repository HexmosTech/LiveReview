## LOC Pricing Rollout Dashboards and Alerts

### Scope
This document defines the minimum dashboards and alert rules required before enabling LOC quota enforcement globally.

### Dashboard 1: Envelope Coverage
- Metric: percentage of LOC-sensitive endpoints returning envelope payload.
- Breakdowns: endpoint, provider, trigger_source.
- Success target: >= 99.5% over 1 hour.
- Panels:
  - Envelope attached vs missing count.
  - 4xx/5xx responses with envelope attached.
  - Top endpoints missing envelope.

### Dashboard 2: Quota Enforcement Health
- Metric: preflight blocked decision rates.
- Breakdowns: block_reason (quota_exceeded, trial_readonly), plan_code.
- Panels:
  - Preflight checks per minute.
  - Blocked percentage.
  - Blocked operation types (manual_review, diff_review, webhook_*).

### Dashboard 3: Accounting Integrity
- Metric: success-accounted events and idempotency conflict rates.
- Panels:
  - loc_usage_ledger inserts by operation_type.
  - Idempotent conflict count by operation_type.
  - Delta between operation volume and ledger volume.
- SLO: ledger-accounted / successful-operations in [0.98, 1.02].

### Dashboard 4: Lifecycle Notifications
- Metric: threshold/reset/trial lifecycle events and email notification outcomes.
- Panels:
  - loc_lifecycle_log event counts by event_type.
  - notified_email false backlog age.
  - email send failures by provider.
- SLO: pending notification backlog age < 15 minutes.

### Dashboard 5: Plan Transition Operations
- Metric: manual upgrade/schedule/cancel/apply transition flow.
- Panels:
  - /api/v1/billing action success/failure counts.
  - Scheduled downgrade queue depth.
  - Applied transitions per hour.

## Alert Rules

### P0 Alerts
1. Envelope coverage drop
- Condition: envelope coverage < 95% for 10 minutes on protected endpoints.
- Action: page on-call backend + disable enforcement rollout flag for new cohorts.

2. Accounting mismatch critical
- Condition: ledger-accounted / successful-operations < 0.90 for 10 minutes.
- Action: page on-call backend; pause enforcement progression.

3. Quota block spike anomaly
- Condition: blocked rate > 3x 7-day baseline for 15 minutes.
- Action: page on-call and inspect plan catalog / reset jobs / traffic anomaly.

### P1 Alerts
1. Lifecycle email backlog
- Condition: unnotified lifecycle events older than 30 minutes > 50.
- Action: ticket + notify on-call in chat.

2. Plan transition job lag
- Condition: scheduled downgrades overdue by > 10 minutes.
- Action: ticket + investigate scheduler worker.

3. Billing action API failure rate
- Condition: /api/v1/billing 5xx > 2% for 15 minutes.
- Action: notify backend on-call.

## Rollout Gates
1. Gate A (internal cohorts)
- All dashboard panels active.
- No P0 alerts in last 24 hours.
- Accounting mismatch within SLO.

2. Gate B (10% orgs)
- No P0 alerts for 48 hours.
- P1 alerts acknowledged/resolved within SLA.

3. Gate C (50% orgs)
- Blocked rates stable vs expected plan mix.
- No unresolved lifecycle notification backlog.

4. Gate D (100% orgs)
- 7-day stable run with no rollback triggers.

## Operational Notes
- Keep rollout flag changes audited with actor and timestamp.
- Run synthetic diff-review calls every 5 minutes to validate envelope and accounting.
- Keep a one-click rollback to pre-enforcement mode available during rollout.
