# Local E2E testing (real accounts, isolated lrc)

This harness runs LiveReview's audit/review flows against a real local
deployment using real roles, while keeping your personal `~/.lrc.toml`
untouched.

## Why it exists

- Audit visibility is role-sensitive (member / owner / super admin). Mocking it
  hides exactly the bugs we care about.
- The git-lrc CLI needs a working API key and config. Pointing the real CLI at
  a local server is useful, but must never leave your global config changed.
- AI and git connectors are often broken or rate-limited. Preflight classifies
  that as a capability gap instead of turning every test red.

## Safety model

By default the harness **never reads or writes `~/.lrc.toml`**. It creates a
private sandbox instead:

```
.lrdata/e2e/lrc-home/.lrc.toml
.lrdata/e2e/lrc-home/.lrc/
.lrdata/e2e/lrc-home/.gitconfig
```

`scripts/e2e/lrc.sh` runs `lrc` with `HOME` pointed at that sandbox and
`GIT_CONFIG_GLOBAL` pointed at its `.gitconfig`.

If you explicitly want to exercise the real config path, set
`E2E_USE_USER_LRC=1`. The harness then backs up `~/.lrc.toml`, writes a journal,
and restores it on `dev-down`. A later `dev-up` / `dev-down` / `e2e-doctor`
also recovers a stale journal first. The default sandbox needs no restore at
all: the original file is never touched.

## Setup

1. Copy the example and set the real account:

   ```bash
   cp scripts/e2e/e2e.env.example .env.e2e.local
   chmod 600 .env.e2e.local
   ```

2. Fill in `E2E_EMAIL`. If you have a local `/#/admin` password, set
   `E2E_PASSWORD` for non-interactive auth. Otherwise set `E2E_OTP` (local dev
   fixtures) or run `make e2e-auth` once; it opens the normal Hexmos sign-in
   and caches the session in `.lrdata/e2e/auth.json` (0600). With `E2E_OTP`
   present, `dev-up` can complete the login headlessly on its own.

3. Start the stack:

   ```bash
   make dev-up                 # default niceurl2
   make dev-up NICEURL=niceurl3   # manual-talent3
   ```

   The new `e2e` tmux window waits for API/UI, then provisions ephemeral API
   keys, a temp member, a temp admin-created member, and the lrc sandbox.

4. Run the tests:

   ```bash
   make e2e-status
   make e2e-preflight
   make e2e
   ```

## Commands

| Command | Purpose |
|---|---|
| `make dev-up NICEURL=niceurl3` | Start API/UI/worker/tunnel plus the E2E sandbox |
| `make dev-down` | Revoke ephemeral keys, deactivate temp users, remove sandbox, restore any journal |
| `make e2e-auth` | Refresh/cache the real browser session (`ARGS="--headless"` for a headless OTP login) |
| `make e2e-status` | Show API/UI health, run state, cached auth, sandbox path |
| `make e2e-doctor` | Check permissions, git-ignore status, required tools, and `~/.lrc.toml` safety |
| `make e2e-preflight` | Probe API/UI/worker/AI/git connector; writes `.lrdata/e2e/preflight.json` |
| `make e2e` | URL sweep, preflight, audit assertions, real lrc review, Playwright UI |
| `make e2e-audit` | Audit assertions only |
| `make e2e-review` | Real lrc review only |
| `make e2e-urls` | Probe all `manual-talent*` URLs |
| `make e2e-visual` | Playwright UI tests only |
| `make e2e-clean` | Clean up without forgetting the cached session |
| `make e2e-shell` | Shell with the generated `run.env` sourced |

Useful options:

```bash
make e2e ARGS="--skip-review"      # no AI review cost
make e2e ARGS="--skip-ui"
make e2e ARGS="--strict"           # broken AI/git/public URL is a failure
E2E_URLS="manual-talent2 manual-talent3" make e2e-urls
E2E_BASE_URL=https://manual-talent3.apps.hexmos.com \
E2E_API_URL=https://manual-talent3.apps.hexmos.com \
make e2e
```

## What is covered

- **Lifecycle**: API/UI health, selected reverse tunnel, worker presence,
  sandbox lrc, stale-journal recovery, unchanged real `~/.lrc.toml`.
- **Auth**: password login, cached session refresh, browser/SSO fallback,
  member login, password change.
- **Audit visibility**: member sees only self; owner sees org events but not
  `actor_capacity=admin` / `actor_kind=super_admin`; super admin sees admin
  capacity and cross-org events; member gets 403/404 on admin endpoints.
- **Audit behavior**: list/meta/facets, CSV/NDJSON/evidence exports and their
  sensitive-read events, API-key kind gating, integrity chain, append-only
  rejection (rolled-back transaction), retention settings read.
- **AI/git preflight**: a cheap chat hello-world and a stored git connector
  test. Broken AI/git marks review tests as skipped unless `--strict`.
- **Real review**: creates a temp git repo, submits staged changes through
  `scripts/e2e/lrc.sh`, polls the review list, and verifies the row.
- **UI**: audit page loads, filters/search, AI usage tab, CSV download,
  retention/integrity controls, member-only view, and container parity with
  the Reviews page at 1440px.

`make e2e` also probes every `manual-talent*` URL read-only. To run the full
mutation suite through a hosted tunnel, point both URLs at it:

```bash
E2E_BASE_URL=https://manual-talent3.apps.hexmos.com \
E2E_API_URL=https://manual-talent3.apps.hexmos.com \
make e2e
```

`run.sh` rewrites Playwright storageState for the requested origin, so the same
cached fixtures work locally or through the tunnel without re-provisioning.

The hash-chain check runs against a clean owner org. If the primary org has
historical rows from before the chain backfill, the report prints a
non-failing diagnostic (`owner-org chain diagnostic: broken at seq ...`) rather
than pretending old data is a live regression.

## Fixtures and the real account

The supplied real account can be a super admin in one org and an owner in
another. Because `ValidateOrgAccess` resolves any global super admin as
`super_admin` in every org, that account cannot also represent a pure owner.
The harness therefore creates two ephemeral real DB-backed users in the owner
org:

- a **member** for member-scope visibility,
- a **pure owner** for owner-scope visibility and for the lrc review key
  (owner AI-usage queries intentionally exclude super-admin work).

The cached super-admin session remains the real account and is used for the
admin/cross-org assertions.

If the deployment has no invitation email service, user creation falls back to
a local DB insert with a real bcrypt password. This is why a fully local stack
can still exercise member/owner scoping; the fallback is local-only and never
used against a hosted target.

## Test data and cleanup

- Every run has a unique `run_id` (`e2e-<timestamp>-<random>`).
- Temp member emails and API-key labels include the run ID.
- API keys are revoked and temp users deactivated on `dev-down` or
  `make e2e-clean`; a crashed run is replayed from the cleanup manifest.
- Audit rows are never deleted. They are append-only by design, and leaving
  the namespaced test history is expected.
- Billing/payment mutations are not run by default; they require an explicit
  opt-in.

## Current boundaries

- `make e2e` runs the full mutation suite against one selected target. The URL
  sweep is read-only across all five `manual-talent*` hosts, so test data is
  not multiplied by five.
- A production UI build is not part of this harness. UI tests use the dev
  server (or a deployed bundle at `E2E_BASE_URL`).
- The AI hello-world can fail because of a bad key, quota, or network. That is
  classified as `AI_CONFIG_BROKEN`; audit tests still run.
- Git connector health is independent of the local lrc review path. The
  connector test uses the stored GitHub credential; the review test does not
  require GitHub.

## Troubleshooting

- `make e2e-status` first.
- `make e2e-doctor` checks file modes, git-ignore, tools, and the real-lrc hash.
- If all public URLs show 502, the autossh tunnels are down; run
  `make dev-up NICEURL=niceurl3` or the matching `make niceurl3`.
- If auth is expired, run `make e2e-auth`.
- If the lrc sandbox is missing, run `make dev-up` (the e2e window recreates it)
  or `make e2e-up`.
- Reports:
  - `.lrdata/e2e/preflight.json`
  - `.lrdata/e2e/audit-report.json`
  - `.lrdata/e2e/review-report.json`
  - `.lrdata/e2e/url-report.json`
  - `.lrdata/e2e/artifacts/playwright-report/index.html`
