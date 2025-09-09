# LiveReview Licence Enforcement & Management – Execution Checklist

This checklist converts the specification in `licence-enforcement-and-management.md` into concrete, incremental phases. Each task references target files and the nature of changes. Execute phases in order. Do not skip verification gates.

Legend:
- (N) new file
- (M) modify existing file
- (OPT) optional / future

## Phase 0 – Preparation & Baseline (COMPLETED)
| Goal | Ensure workspace, env vars, and baseline build are healthy |

Tasks:
1. [x] (M) `go.mod` – added / confirmed deps (currently indirect):
	- github.com/golang-jwt/jwt/v5
	- github.com/denisbrodbeck/machineid
	- github.com/shirou/gopsutil/v3
2. [x] Public key endpoint reachable: curl https://parse.apps.hexmos.com/jwtLicence/publicKey → JSON (kid=v1)
3. [x] (N) `internal/license/config.go` – hard-coded constants:
	- api_base=https://parse.apps.hexmos.com timeout=60s grace_days=3 validation_interval=24h include_hardware_id=true enforcement=soft
4. [ ] Optional enforcement toggling (DEFERRED) – intentionally not implemented.

Note: Licence subsystem intentionally non-configurable at runtime; changes require code edit + rebuild.

Verification Gate (Phase 0):
 - [x] Build command (go build livereview.go) succeeded.
 - [x] Public key endpoint returned JSON (kid=v1).
 - [x] Workspace clean of unintended changes.

---
## Phase 1 – Database Schema & Storage Skeleton
| Goal | Persist licence state (single row) |

Tasks:
1. [x] (N) Migration added `db/migrations/20250909120000_create_license_state.sql` (singleton table + indexes + trigger).
2. [x] Migration applied (dbmate up executed successfully; table confirmed by test).
3. [x] (N) `internal/license/storage.go` with GetLicenseState / UpsertLicenseState / UpdateValidationResult.
4. [x] (N) `internal/license/types.go` with LicenseState + status constants.
5. [x] DB driver already present (`github.com/lib/pq`).

Verification Gate (Phase 1):
 - [x] Migration applied (dbmate up) & table exists.
 - [x] Storage CRUD smoke test (TestStorage) passed.
 - [x] Build succeeds after adding storage/types.

---
## Phase 2 – Core Licence Service & Offline Validation
| Goal | Implement core logic for storing, parsing, offline verifying JWT |

Tasks:
1. [x] (N) `internal/license/validator.go` – offline + public key fetch + online validate.
2. [x] (N) `internal/license/hardware.go` – basic fingerprint (goos/goarch/host hash).
3. [x] (N) `internal/license/service.go` – LoadOrInit, EnterLicense, PerformOnlineValidation.
4. [x] (N) `internal/license/errors.go` – custom errors + NetworkError type.
5. [x] (M) `go.mod` – jwt dependency already present (indirect acceptable for now).
6. [x] (M) `Makefile` – added `license-test` target.
7. [ ] (N) `internal/license/service_test.go` – (Deferred) targeted tests for validator/service (TODO Phase 11 hardening).

Verification Gate (Phase 2 interim):
 - [x] Build (go build livereview.go) passes.
 - [x] Existing storage test still green.
 - [ ] Dedicated service tests (deferred; add mocks in later phase).

---
## Phase 3 – API Endpoints
| Goal | Expose licence status & mutation endpoints |

Tasks:
1. [x] (N) `internal/api/license.go` – implemented status/update/refresh handlers.
2. [x] (M) `internal/api/server.go` – route registration via attachLicenseRoutes.
3. [x] (N) `internal/api/license_dto.go` – DTO struct.
4. [ ] Logging for transitions (to be added in Phase 10 observability).
5. [ ] Integration test `tests/license_api_test.go` (deferred – will add when service tests expanded).

Verification Gate (Phase 3 final):
 - [x] Build succeeds with new endpoints.
 - [x] Manual curl smoke executed (status/update/refresh) – negative path validated.
 - [x] Invalid token returns 400 with JSON error code `license_invalid`.
 - [x] Basic integration test added (`internal/license/integration_test.go`).

---
## Phase 4 – Scheduler & Grace Logic (COMPLETED)
| Goal | Daily validation + network-only grace period enforcement |

Tasks:
1. [x] (N) `internal/license/scheduler.go` – ticker loop + initial 5s delay.
2. [x] (M) `internal/license/service.go` – escalation logic updated (grace after 3rd network failure) + grace expiry helper.
3. [x] (M) `internal/license/types.go` – added `ComputeDaysRemaining()`.
4. [x] (M) `internal/api/server.go` – scheduler start/stop wiring.
5. [x] (N) `internal/license/scheduler_test.go` – grace expiry test.

Verification Gate (Phase 4):
 - [x] Build passes with scheduler.
 - [x] Grace expiry test passes (skipped if DB not configured).
 - [ ] Manual runtime observation of periodic validation (pending Phase 10 logging).
 - [ ] Manual simulated network outage escalation (pending).

---
## Phase 5 – Frontend State & API Client (COMPLETED)
| Goal | Frontend can read/display licence status & update token |

Tasks:
1. [x] (N) `ui/src/api/license.ts` – Functions: `getLicenseStatus()`, `updateLicense()`, `refreshLicense()`.
2. [x] (N) `ui/src/store/License/slice.ts` – Redux slice with async thunks.
3. [x] (M) `ui/src/store/rootReducer.ts` – Register licence slice.
4. [x] (M) `ui/src/App.tsx` – Dispatch initial `fetchLicenseStatus` on mount.
5. [x] (N) `ui/src/store/License/types.ts` – Type definitions & initial state.
6. [x] (N) `ui/src/__tests__/licenseSlice.test.ts` – Reducer state transition tests.

Verification Gate (Phase 5):
 - [x] Unit tests for slice pass.
 - [x] App triggers initial load (dispatch added in App.tsx).
 - [ ] Manual devtools inspection (pending manual runtime check).

---
## Phase 6 – Licence Entry Modal (Blocking Mode)
| Goal | Collect initial token when missing; allow enforcement / gating |

Tasks:
1. [x] (N) `ui/src/components/License/LicenseModal.tsx` – Controlled component with strict blocking (no close in required states).
2. [x] (M) `ui/src/App.tsx` – Enforced auto-open & blocking for `missing|invalid|expired` (currently always enforced; config-based soft/strict toggle still TODO Phase 9).
3. [ ] (M) Slice – (Deferred) central modal visibility actions `openModal` / `closeModal` (local component state used instead for now). 
4. [x] (N) `ui/src/components/License/LicenseModal.test.tsx` – Basic interaction test (dispatch on save) added.
5. [ ] (OPT) Accessibility: focus trap, aria-modal, escape handling (to add when polishing UI phases 7–9).

Verification Gate:
- [x] Remove/reset licence state → modal appears automatically (blocking) until valid token saved.
- [ ] Enter invalid token → inline error displayed (manual confirmation pending; component logic present via `lastError`).
- [x] Enter valid token → modal closes & status bar reflects new status.

Notes:
- Current implementation enforces blocking regardless of an external "enforcement=soft" flag; introduction of a runtime/build-time toggle remains scheduled for Phase 9.
- Modal state lives in `App.tsx`; migrate to slice only if multiple disparate triggers required.

---
## Phase 7 – Status Bar Component (COMPLETED)
| Goal | Persistent unobtrusive visual indicator |

Tasks:
1. [x] (N) `ui/src/components/License/LicenseStatusBar.tsx` – Maps status→color/label; shows days remaining & actions.
2. [x] (M) `App.tsx` – Integrated `<LicenseStatusBar />` replacing ad-hoc bar.
3. [x] Styling leveraged existing Tailwind palette (no extra config needed).
4. [x] (N) `ui/src/components/License/LicenseStatusBar.test.tsx` – Basic interaction tests (open modal + refresh dispatch).
5. [ ] (OPT) Add warning/grace color simulation test & snapshot (deferred to Phase 11 hardening).

Verification Gate:
- [x] Bar visible across authenticated routes.
- [ ] Manual simulation of warning/grace/expired (pending ability to force statuses easily; will add helper in Phase 10 debug endpoint or test harness).

---
## Phase 8 – Settings Licence Tab (IN PROGRESS)
| Goal | Detail view & manual refresh / re-entry (restricted to super_admin & owner) |

Tasks:
1. [x] (N) `ui/src/pages/Settings/LicenseTab.tsx` – Metadata (status, subject, seats, expiry, validation info) + refresh & replace buttons.
2. [x] (M) `ui/src/pages/Settings/Settings.tsx` – Conditional tab injection only if role ∈ {super_admin, owner}.
3. [ ] (M) Replace token UX: reuse central modal instead of window.prompt (deferred – will lift modal control to slice or context).
4. [x] (N) `ui/src/pages/Settings/LicenseTab.test.tsx` – Role-based visibility tests (member denied, owner allowed).
5. [ ] (OPT) Add super_admin visibility test + refresh action dispatch assertion.
6. [ ] (OPT) Display days remaining & color-coded status badges consistent with StatusBar.

Verification Gate:
- [x] Tab hidden for non-privileged roles (member) & visible for owner.
- [ ] Manual refresh network call observed (pending manual test / mock dispatch assertion addition).
- [ ] Replace token via improved modal flow (pending Task 3).

---
## Phase 9 – Blocking Overlay & Hard Enforcement
| Goal | Prevent app usage when licence invalid (strict mode) |

Tasks:
1. (N) `ui/src/components/LicenseBlockingScreen.tsx` – Full-screen overlay (explanation + token update CTA).
2. (M) `App.tsx` – Pre-route guard: if `status=expired|invalid` and mode=strict → block.
3. (M) Backend: ensure `status` mapping (ACTIVE→valid, others → map) is consistent.
4. (M) Add config `LIVEREVIEW_LICENSE_ENFORCEMENT` consumption in frontend (expose via server-rendered config endpoint or build-time env `VITE_...`).

Verification Gate:
- Toggle enforcement mode soft/strict – observe behavior difference.
- Expired token scenario blocks navigation.

---
## Phase 10 – Logging, Metrics & Diagnostics
| Goal | Observability for licence behaviour |

Tasks:
1. (M) `internal/license/service.go` – Structured log lines for transitions: `license.status.change old= active new= grace reason= network_failures=3`.
2. (M) Add counters (if metrics infra exists) e.g., Prometheus: `license_validation_success_total`, `license_validation_failure_total{reason}`.
3. (N) `internal/license/debug.go` (OPT) – Expose `/api/v1/license/debug` (protected) returning raw state.
4. (M) Frontend – Console warn when entering grace.

Verification Gate:
- Trigger transitions; confirm logs & metrics increments.
- Query metrics endpoint (if available) for counters.

---
## Phase 11 – Comprehensive Testing & Hardening
| Goal | Ensure reliability before full rollout |

Tasks:
1. Backend unit coverage ≥ key functions: `validator.go`, `service.go`, `scheduler.go`.
2. Add test: simulate 3 consecutive network failures → grace; 4th after grace days → expired.
3. Add test: licence error immediate block (e.g., TOKEN_EXPIRED response).
4. Frontend Cypress/Playwright (if available): modal flow, status bar, blocking overlay.
5. Security review: ensure tokens never logged (grep codebase for `license_token`).
6. Load test validate endpoint usage (ensure we do not spam fw-parse — confirm interval logic).

Verification Gate:
- `go test ./...` green.
- Frontend tests green.
- Manual offline test: disconnect network, restart app → offline still runs (if last validation <24h) else warning.

---
## Phase 12 – Rollout & Post-Deployment
| Goal | Controlled activation & monitoring |

Tasks:
1. Deploy with `LIVEREVIEW_LICENSE_ENFORCEMENT=soft` for 48h; collect metrics.
2. Add dashboard alert: validation failure ratio > 20% over 1h.
3. Switch to `strict` after confidence window.
4. Document operational runbook: `docs/runbook-license.md` (incident procedures).

Verification Gate:
- No spike in user errors after activation.
- Support can retrieve status quickly via API.

---
## Appendix A – Status Transition Matrix (Implementation Aid)
| From | Event | To | Notes |
|------|-------|----|------|
| active | network fail (n=1) | warning | store failure count |
| warning | network fail (n=3) | grace | start grace_period_start |
| grace | network fail & grace days exceeded | expired | hard block |
| active| licence error | expired | immediate |
| warning/grace | online success | active | reset counters |

## Appendix B – Manual Spot Check Script (Local)
```bash
# 1. Start backend
livereview api --port 8888 &

# 2. Query status (expect missing / invalid early)
curl -s localhost:8888/api/v1/license/status | jq

# 3. Insert token
curl -s -X POST localhost:8888/api/v1/license/update -d '{"token":"<LICENSE_JWT>"}' -H 'Content-Type: application/json' | jq

# 4. Force refresh
curl -s -X POST localhost:8888/api/v1/license/refresh | jq

# 5. Simulate network failure
export LIVEREVIEW_LICENSE_API_BASE=http://127.0.0.1:59999/parse
curl -s -X POST localhost:8888/api/v1/license/refresh | jq

# 6. Restore
export LIVEREVIEW_LICENSE_API_BASE=https://parse.apps.hexmos.com/parse
```

## Appendix C – Risk Mitigations
- Network outages: grace logic + offline JWT ensures continuity.
- Key rotation (future): design caches kid; add support when multi-key endpoint appears.
- Token leakage: not written to frontend persistent storage.
- Abuse of validate endpoint: scheduler ensures at most 1 per day unless forced by expiry.

## Appendix D – Deferred / Optional Items
- Hardware binding enforcement (server-side) — requires fw-parse enhancement.
- Multi-tenant multi-licence model (current spec = singleton licence per instance).
- Admin UI listing of historical validations.

---
End of checklist.

