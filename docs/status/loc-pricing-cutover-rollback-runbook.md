## LOC Pricing Cutover Rollback Runbook

### Purpose
Provide a deterministic rollback procedure when LOC pricing enforcement causes user-visible regressions, accounting drift, or quota decision anomalies.

### Rollback Triggers
Start rollback immediately if any condition is true:
1. P0 alert from envelope coverage or accounting integrity.
2. Sustained false-positive blocks on paid orgs.
3. Billing action API failures prevent plan transitions.
4. On-call cannot remediate within 15 minutes.

### Preconditions
- On-call backend and incident commander assigned.
- Access to feature flags and deployment controls.
- SQL shell access for verification queries.

### Fast Rollback (Target < 5 minutes)
1. Freeze rollout progression
- Stop any automation that increases cohort percentage.

2. Disable enforcement flag
- Set enforcement mode from hard-block to observe-only.
- Keep envelope response enabled to preserve telemetry continuity.

3. Pause scheduler jobs touching plan transitions
- Pause downgrade apply worker if transition behavior is involved.
- Keep reset scheduler enabled unless it is root cause.

4. Announce status
- Post incident update in engineering channel with ETA and impact.

### Data Integrity Checks After Fast Rollback
Run these checks to confirm system is safe:
1. Preflight behavior
- Verify endpoints return allowance info without hard block.

2. Accounting continuity
- Verify ledger writes still occur for successful operations.

3. Lifecycle pipeline
- Verify lifecycle logs continue and notification backlog is not growing unexpectedly.

### Deep Rollback (If Required)
Use only if fast rollback is insufficient.
1. Revert to last known good LiveReview binary.
2. Revert to previous plan-catalog config version.
3. Re-enable only minimal billing read endpoints while write endpoints are stabilized.

### Communication Template
- Incident: LOC pricing rollback initiated.
- Impact: quota enforcement temporarily disabled; usage visibility retained.
- User effect: no hard quota blocking during rollback window.
- Next update: in 15 minutes.

### Recovery Criteria Before Re-Cutover
All criteria must pass before retrying cutover:
1. No P0 alerts for 24 hours.
2. Accounting SLO within [0.98, 1.02] for 24 hours.
3. Synthetic and canary org checks pass for manual, diff-review, and webhook flows.
4. Incident action items for root cause are completed.

### Ownership
- Incident Commander: decides rollback start/stop.
- Backend On-call: executes flag and worker actions.
- SRE/Platform: verifies dashboards and alert clear state.
- Product Owner: approves re-cutover window.
