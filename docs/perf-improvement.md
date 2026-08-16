# LiveReview Production Performance: RCA and Fix Plan

**Status:** Fix 1 (bundle splitting) and Fix 2 (frontend caching layer) implemented and verified locally — see §6. Fixes 3-6 (backend query parallelization, connection pool sizing, chat streaming, csrf-token) not yet started.
**Trigger:** Login → dashboard → chat flow on `livereview.hexmos.com` takes ~2 minutes end to end, including on warm/cached visits
**Inputs analyzed:** HAR capture of full login→dashboard→chat session (`livereview.hexmos.com.har`), Chrome DevTools Recorder trace of the same session, and a codebase investigation of the corresponding backend/frontend code paths
**Owner:** TBD

---

## 1. Summary

A single login → dashboard → "ask the chat bot a question" session was captured end to end (HAR + Chrome Recorder). The observed timeline:

| Phase | Duration | What's happening |
|---|---|---|
| OTP submit → first API call fires | **~32.5s** | Dead air, zero network requests. Browser is downloading/parsing/executing JS. |
| First API call → dashboard mostly settled | **~70s** | ~25 API calls fire, many of them exact duplicates, each taking 250ms–2s server-side, staggered across several waves (including a re-fetch storm on alt-tab/window-focus) |
| User asks chat bot a question → answer renders | **~40s** | Single blocking HTTP call, fully synchronous multi-step LLM agent loop, nothing streamed |
| **Total** | **~142s** | Matches the reported "2 minutes" almost exactly |

None of this is a single bug — it's five independent, compounding problems, each owned by a different part of the stack:

1. A single 7.1MB obfuscated JS vendor bundle blocks first paint / first API call (~32s)
2. No frontend data-fetching cache — every component independently re-fetches the same endpoints (duplicate calls: `csrf-token` ×4, `dashboard` ×4, `dashboard/refresh` ×3, `billing/status` ×3, `quota/status` ×3, `system/info` ×2, etc.)
3. Every authenticated endpoint chains 6–15+ *sequential* DB round-trips through middleware + handler, and the DB connection pool (25 conns, single process) queues under load
4. `/api/v1/chat/send` blocks for the full duration of a 20-step LLM agent loop with no streaming
5. `/csrf-token` doesn't exist in production — every request silently 404s-to-SPA-fallback and CSRF protection is a no-op

Fixing all five is required to get this down from ~2 minutes to a "feels instant" experience (target: <3s to interactive dashboard, chat responses start streaming within ~1-2s).

---

## 2. Root Cause Analysis

### 2.1 The ~32.5s dead-air gap after login (bundle bootstrap)

**Evidence:** HAR shows zero network activity between the OTP-redirect landing and the first `/api/v1/ui-config` call. `pageTimings.onContentLoad` for that navigation is 32507ms.

**Root cause:**
- `ui/webpack.config.js:235-246` defines exactly one `splitChunks` cache group for all of `node_modules` (`test: /[\\/]node_modules[\\/]/, name: 'vendors', chunks: 'all'`). Every third-party dependency — charting libs, Monaco/grid-layout, diff viewer, everything — gets bundled into one chunk.
- Confirmed in the actual build output (`ui/dist/`): `vendors.<hash>.js` is **7.1MB**, the next-largest chunk is 286KB (a 20x gap).
- Production builds run `npm run build:obfuscated` (`Dockerfile:20`, `Makefile:668,766`), applying `webpack-obfuscator` with `stringArray: true`, `rotateStringArray: true`, `shuffleStringArray: true`, `stringArrayThreshold: 0.75` (`webpack.config.js:214-233`) to this same bundle. String-array-rotation obfuscation adds real per-access deobfuscation overhead on top of the raw parse cost of a 7MB file.
- Route-level `React.lazy` splitting *does* exist (`ui/src/App.tsx:16-35`), so this isn't a total absence of code-splitting — it's that the vendor chunk defeats it, since nearly everything ends up in one shared chunk loaded on every route including the login page.

**Why this matters most:** this delay happens on literally every page load, cached or not (JS parse/execute cost isn't fixed by HTTP caching — a cached-but-unparsed 7MB obfuscated bundle is still slow). This explains the user's "even when cached, it's not fast" observation.

### 2.2 No frontend caching layer → duplicate API calls

**Evidence:** Within one page session, HAR shows: `csrf-token` ×4, `dashboard` ×4, `dashboard/refresh` ×3, `billing/status` ×3, `quota/status` ×3, `billing/upgrade/request-status` ×3, `system/info` ×2, `billing/usage/me` ×2, `billing/usage/members` ×2, `activities` ×2.

**Root cause:**
- No React Query / SWR / RTK Query anywhere in the codebase (confirmed via grep across `ui/src` and `ui/package.json`). Every component independently calls `apiClient.get(...)` in its own `useEffect`.
- `/billing/status` is called independently from `Navbar.tsx:241`, `Dashboard.tsx:192`, `NewReview.tsx:109`, `TeamCheckout.tsx:255`, `SubscriptionTab.tsx:800`.
- `/quota/status` independently from `Navbar.tsx:192,242`, `Dashboard.tsx:193`, `NewReview.tsx:108`, `SubscriptionTab.tsx:801`.
- `/system/info` independently from `URLMismatchBanner.tsx:52`, `api/system.ts:15`, `api/auth.ts:196`, `Settings.tsx:436`, `DomainValidator.tsx:36`.
- `Navbar` and `URLMismatchBanner` are both mounted unconditionally in the app shell (`App.tsx:313-319`), so their calls fire on *every* page. `Dashboard.tsx` then duplicates the exact same `billing/status` + `quota/status` calls Navbar already made.
- **Re-fetch storm on window focus:** `Dashboard.tsx:132-163` (`loadDashboardData`) is wired to `window.addEventListener('focus', ...)` and `document.addEventListener('visibilitychange', ...)` (lines 157-161), in addition to a `setInterval(..., 5 * 60 * 1000)` poll (line 150). Confirmed against the Chrome Recorder trace: the Alt+Tab events during the session directly triggered additional full `dashboard/refresh` POSTs.
- A second, separate `useEffect` in `Dashboard.tsx` (lines 179-193) independently re-fetches `billing/status` + `quota/status` + `billing/upgrade/request-status` via `Promise.all`, uncoordinated with Navbar's identical calls.

### 2.3 Every endpoint is slow server-side, even ones with trivial payloads

**Evidence:** HAR timings show `connect: -1` (kept-alive connections, no handshake cost) but 250ms–2s of pure server "wait" time on calls that return a few KB of JSON. Critically, ~12 endpoints fired in parallel from the frontend all resolved in *lockstep* around the same ~1s mark, regardless of how much actual work each one does.

**Root cause — two layers:**

**(a) Sequential DB round-trips stacked in middleware + handlers, none parallelized:**
- Auth middleware alone does 6-7 sequential queries before any handler logic runs:
  - `ValidateAccessToken` (`internal/api/auth/token_service.go:188-250`): `SELECT EXISTS(...) FROM auth_tokens` → a **synchronous** `UPDATE auth_tokens SET last_used_at = NOW()` (line 224 — not async despite comments elsewhere suggesting it should be) → `SELECT ... FROM users WHERE id = $1`.
  - `BuildOrgContextFromHeader` (`middleware.go:307-338`): 1 query.
  - `ValidateOrgAccess` (`middleware.go:433-486`): 2 sequential queries (super-admin check, then role check).
  - `BuildOrgBillingPlanContext` (`org_billing_plan_context.go:15-63`): 1 query, applied to the billing/quota route group.
- `GET /api/v1/quota/status` (`internal/api/quota_handler.go:39-140`) adds its own chain on top: org-creator check, daily-usage count, a `CheckPreflight` call to the LOC accounting service, and (for paid plans) a seats query — **10+ sequential round trips total** for one "quota status" response.
- `POST /api/v1/dashboard/refresh` (`internal/api/dashboard.go:858-905`) sequentially calls, one after another (not parallelized): `RefreshOrgDashboard` (itself 4 sequential `QueryRowContext` calls + onboarding data + connector setup progress), then `RefreshOrgReviewLayers`, then `RefreshOrgSystemOverview`, then `RefreshOrgPeople` — each with its own query + cache get/upsert pattern. ~10+ more sequential round trips, all in one HTTP request.
- `GET /api/v1/system/info` (`server.go:2594-2645`) does **no DB calls** and has no auth middleware (public route). Its 900ms-2s observed latency in production isn't its own cost — it's contention (see below).

**(b) Small, contended connection pool + single-process API:**
- `internal/database/database.go:28-30`: `SetMaxOpenConns(25)`, `SetMaxIdleConns(10)`.
- `ecosystem.config.js:1-29`: `livereview-api` runs as a **single pm2 instance** (no cluster mode), alongside 2 separate `livereview-worker` processes, all competing for the same 25-connection ceiling.
- When the frontend fires ~12 endpoints in parallel (per §2.2) and each one internally chains 6-15 sequential queries (per §2.3a), the pool saturates fast. Cheap, DB-free requests like `system/info` end up queued behind DB-heavy requests inside the same single Go process, which is exactly why unrelated endpoints resolve in lockstep in the HAR.

### 2.4 `/api/v1/chat/send` — 39.8s, single blocking request, no streaming

**Evidence:** HAR shows one `POST /api/v1/chat/send` entry with `wait: 39830ms`. Chrome Recorder confirms this fires when the user clicks a suggested prompt ("Show me review activity per month and the top reviewers") in the chat widget.

**Root cause:**
- Route: `internal/api/server.go:1338-1347`, handler `internal/api/webchat_handler.go:74-179` (`HandleWebChat`).
- Line 105: synchronously connects to an internal MCP server (`mcpagent.ConnectMCP`).
- Line 127: `agent.RunTurnWithArtifacts(...)` is **fully awaited** before `c.JSON(...)` is called (line 141) — nothing streams back until the entire agent turn is done.
- `RunTurnWithArtifacts` (`internal/mcpagent/agent.go:97`) runs a multi-step agent loop with `maxSteps := 20` (set in `webchat_handler.go:95`): a classify LLM call first ("Call #0"), then depending on classification, either a tool-use loop or a count-query plan — each of which can issue multiple further sequential LLM calls, all inside the one HTTP request.
- Frontend: `ui/src/api/chatbot.ts:49-58` (`sendChatMessage`) does a plain `apiClient.post<ChatResponse>(...)` and awaits the full response — no `EventSource`, no WebSocket.
- No SSE/WebSocket infrastructure exists for chat anywhere in the codebase. The only `text/event-stream` reference found is `internal/mcpagent/mcp_client.go:241`, which is the *internal* agent→MCP-server protocol, unrelated to streaming replies back to the browser.

### 2.5 `/csrf-token` is dead in production — silent no-op + wasted round trip

**Evidence:** HAR shows `GET /csrf-token` returning `200 text/html` (4 times per session), not JSON.

**Root cause:**
- The Go API server has no `/csrf-token` route anywhere in `internal/api/server.go`.
- Production runs the Go UI-proxy server (`ecosystem.config.js:22-29` → `cmd/ui.go`), which proxies `/api/*` only (`cmd/ui.go:131-134`) and falls back to serving `index.html` for anything unmatched — so `/csrf-token` silently 200s with the SPA shell.
- Frontend: `ui/src/api/apiClient.ts:72-95` (`ensureCsrfToken`) fetches `/csrf-token`, `res.json()` throws on the HTML body, and the error is swallowed by `.catch(() => ({}))` (line 87) — `csrfTokenCache` stays `null` forever. CSRF protection is silently inert in production.
- A real `csurf`-based implementation exists in `ui/server.js:1-32` but is dead code: not referenced by any `ui/package.json` script, the Dockerfile, or `ecosystem.config.js`. Leftover from an earlier Express/CRA-based deployment.

---

## 3. Fix Plan

Ordered by impact-to-effort ratio. Each item is scoped to be shippable independently — no single PR needs to touch more than one of these areas.

### Fix 1 — Split the vendor bundle, stop obfuscating the biggest chunk
**Addresses:** §2.1 (~32s bootstrap gap)
**Effort:** Small
**Where:** `ui/webpack.config.js:235-246`, `ui/webpack.config.js:214-233`

- Replace the single `vendors` cache group with multiple groups keyed by library (e.g. charting library, Monaco/grid-layout, diff viewer, date libs, the rest) so route-level `React.lazy` splits actually take effect — a user landing on the dashboard shouldn't download Settings-only or Checkout-only vendor code.
- Set a `maxSize` on `splitChunks` so webpack auto-splits any cache group that's still too large.
- Re-evaluate whether `webpack-obfuscator`'s heaviest settings (`stringArray`, `rotateStringArray`, `shuffleStringArray`, `stringArrayThreshold: 0.75`) need to apply to every chunk, or just chunks containing proprietary logic — vendor code doesn't need string-array rotation, and it's likely most of the 7.1MB.
- **Verify:** rebuild, confirm largest chunk size is meaningfully reduced (target: no single chunk over ~1MB before gzip), and re-capture a full HAR (all resource types, not XHR-filtered) on the login flow to measure the actual bootstrap time reduction.

### Fix 2 — Add a shared frontend data-fetching/cache layer
**Addresses:** §2.2 (duplicate calls, re-fetch storms)
**Effort:** Medium (mechanical but touches many files)
**Where:** New dependency (`@tanstack/react-query` or similar) + `Navbar.tsx`, `Dashboard.tsx`, `NewReview.tsx`, `TeamCheckout.tsx`, `SubscriptionTab.tsx`, `URLMismatchBanner.tsx`, `api/system.ts`, `api/auth.ts`, `Settings.tsx`, `DomainValidator.tsx`

- Introduce React Query (or equivalent) as the single source of truth for server state. Start with the highest-duplication endpoints: `billing/status`, `quota/status`, `system/info`, `dashboard`.
- Migrate each independent `useEffect`-based fetch in the files above to a shared query hook keyed by endpoint (e.g. `useBillingStatus()`, `useQuotaStatus()`, `useSystemInfo()`) so five components calling the same endpoint collapse into one network call.
- Set sane `staleTime`/`cacheTime` per endpoint — `system/info` barely changes and can be cached for minutes; `quota/status` needs to be fresher but still doesn't need to be re-fetched by every component independently.
- Replace the manual `window.addEventListener('focus', ...)` / `visibilitychange` refetch in `Dashboard.tsx:157-161` with React Query's built-in `refetchOnWindowFocus` (scoped to the specific queries that actually need it) rather than an unconditional full `dashboard/refresh`.
- **Verify:** re-capture HAR on the same login→dashboard flow, confirm each endpoint fires once (not 2-4x) per page load, and confirm alt-tab no longer triggers a full refresh cascade.

### Fix 3 — Parallelize sequential server-side work, trim the auth middleware chain
**Addresses:** §2.3a (900ms-2s per "simple" endpoint)
**Effort:** Small-Medium
**Where:** `internal/api/dashboard.go:858-905`, `internal/api/quota_handler.go:39-140`, `internal/api/auth/token_service.go:188-250`, `internal/api/auth/middleware.go:433-486`

- `dashboard.go:876-905` — run `RefreshOrgReviewLayers`, `RefreshOrgSystemOverview`, `RefreshOrgPeople` concurrently (goroutines + `errgroup`) instead of sequentially; they don't appear to depend on each other's output.
- `quota_handler.go` — parallelize the independent checks (org-creator check, daily-usage count, `CheckPreflight`, seats query) where they don't have data dependencies.
- `token_service.go:224` — make the `UPDATE auth_tokens SET last_used_at` fire-and-forget (background goroutine, don't block the request on it). It's a bookkeeping write, not something the request needs to wait on.
- `middleware.go:433-486` (`ValidateOrgAccess`) — check whether the super-admin check and role check can be combined into one query instead of two sequential ones.
- **Verify:** re-run HAR capture, confirm `dashboard/refresh` drops from ~1-1.9s to a few hundred ms, and `quota/status`/`billing/status` similarly drop.

### Fix 4 — Right-size the DB connection pool and process concurrency
**Addresses:** §2.3b (request queuing under parallel load)
**Effort:** Small
**Where:** `internal/database/database.go:28-30`, `ecosystem.config.js:1-29`

- Raise `SetMaxOpenConns`/`SetMaxIdleConns` from the current 25/10 ceiling — check actual Postgres `max_connections` on the prod instance first to know the real headroom, and account for the 2 worker processes sharing the same DB.
- Evaluate running `livereview-api` in pm2 cluster mode (multiple instances) rather than a single process, so a burst of parallel requests isn't serialized through one Go process/event loop.
- This fix should land *after* Fix 3 (parallelizing sequential work) — otherwise a bigger pool just lets the same wasteful sequential-query pattern run more requests concurrently instead of fixing the underlying inefficiency.
- **Verify:** load-test the dashboard-load burst pattern (∼12 parallel requests) against a staging environment before/after, confirm resolution times stop clustering in lockstep.

### Fix 5 — Stream `/api/v1/chat/send` instead of blocking for the full agent turn
**Addresses:** §2.4 (40s blocking chat response)
**Effort:** Medium (new SSE endpoint + frontend streaming consumer)
**Where:** `internal/api/webchat_handler.go:74-179`, `internal/mcpagent/agent.go:97`, `ui/src/api/chatbot.ts:49-58`, chat UI component that renders the response

- Convert `HandleWebChat` to an SSE (or WebSocket) response: flush an event as each agent step / classify result / partial answer becomes available, rather than buffering the whole `RunTurnWithArtifacts` result.
- `RunTurnWithArtifacts` likely needs a callback/channel parameter so `agent.go` can emit progress events (e.g. "classifying", "running tool X", "step N/20") as it goes, instead of returning only at the end.
- Frontend: replace the `apiClient.post` + await in `chatbot.ts` with an `EventSource` (or `fetch` + `ReadableStream`) consumer that renders tokens/steps progressively, plus a visible "thinking" / step-progress indicator so the UI doesn't look frozen even before the first content arrives.
- Even without full token-level streaming, just emitting step-boundary events (e.g. "step 3 of 20 done") would make the 40s feel dramatically shorter and give users confidence it's not hung.
- **Verify:** re-run the same "Show me review activity per month and top reviewers" prompt from the Recorder script, confirm the UI shows visible progress within ~1-2s of submission rather than a blank wait.

### Fix 6 — Fix or remove `/csrf-token`
**Addresses:** §2.5 (silent CSRF no-op + wasted round trip)
**Effort:** Small
**Where:** `ui/src/api/apiClient.ts:72-95`, `ui/server.js` (dead code), Go server routing

- Decide: either wire real CSRF protection into the Go API server (issue a real token at a real `/csrf-token`-equivalent route, e.g. as part of the session/auth response) or remove the client-side `ensureCsrfToken` call entirely if CSRF isn't actually needed given the auth model (e.g. if all mutating requests already require a bearer token, CSRF risk may be low — confirm this before removing).
- Delete the dead `ui/server.js` Express/`csurf` implementation once the decision is made, so it doesn't mislead future readers into thinking CSRF is handled.
- **Verify:** confirm no more `text/html` responses to `/csrf-token` in HAR, and confirm (via a security review) whichever path is chosen actually protects mutating endpoints.

---

## 4. Suggested Sequencing

1. **Fix 1 (bundle splitting)** and **Fix 6 (csrf-token)** — independent, low-risk, ship first, immediate visible wins on the ~32s gap.
2. **Fix 3 (parallelize server work)** before **Fix 4 (pool sizing)** — fix the inefficiency before scaling the pool that masks it.
3. **Fix 2 (frontend caching layer)** — can proceed in parallel with the backend fixes; biggest reduction in *number* of requests even before any single request gets faster.
4. **Fix 5 (streaming chat)** — largest single-endpoint win (40s → perceived near-instant), but also the most involved (needs an SSE/streaming contract change), so it's reasonable to tackle last or in parallel with a dedicated push.

## 5. Verification Plan

For each fix, re-capture a HAR (full resource types this time, not XHR-filtered, to also see JS bundle load timing) and Chrome Recorder trace of the same login → dashboard → chat-question flow used in this investigation, and compare against the baseline numbers in §1. Track:

- Time from OTP submit to first API call (baseline: ~32.5s)
- Number of duplicate calls per endpoint per session (baseline: up to 4x)
- p50/p95 server wait time on `billing/status`, `quota/status`, `dashboard/refresh`, `system/info` (baseline: 900ms-2s each)
- Time to first content from `chat/send` (baseline: 39.8s to any content at all)
- Total time from login submit to chat answer fully rendered (baseline: ~142s)

---

## 6. Implementation Notes (Fixes 1 & 2)

### Fix 1 — bundle splitting (`ui/webpack.config.js`)

Implemented as planned, with one correction found along the way: `WebpackObfuscator`'s `exclude` is **not** a valid key inside its options object (confirmed against `node_modules/webpack-obfuscator/README.md`) — the plugin only accepts an excludes list as a *second constructor argument*. The existing config passed `exclude: /node_modules/` inside the options object, which the plugin silently ignores, so the vendor bundle was being obfuscated too (confirmed via the `a22aJb=a22d`-style string-array dispatch pattern at the top of the pre-fix `vendors.*.js`). Fixed by moving the exclusion to the second argument.

Cache groups replaced: one `vendors` group forcing every `node_modules` module (regardless of whether it's reachable synchronously or only from a lazy `React.lazy` route) into a single named chunk, with four groups:
- `vendor-framework` (react/react-dom/redux/router) — kept eager, small and stable.
- `vendor-charts` (echarts, vega*, recharts, d3-*) — `chunks: 'async'`, confirmed via `git grep` that none of these are imported outside already-lazy routes (Dashboard widgets, Reports, Chatbot).
- `vendor-heavy` (react-grid-layout, framer-motion, jspdf*, @tanstack/react-table, moment-timezone) — same, `chunks: 'async'`. `moment-timezone` alone ships a 724KB packed locale-data file that was previously loading on every page including login.
- `vendor-misc` (everything else) — deliberately **no static `name`**, only a naming *function* that still prefixes `vendor-` (so the obfuscator excludes glob keeps matching) while letting webpack split by actual usage graph. A static name here was tried first and reintroduced the original bug: it force-merges sync-needed and lazy-only modules into the same physical file, dragging lazy-only code (again, `moment-timezone`) back into the eager payload.

**Result** (measured from the actual `ui/dist` build output, `npm run build`):

| | Before | After |
|---|---|---|
| Eager entrypoint (`index.html`'s script tags, what has to download+parse+execute before React can mount) | ~7.4MB (single `vendors.js` was 7.1MB alone) | ~570-610KB |
| Largest single chunk | 7.1MB | 708KB (`vendor-heavy`, moment-timezone data — async-only, not in the eager path) |

That's roughly a **12-13x reduction** in what blocks first render, on every page including login. Verified: production build (`npm run build`) completes cleanly; obfuscated build (`OBFUSCATE=true npm run build`) confirmed `vendor-*` chunks are plain minified code (no obfuscation dispatch pattern) while `main.js` and lazy route chunks are correctly obfuscated; headless Chrome render of the built `dist/` shows the login page mounts correctly with no console errors.

### Fix 2 — shared frontend data-fetching cache (`@tanstack/react-query`)

Added `@tanstack/react-query` (`ui/src/api/queryClient.ts`, wired into `ui/src/index.tsx` above `AppContextProvider`). Default `staleTime: 30s`, `refetchOnWindowFocus: false` globally — this directly removes the alt-tab refetch-storm behavior identified in §2.2, rather than just scoping it down.

Migrated to shared query hooks:
- **`billing/status`, `quota/status`, `billing/upgrade/request-status`, `billing/usage/me`, `billing/usage/members`** — new generic hooks in `ui/src/api/billing.ts` (`useBillingStatusQuery<T>()` etc.), generic over the response type so each call site keeps its own locally-defined response shape while sharing the same `queryKey` (and therefore the same cached request) across files. Wired into `Navbar.tsx` (`BillingChip`, previously a raw `useEffect` + `apiClient.get` pair for two divergent self-hosted/cloud code paths, now `useMemo`-derived from the query results), `Dashboard.tsx` (previously its own independent `Promise.all` + 60s manual `setInterval`, now the same shared queries with `refetchInterval: 60_000`), and `NewReview.tsx`.
- **`/api/v1/dashboard` + `/api/v1/dashboard/refresh`** — new `useDashboardQuery()` in `ui/src/api/dashboard.ts`, wired into `Dashboard.tsx` and all three widget-layer context providers (`PeopleData.tsx`, `SystemOverviewData.tsx`, `ReviewLayersData.tsx`, which previously each independently called `getDashboardData()` on mount) plus `CreateReviewCLI.tsx`. This also removes the `window.addEventListener('focus', ...)`/`visibilitychange` listeners on `Dashboard.tsx` that were confirmed (via the Chrome Recorder trace) to re-trigger the full expensive `dashboard/refresh` cascade on every alt-tab.
- **`/api/v1/system/info`** — `ui/src/hooks/useSystemInfo.ts` rewritten to use `useQuery` instead of local `useState`/`useEffect`, preserving its exact return shape (including the original error-fallback behavior of returning `{dev_mode: false, version: null}` rather than `null` on failure) so no caller needed to change.

**Deliberately left unchanged:**
- `TeamCheckout.tsx`'s `/billing/status` poll — this is an active post-checkout activation-confirmation loop (up to 24 attempts) that needs uncached, live reads on every attempt to detect a plan change; putting it behind the shared 30s-stale cache would break its purpose.
- `SubscriptionTab.tsx`'s billing/quota fetch — sends org-scoped `X-Org-Context` headers and bundles additional endpoints (`billing/usage/summary`, `billing/usage/operations`) the shared hooks don't cover; forcing it into the generic hooks risked showing the wrong org's billing data on a page where that would be a real correctness bug, not just a perf regression.

**New finding surfaced during this work (not yet fixed):** `URLMismatchBanner.tsx`, `DomainValidator.tsx`, `Settings.tsx`, and `LicenseStatusBar.tsx` (via `api/system.ts`'s `getSystemInfo`) all call a bare `/system/info` (no `/api/v1` prefix) — a route that **does not exist** on the Go backend (only `/api/v1/system/info` does). In production this silently 404s-through to the SPA's `index.html` fallback via the UI proxy, the same failure mode as the `/csrf-token` bug in Fix 6. This is separate from the `/api/v1/system/info` duplication this pass fixed (that one's real and now deduped). Left alone here since fixing it requires confirming the correct response shape against the real backend handler first — folding it into Fix 6 (dead-route class of bugs) is the natural place for it.

**Result:** verified with `npx tsc --noEmit` (clean) and a full production build (clean, entrypoint size unaffected — react-query adds ~40KB). Not yet verified against a live backend/HAR recapture since that requires a real login session against `livereview.hexmos.com`; recommend re-running the HAR + Recorder capture from §1 once this ships to confirm each endpoint now fires once per page load instead of 2-4x.
