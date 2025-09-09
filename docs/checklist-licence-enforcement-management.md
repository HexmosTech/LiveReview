# LiveReview Licence Enforcement & Management – Execution Checklist

This checklist converts the specification in `licence-enforcement-and-management.md` into concrete, incremental phases. Each task references target files and the nature of changes. Execute phases in order. Do not skip verification gates.

Legend:
- (N) new file
- (M) modify existing file
- (OPT) optional / future

## Phase 0 – Preparation & Baseline
| Goal | Ensure workspace, env vars, and baseline build are healthy |

Tasks:
1. (M) `livereview.toml` – Add `[license]` section (if not already):
	```toml
	[license]
	api_base = "${LIVEREVIEW_LICENSE_API_BASE}"
	timeout = "30s"
	grace_days = 3
	validation_interval = "24h"
	include_hardware_id = true
	skip_validation = false
	```
2. (M) `go.mod` – Add required dependencies (will be used in Phase 2):
	- `github.com/golang-jwt/jwt/v5`
	- `github.com/denisbrodbeck/machineid`
	- `github.com/shirou/gopsutil/v3`
3. (M) Deployment manifests / Dockerfile(s) – ensure env var passthrough:
	- `LIVEREVIEW_LICENSE_API_BASE=https://parse.apps.hexmos.com/parse`
	- `LIVEREVIEW_LICENSE_GRACE_DAYS=3`
	- `LIVEREVIEW_LICENSE_VALIDATION_INTERVAL=24h`
	- `LIVEREVIEW_LICENSE_INCLUDE_HARDWARE_ID=true`
4. Confirm fw-parse instance reachable: `curl -sS https://parse.apps.hexmos.com/parse/jwtLicence/publicKey | jq` (expect `data.kid`).
5. Create feature flag env for staged rollout: `LIVEREVIEW_LICENSE_ENFORCEMENT=soft` (values: `off|soft|strict`).

Verification Gate:
- `go build ./...` succeeds.
- Public key endpoint returns JSON.
- No uncommitted accidental changes (git clean).

---
## Phase 1 – Database Schema & Storage Skeleton
| Goal | Persist licence state (single row) |

Tasks:
1. (N) `internal/license/migrations/001_create_license_state.sql` – Create table per spec (include indices):
	```sql
	CREATE TABLE IF NOT EXISTS license_state (...spec columns...);
	CREATE UNIQUE INDEX IF NOT EXISTS ux_license_state_singleton ON license_state(id);
	CREATE INDEX IF NOT EXISTS idx_license_state_expires_at ON license_state(expires_at);
	```
2. (M) `internal/api/database.go` (or equivalent DB init) – Ensure migration runner loads new SQL file(s). If no migration system exists, add minimal loader to execute SQL at startup.
3. (N) `internal/license/storage.go` – Functions:
	- `GetLicenseState() (*LicenseState, error)` (return singleton row or nil)
	- `UpsertLicenseState(state *LicenseState) error`
	- `UpdateValidationResult(success bool, errCode *string)`
4. (N) `internal/license/types.go` – Define `LicenseState`, `ValidationFailure`, enumerated statuses.
5. (M) `go.mod` – If DB driver not present (SQLite / Postgres), confirm imports (already in project; reuse existing).

Verification Gate:
- Run migration locally (manual `sqlite3` / `psql` or start app) – table exists.
- `go test ./internal/license -run TestStorage` (write a minimal test if none exists).
- Inserting and retrieving dummy row works (add temporary test code or REPL test).

---
## Phase 2 – Core Licence Service & Offline Validation
| Goal | Implement core logic for storing, parsing, offline verifying JWT |

Tasks:
1. (N) `internal/license/validator.go` – Implement:
	- `ValidateOfflineJWT(token, publicKey, kid)` (per spec)
	- `FetchPublicKey(ctx)` – GET `${api_base}/jwtLicence/publicKey`
	- `ValidateOnline(ctx, token, dryRun bool)` – POST validate endpoint
2. (N) `internal/license/hardware.go` – Implement hardware fingerprint (hash of machineid + host + cpu) – behind config flag.
3. (N) `internal/license/service.go` – Methods:
	- `LoadOrInit() (*LicenseState, error)`
	- `EnterLicense(token string) (*LicenseState, error)` (decode offline, store metadata)
	- `PerformOnlineValidation(force bool)` – orchestrates offline first then online if needed
4. (N) `internal/license/errors.go` – Centralize custom errors (e.g., `ErrLicenseMissing`, `ErrLicenseExpired`).
5. (M) `go.mod` – Add external libs (run `go get`).
6. (M) `Makefile` – Add target `license-test` running focused tests.
7. (N) `internal/license/service_test.go` – Unit tests for offline validation (valid & tampered token cases – can craft with public key stub or mark with TODO if key not available in test env).

Verification Gate:
- `go build ./...` passes.
- Unit tests pass: `go test ./internal/license -count=1`.
- Simulate offline check with saved public key file (mock HTTP client).

---
## Phase 3 – API Endpoints
| Goal | Expose licence status & mutation endpoints |

Tasks:
1. (N) `internal/api/license.go` – Handlers:
	- `GET /api/v1/license/status` – returns current state (mask token)
	- `POST /api/v1/license/update { token }`
	- `POST /api/v1/license/refresh` – force online validation
2. (M) `internal/api/server.go` (or router assembly file) – Register new routes.
3. (N) `internal/api/license_dto.go` – Response structs (keep API boundary clean).
4. (M) `internal/api/handlers/` (if existing pattern) – follow conventions (Echo / mux adapter).
5. (M) Add logging (status transitions) via existing logging package.
6. (N) Basic integration test: `tests/license_api_test.go` – spin up test server, call endpoints with mock service.

Verification Gate:
- Curl checks:
  - `curl -s localhost:8888/api/v1/license/status`
  - `curl -X POST localhost:8888/api/v1/license/update -d '{"token":"<fake>"}'`
- Proper JSON error on invalid token.
- No panics in logs.

---
## Phase 4 – Scheduler & Grace Logic
| Goal | Daily validation + network-only grace period enforcement |

Tasks:
1. (N) `internal/license/scheduler.go` – Implement ticker based on config `validation_interval`.
2. (M) `service.go` – Add `ApplyValidationOutcome(result, err)` updating counters & status transitions:
	- Network error → increment `validation_failures`, possibly enter `warning/grace`.
	- Licence error → set status `expired` immediately.
3. (M) `types.go` – Add derived helper: `ComputeDaysRemaining()`.
4. (M) `cmd/api.go` – Start scheduler after server init.
5. (N) `internal/license/scheduler_test.go` – Time-compressed test (override interval via dependency injection).

Verification Gate:
- Adjust interval to `5s` in dev; observe logs for periodic validation.
- Simulate network outage (e.g., change base URL) – status transitions to warning/grace as expected.
- Restore URL – status resets to active.

---
## Phase 5 – Frontend State & API Client
| Goal | Frontend can read/display licence status & update token |

Tasks:
1. (N) `ui/src/api/license.ts` – Functions: `getStatus()`, `updateLicense(token)`, `refreshLicense()`.
2. (N) `ui/src/store/license/slice.ts` – Redux slice with async thunks.
3. (M) `ui/src/store/index.ts` – Register licence slice.
4. (M) `ui/src/App.tsx` – On mount: dispatch `license/initLoad` (do not block UI yet if mode = soft).
5. (N) `ui/src/types/license.ts` – Shared TypeScript interfaces (align with backend DTO).
6. (N) `ui/src/__tests__/licenseSlice.test.ts` – Basic store tests (mock fetch).

Verification Gate:
- `npm test -- licenseSlice` passes.
- Devtools shows populated licence state after startup call.

---
## Phase 6 – Licence Entry Modal (Blocking Mode)
| Goal | Collect initial token when missing; allow enforcement gating |

Tasks:
1. (N) `ui/src/components/LicenseModal.tsx` – Controlled component.
2. (M) `App.tsx` – Conditional render: if `enforcement=strict && status===missing|invalid|expired` show modal overlay.
3. (M) Slice – Add actions: `openModal`, `closeModal`, `submitToken`.
4. (N) `ui/src/__tests__/LicenseModal.test.tsx` – Interaction tests (enter token dispatches update thunk).

Verification Gate:
- Remove db row (or reset state) → modal appears.
- Enter invalid token → inline error.
- Enter valid token → modal closes & status bar updates.

---
## Phase 7 – Status Bar Component
| Goal | Persistent unobtrusive visual indicator |

Tasks:
1. (N) `ui/src/components/LicenseStatusBar.tsx` – Map status → color + text.
2. (M) `App.tsx` (layout wrapper) – Insert `<LicenseStatusBar />` above global notifications.
3. (M) Tailwind / CSS additions if required.
4. (N) `ui/src/__tests__/LicenseStatusBar.test.tsx` – Snapshot & state mapping tests.

Verification Gate:
- Status bar visible in all main routes.
- Status changes (simulate warning/grace) reflect color/text updates.

---
## Phase 8 – Settings Licence Tab
| Goal | Detail view & manual refresh / re-entry |

Tasks:
1. (N) `ui/src/pages/Settings/LicenseTab.tsx` – Display full metadata.
2. (M) `ui/src/pages/Settings/Settings.tsx` – Add tab descriptor `{ id:'license', name:'License' }`.
3. (M) Add actions: manual refresh, replace token.
4. (N) `ui/src/__tests__/LicenseTab.test.tsx` – Render & mock API interactions.

Verification Gate:
- Tab only visible to authorized users (decide scope – likely all logged-in?).
- Manual refresh triggers backend call (inspect network panel).

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

