## Plan: LiveReview LOC Pricing Migration (Detailed 3 Phases)

Updated with your new requirements fully embedded: manual-only plan changes, mandatory starter plan, plan context in every request path, and plan+usage in every response.

### Phase 1: Product Contract and Design (UI-first, contract-first)
Goal: lock behavior and data contracts before backend coding so all downstream architecture is consistent.

1. Finalize non-negotiable business rules:
- All orgs start on 100k LOC / $32.
- Plan changes are manual-only.
- No automatic upgrade/downgrade under threshold/quota conditions.
- Upgrade is immediate with proration.
- Downgrade is next-cycle effective.
- Quota is org-month LOC only; user/repo limits removed.

2. Finalize counting and enforcement semantics:
- Billable LOC = added + deleted lines in diff sent to AI.
- Full diff billed per trigger.
- At 100%, hard-block review/comment operations requiring AI.

3. Define canonical Plan + Usage Envelope (mandatory in all relevant responses):
- Plan fields: plan id/name/price/LOC limit.
- Usage fields: used/remaining/percent/threshold/blocked.
- Billing window: period start/end/reset.
- Operation fields: trigger type, operation billable LOC, operation id, idempotency key, accounted timestamp.
- Action metadata: upgrade URL + downgrade policy hint.

4. Define response behavior contract:
- Envelope returned on success responses.
- Envelope also returned on quota/plan-related errors.
- Standardized error taxonomy for frontends/CLI.

5. Define request pipeline architecture:
- Plan Context Resolver injects plan into request context at start.
- Accounting and enforcement both read from same context.
- Response injector guarantees envelope output.

6. Define lifecycle events + notification policy:
- Events at 80%, 90%, 100%, plus period reset/start and plan-change lifecycle.
- Email notifications default ON, org opt-out available.

7. Define UI information architecture:
- LiveReview billing page: plan, usage meter, threshold markers, reset date, lifecycle timeline.
- Review/comment UX: pre/post usage visibility and block-state UX.
- git-lrc UX: concise usage summaries and actionable blocked messages.

Phase 1 acceptance criteria:
1. Contract spec finalized and agreed.
2. Error taxonomy and payload samples finalized.
3. Lifecycle policy/messaging finalized.
4. All trigger paths mapped to contract coverage.

---

### Phase 2: Backend/Billing Implementation (contract realization)
Goal: implement guaranteed contract behavior and enforcement in all execution paths.

1. Data model and migration layer:
- Add immutable per-operation usage ledger with org/team/user/review attribution.
- Add lifecycle events store.
- Add monthly rollups for fast reads.
- Add scheduled plan-change persistence (next plan + effective timestamp).
- Use dbmate migrations only.

2. Core services:
- Implement Plan Context Resolver middleware.
- Implement centralized Usage Accounting Service (deterministic LOC + idempotency).
- Implement Response Envelope Injector middleware/decorator.
- Implement standardized quota/plan error payload builder.

3. Integrate all trigger paths:
- Manual trigger.
- API diff-review (including git-lrc path).
- Webhook MR update.
- Webhook comment-response/re-review.

4. Enforcement and concurrency:
- Pre-flight quota check.
- Post-accounting guard for race prevention.
- Hard-block response includes full envelope.

5. Billing and plan transitions with Razorpay:
- Tier mapping for all LOC plans.
- Manual upgrade flow with immediate proration and effective limit change.
- Manual downgrade scheduling for next cycle only.
- Explicit prevention of automatic tier mutation.

6. Lifecycle notifications:
- Emit threshold/period/plan events.
- Email dispatch with dedup and cooldown.
- Honor org-level opt-out.

7. Operational readiness:
- Metrics for envelope coverage, accounting latency, blocked attempts, notification reliability.
- Admin diagnostics for per-operation audit lookup.

Phase 2 acceptance criteria:
1. All covered endpoints return valid envelope.
2. All trigger paths account usage exactly once.
3. Threshold events are deterministic and deduplicated.
4. Manual-only plan transition rules hold end-to-end.

---

### Phase 3: UX Rollout and Cutover (LiveReview + git-lrc)
Goal: expose the new model clearly to users and safely migrate enforcement.

1. LiveReview UI:
- Billing dashboard with manual plan picker.
- Upgrade flow with proration preview/confirmation.
- Downgrade scheduler with effective date visibility.
- Envelope-backed usage in review/comment flows.
- Lifecycle timeline and drilldowns (org/team/user/review).

2. git-lrc UX:
- Show plan+usage after operations.
- Show near-limit and hard-block messages with upgrade CTA.
- Add usage/plan inspection command.

3. Migration rollout:
- Dual-run: compute/store usage and return envelopes while old limit logic still active.
- Compare telemetry and fix parity gaps.
- Enable LOC enforcement by org cohorts.
- Promote to global.
- Remove seat/user-centric enforcement and stale UI copy.

4. Documentation and support:
- Customer docs for pricing, thresholds, manual plan changes.
- Support runbooks for billing/enforcement/notification incidents.

Phase 3 acceptance criteria:
1. LiveReview and git-lrc show consistent envelope state for same operations.
2. Staged rollout passes without critical billing/enforcement regressions.
3. Support team signs off on plan-change and quota-block handling.


---

Razorpay update

I’m treating this as a planning task for manual operator-driven LOC plan upgrades and downgrades (including Razorpay impact), and I’ll first map the existing billing and quota flow in both `LiveReview` and `git-lrc` so the plan is concrete and executable. I’ll then produce a phased implementation plan with exact files, dependencies, and verification steps.I’m running parallel discovery now: one pass to inventory existing memories, one to trace LiveReview’s Razorpay and LOC accounting internals, and one to trace git-lrc client-side surfaces that must reflect manual plan changes.

Map LiveReview billing flow

Map git-lrc quota UX surfaces

Read memory

Searched for regex `Razorpay|razorpay|RazorPay` (`LiveReview`), 20 results

Searched for regex `billing|subscription|plan|quota|pricing` (`LiveReview`), 20 results

Searched for regex `webhook|payment|invoice` (`LiveReview`), 20 results

Searched for files matching `LiveReview` · `**/*billing* -o /home/shrsv/bin/LiveReview/**/*plan* -o /home/shrsv/bin/LiveReview/**/*quota* -o /home/shrsv/bin/LiveReview/**/*razorpay*`, no matches

Semantic workspace search is not currently available

Searched for files matching `**/storage_status.md`, 2 matches

Searched for files matching `**/network_status.md`, 2 matches

Searched for files matching `**/check-status-doc-links.sh`, 2 matches

Searched for regex `plan|quota|envelope|upgrade|downgrade` (`git-lrc`), 20 results

Searched for files matching `LiveReview` · `**/*billing* -o /home/shrsv/bin/LiveReview/**/*payment*`, no matches

Searched for files matching `LiveReview` · `**/*subscription* -o /home/shrsv/bin/LiveReview/**/*quota*`, no matches

Searched for files matching `LiveReview` · `**/*admin* -o /home/shrsv/bin/LiveReview/**/*operator*`, no matches

Searched for files matching `LiveReview` · `db/migrations/*`, 20 matches

Read [](file:///home/shrsv/bin/LiveReview)

Searched for files matching `LiveReview` · `internal/**/*.go`, 20 matches

Searched for regex `usage|quota|plan|envelope` (`git-lrc` · `cmd/**`), 20 results

Searched for regex `PlanUsageEnvelope|envelope|usage|quota` (`git-lrc` · `internal/**`), 20 results

Read [](file:///home/shrsv/bin/git-lrc/internal/reviewmodel/types.go)

Read [](file:///home/shrsv/bin/git-lrc/storage/storage_status.md)

Read [](file:///home/shrsv/bin/git-lrc/network/network_status.md)

Read [](file:///home/shrsv/bin/LiveReview/storage)

Read [](file:///home/shrsv/bin/LiveReview/network)

Searched for regex `billing|plan|quota|subscription|loc` (`LiveReview` · `storage`), no results

Searched for regex `billing|plan|quota|subscription|razorpay` (`LiveReview` · `network`), no results

Searched for regex `PlanUsageEnvelope|envelope|quota|usage` (`git-lrc` · `**/*test*.go`), 20 results

Searched for regex `envelope|quota|plan|usage` (`git-lrc` · `internal/reviewapi/**`), 20 results

Read [](file:///home/shrsv/bin/git-lrc/internal/appcore/usage_inspect.go)

Read [](file:///home/shrsv/bin/git-lrc/cmd/app.go#1-1), lines 1 to 150

Read [](file:///home/shrsv/bin/LiveReview/storage/payment)

Read [](file:///home/shrsv/bin/LiveReview/network/payment)

Read [](file:///home/shrsv/bin/LiveReview/storage/storage_status.md)

Read [](file:///home/shrsv/bin/LiveReview/network/network_status.md)

Searched for regex `formatEnvelope|envelope` (`git-lrc` · `internal/appcore/**`), 20 results

Searched for files matching `git-lrc` · `internal/reviewapi/envelope_contract_test.go`, 1 match

Read [](file:///home/shrsv/bin/git-lrc/cmd/app.go#150-150), lines 150 to 200

Searched for regex `billing|payment|subscription|plan|quota|admin` (`LiveReview` · `internal/api`), no results

Searched for files matching `LiveReview` · `internal/api/*.go`, 20 matches

Searched for regex `printEnvelopeUsageSummary|formatEnvelopeUsageLines` (`git-lrc` · `internal/appcore/review_runtime.go`), 6 results

Read [](file:///home/shrsv/bin/git-lrc/internal/reviewapi/envelope_contract_test.go)

Read [](file:///home/shrsv/bin/git-lrc/internal/appcore/review_runtime.go#1540-1540), lines 1540 to 1600

Searched for regex `test.*billing|billing.*test|Test.*Billing` (`LiveReview`), 3 results

Read [](file:///home/shrsv/bin/LiveReview/internal/api/billing_actions_handler.go#1-1), lines 1 to 150

Read [](file:///home/shrsv/bin/LiveReview/internal/api/license_seats.go#1-1), lines 1 to 100

Searched for regex `PlanType|PlanStarter|Plan.*LOC|license.PlanType` (`LiveReview`), 30 results

Read [](file:///home/shrsv/bin/LiveReview/storage/license)

Read [](file:///home/shrsv/bin/LiveReview/internal/license)

Searched for regex `PollReview|SubmitReview` (`git-lrc` · `internal/reviewapi/**`), 8 results

Read [](file:///home/shrsv/bin/git-lrc/internal/appcore/review_runtime.go#1-1), lines 1 to 200

Read [](file:///home/shrsv/bin/git-lrc/internal/appcore/usage_inspect.go#75-75), lines 75 to 83

Searched for regex `Blocked|TrialReadOnly|blocked|trial|quota` (`git-lrc` · `internal/appcore/review_runtime.go`), 5 results

Read [](file:///home/shrsv/bin/git-lrc/internal/reviewapi/helpers.go#107-107), lines 107 to 240

Read [](file:///home/shrsv/bin/git-lrc/internal/reviewapi/helpers.go#161-161), lines 161 to 220

Read [](file:///home/shrsv/bin/LiveReview/internal/license/plans.go#1-1), lines 1 to 80

Read [](file:///home/shrsv/bin/LiveReview/storage/license/loc_accounting_store.go#1-1), lines 1 to 50

Read [](file:///home/shrsv/bin/LiveReview/storage/license/plan_change_store.go#1-1), lines 1 to 100

Read [](file:///home/shrsv/bin/LiveReview/internal/license/types.go)

Searched for regex `plan|quota|envelope|usage|PlanUsageEnvelope` (`git-lrc` · `tests/**`), 1 result

Read [](file:///home/shrsv/bin/git-lrc/internal/appcore/review_runtime.go#330-330), lines 330 to 370

Searched for regex `billing|plan|quota|org_billing` (`LiveReview` · `db/migrations`), no results

Read [](file:///home/shrsv/bin/LiveReview/db/migrations/20260327100200_create_loc_usage_ledger.sql)

Read [](file:///home/shrsv/bin/LiveReview/db/migrations/20251209_add_payment_tracking.sql)

Read [](file:///home/shrsv/bin/git-lrc/tests)

Read [](file:///home/shrsv/bin/git-lrc)

Searched for regex `plan|quota|Envelope|blocked` (`git-lrc` · `**/*_test.go`), 30 results

Searched for files matching `LiveReview` · `db/migrations/*org_billing* -o /home/shrsv/bin/LiveReview/db/migrations/*plan_catalog*`, no matches

Searched for text `org_billing_state` (`LiveReview` · `db/migrations`), no results

Searched for regex `plan|quota|envelope|usage` (`git-lrc` · `network/**`), 9 results

Searched for regex `plan|quota|envelope|usage` (`git-lrc` · `storage/**`), 1 result

Read [](file:///home/shrsv/bin/git-lrc/network/review_operations.go)

Read [](file:///home/shrsv/bin/LiveReview/db/migrations)

Read [](file:///home/shrsv/bin/LiveReview/db/migrations/20260327100000_create_plan_catalog.sql)

Read [](file:///home/shrsv/bin/LiveReview/db/migrations/20260327100100_create_org_billing_state.sql)

Read [](file:///home/shrsv/bin/LiveReview/db/migrations/20260327100300_create_loc_lifecycle_log.sql)

Searched for regex `usage|quota|plan` (`git-lrc` · `cmd/app.go`), 20 results

Searched for files matching `git-lrc` · `internal/appcore/usage_inspect.go`, 1 match

Searched for regex `webhook|handler.*webhook|Handler.*ebhook` (`LiveReview` · `internal/api`), no results

Searched for regex `requirePlanManager|operator|admin.*billing|billing.*admin` (`LiveReview`), 20 results

Searched for regex `submitResp|result\.Envelope|envelope\.PlanCode|envelope\.Blocked` (`git-lrc` · `internal/appcore/review_runtime.go`), 18 results

Read memory

Searching for regex `*test.go` (`LiveReview`)

Searched for files matching `LiveReview` · `**/*test.go`, 20 matches

Read [](file:///home/shrsv/bin/LiveReview/internal/api/billing_actions_handler.go#150-150), lines 150 to 250

Searched for text `test` (`LiveReview` · `internal/license`), no results

Read [](file:///home/shrsv/bin/LiveReview/internal/license/payment/integration_test.go#1-1), lines 1 to 100

Searched for regex `Warning|warn|Threshold|threshold|changed|transition|change` (`git-lrc` · `internal/appcore/review_runtime.go`), 15 results

Searched for regex `http\.StatusForbidden|http\.StatusTooManyRequests|403|429` (`git-lrc`), 20 results

Searched for text `BillingActionsHandler` (`LiveReview` · `internal/api`), no results

Read [](file:///home/shrsv/bin/LiveReview/docs/loc-pricing.md)

Searched for regex `StatusForbidden|StatusTooManyRequests|403|429` (`git-lrc` · `internal/appcore/review_runtime.go`), 2 results

Searched for text `attachBillingRoutes` (`LiveReview` · `internal/api`), no results

Searched for text `BillingActions` (`LiveReview` · `internal/api`), no results

Searched for regex `buildFakeSubmitResponse|submitReviewWithRecovery` (`git-lrc` · `internal/appcore/**`), 5 results

Read [](file:///home/shrsv/bin/git-lrc/internal/appcore/review_runtime.go#310-310), lines 310 to 365

Searched for text `NewBillingActionsHandler` (`LiveReview`), 2 results

Searched for text `routerAPI` (`LiveReview` · `internal/api`), no results

Searched for regex `plan|quota|envelope|Envelope` (`git-lrc` · `internal/appcore/auth_recovery.go`), no results

Read [](file:///home/shrsv/bin/LiveReview/internal/api/server.go#730-730), lines 730 to 760

Read [](file:///home/shrsv/bin/LiveReview/internal/api/server.go#760-760), lines 760 to 800

Read [](file:///home/shrsv/bin/LiveReview/internal/license/payment)

Searched for text `Razorpay` (`LiveReview` · `internal/license/payment`), no results

Read [](file:///home/shrsv/bin/git-lrc/internal/appcore/auth_recovery.go#1-1), lines 1 to 100

Read [](file:///home/shrsv/bin/LiveReview/internal/license/payment/payment_types.go#1-1), lines 1 to 80

Read [](file:///home/shrsv/bin/LiveReview/internal/license/payment/webhook_handler.go#1-1), lines 1 to 100

Read [](file:///home/shrsv/bin/LiveReview/internal/license/payment/webhook_handler.go#100-100), lines 100 to 250

Searched for files matching `LiveReview` · `check-status-doc-links.sh`, no matches

Searched for text `check-status-doc` (`LiveReview`), 4 results

Read [](file:///home/shrsv/bin/LiveReview/internal/license/loc_accounting.go#1-1), lines 1 to 80

Read [](file:///home/shrsv/bin/LiveReview/scripts/check-status-doc-links.sh#1-1), lines 1 to 100

Read memory

Read memory [](file:///home/shrsv/.vscode-server/data/User/workspaceStorage/ec2d7313b024a49d15f31cf5a0891be6/GitHub.copilot-chat/memory-tool/memories/MTUyMWFiMzgtYTA2OC00ODAxLTgwZTUtNWVjNTQ2MGFiMDI1/livereview-loc-accounting-exploration.md)

Searched for text `handlePayment` (`LiveReview` · `internal/license/payment`), no results

Read [](file:///home/shrsv/bin/LiveReview/internal/api/billing_actions_handler.go#150-150), lines 150 to 200

Searched for text `TODO|FIXME|gap|missing|NYI` (`LiveReview` · `internal/license/payment`), no results

Read [](file:///home/shrsv/bin/LiveReview/internal/license/scheduler.go)

Searched for text `billing_actions_handler_test` (`LiveReview`), no results

Searched for text `usage_envelope` (`LiveReview` · `internal/api`), no results

Read [](file:///home/shrsv/bin/LiveReview/docs)

Read [](file:///home/shrsv/bin/LiveReview/internal/api/usage_envelope_contract_test.go#1-1), lines 1 to 50

Read [](file:///home/shrsv/bin/LiveReview/storage/license/plan_change_store.go#100-100), lines 100 to 264

Read [](file:///home/shrsv/bin/LiveReview/internal/license/payment_facade.go)

Discovery found the client-side envelope and usage inspection surfaces clearly; next I’m pulling the detailed LiveReview billing/Razorpay exploration output and the current session plan file, then I’ll draft an operator-focused manual upgrade/downgrade plan with exact phases and verification.

Read [](file:///home/shrsv/.vscode-server/data/User/workspaceStorage/ec2d7313b024a49d15f31cf5a0891be6/GitHub.copilot-chat/chat-session-resources/1521ab38-a068-4801-80e5-5ec5460ab025/call_eQSwAXD0TJ0PoASvPim61u0u__vscode-1774615089859/content.txt)

Read memory [](file:///home/shrsv/.vscode-server/data/User/workspaceStorage/ec2d7313b024a49d15f31cf5a0891be6/GitHub.copilot-chat/memory-tool/memories/MTUyMWFiMzgtYTA2OC00ODAxLTgwZTUtNWVjNTQ2MGFiMDI1/plan.md)

I’ve now got enough codebase context to draft a precise plan; before I lock it, I want to confirm three policy choices that materially affect the implementation and rollback safety. I’m asking these now so the plan matches your exact operating model and avoids wrong assumptions about downgrade timing and Razorpay behavior.

Analyzing your answers...

I have your policy decisions now and I’m updating the persisted session plan to a focused operator-driven LOC upgrade/downgrade plan aligned to: next-cycle downgrade default, operator/superadmin-only authority, and Razorpay-first consistency. After saving, I’ll show you the full scannable plan here for approval.

Deleted memory [](file:///home/shrsv/.vscode-server/data/User/workspaceStorage/ec2d7313b024a49d15f31cf5a0891be6/GitHub.copilot-chat/memory-tool/memories/MTUyMWFiMzgtYTA2OC00ODAxLTgwZTUtNWVjNTQ2MGFiMDI1/plan.md)

Created memory file [](file:///home/shrsv/.vscode-server/data/User/workspaceStorage/ec2d7313b024a49d15f31cf5a0891be6/GitHub.copilot-chat/memory-tool/memories/MTUyMWFiMzgtYTA2OC00ODAxLTgwZTUtNWVjNTQ2MGFiMDI1/plan.md)

## Plan: Operator-Driven LOC Plan Transitions

Build a safe manual upgrade/downgrade workflow for operators with three hard rules already aligned to your choices: downgrade defaults to next billing cycle, only platform operator/superadmin can run cross-org actions, and Razorpay is source-of-truth before internal state changes.

**Steps**
1. Phase 1: Policy and invariants.
2. Add explicit transition modes and reason codes in backend transition requests and lifecycle events: immediate_upgrade, schedule_downgrade, cancel_scheduled_downgrade, force_immediate_downgrade.
3. Enforce invariants in service flow: no internal plan mutation if Razorpay update fails, idempotency key mandatory for operator actions, immediate upgrade auto-cancels any scheduled downgrade.
4. Phase 2: Operator-only API surface (depends on 1-3).
5. Add operator endpoints for cross-org plan operations with strict operator/superadmin auth boundary and normalized responses with before_plan, after_plan, effective_at, razorpay_sync_status, operation_id, envelope.
6. Keep existing org owner/admin paths separate and non-privileged for cross-org actions.
7. Phase 3: Scheduled downgrade execution (depends on 5-6).
8. Wire scheduled downgrade runner at service startup and process due transitions at fixed interval with idempotent execution.
9. For each due downgrade: call Razorpay first, then commit internal plan state and emit lifecycle event only on Razorpay success.
10. Add retry + dead-letter status for scheduler failures so operators can re-run failed transitions safely.
11. Phase 4: Razorpay reconciliation and observability (parallel after API contract is stable).
12. Add reconciliation job to detect internal-vs-Razorpay plan drift and expose operator-readable drift status.
13. Add structured logs and metrics: transition_started, transition_succeeded, transition_failed, reconciliation_drift_detected.
14. Add audit timeline endpoint for operator actions plus webhook outcomes.
15. Phase 5: Envelope and client behavior (parallel with 12-14).
16. Ensure envelope always reflects active plan, scheduled plan, effective timestamps, blocked/trial flags after each transition-related response.
17. In CLI, show informational plan-change deltas when plan_code changes between submit and poll responses.
18. Keep backend fully authoritative; CLI remains display-only.
19. Phase 6: Testing and rollout (depends on all phases).
20. Add API auth tests for operator-only access and negative cases.
21. Add integration tests for upgrade success/failure rollback, scheduled downgrade execution once-only, and retry idempotency.
22. Add webhook/reconciliation tests for ordering races and proration-related state sync.
23. Roll out in gates: logging-only mode, pilot operator cohort, staged production enablement, explicit rollback switch for operator endpoints and scheduler loop.

**Relevant files**
- LiveReview/internal/api/billing_actions_handler.go - transition orchestration and scheduler flow
- LiveReview/internal/api/server.go - operator route registration and scheduler startup
- LiveReview/storage/license/plan_change_store.go - transition persistence and due-transition apply logic
- LiveReview/storage/payment/subscription_store.go - subscription sync metadata and updates
- LiveReview/internal/license/payment/webhook_handler.go - webhook reconciliation behavior
- LiveReview/internal/license/plans.go - allowed transition target plans
- LiveReview/internal/api/usage_envelope_contract_test.go - envelope transition contract coverage
- LiveReview/storage/storage_status.md - required storage status updates
- LiveReview/network/network_status.md - required network status updates
- LiveReview/scripts/check-status-doc-links.sh - status-doc line-link validation
- git-lrc/internal/appcore/review_runtime.go - display plan-change deltas
- git-lrc/internal/appcore/usage_inspect.go - show active vs scheduled plan details
- git-lrc/internal/reviewapi/envelope_contract_test.go - parse and assert transition envelope fields

**Verification**
1. Operator endpoint tests prove operator/superadmin-only access and block org-admin cross-org plan mutation.
2. Integration tests prove Razorpay-first behavior: failed Razorpay update leaves internal state unchanged.
3. Scheduler tests prove next-cycle downgrade applies exactly once and remains idempotent on retries/restarts.
4. Reconciliation tests prove drift detection when webhook/order timing diverges.
5. Envelope tests prove transition metadata consistency in success and error responses.
6. CLI tests prove transition messaging appears on envelope deltas without changing backend decisions.
7. Status docs pass link checks after each storage/network change via LiveReview/scripts/check-status-doc-links.sh.

**Decisions captured**
- Next-cycle downgrade is default.
- Cross-org manual transitions are operator/superadmin only.
- Razorpay-first consistency is mandatory.
- Scope includes manual LOC plan lifecycle and auditability.
- Scope excludes customer self-serve billing UX redesign and non-Razorpay providers.

If you approve, I can refine this into an execution checklist with phase-by-phase issue breakdown (tickets + acceptance criteria) next.