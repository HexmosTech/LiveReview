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

### Fix 1 correction — chunk over-fragmentation regression (found via a second HAR capture post-deploy)

After shipping Fixes 1 & 2, a fresh HAR (`~/Downloads/lr-perf-debug/v2`, full resource types this time, not XHR-filtered) confirmed Fix 1 worked for the login page itself — `onContentLoad` on the post-login navigation dropped from **32.5s to 5.4s** — but exposed a second problem: **`onLoad` was still 35s**. Right after auth resolved (~6.3s in), the browser fired **~90 separate JS chunk requests simultaneously** (35 `vendor-charts-*` files, 5 `vendor-heavy-*`, 39 `vendor-misc-*`), with HAR durations climbing linearly from ~2s to **28.5s** — the textbook signature of requests queuing behind the browser's per-origin connection limit.

**Root cause:** `optimization.splitChunks.maxSize: 300000`, added in the original Fix 1 pass specifically to stop any single vendor group from re-growing into one giant file, was applying globally — including to `vendor-charts`/`vendor-heavy`/the async `vendor-misc` fallback. Those groups aren't small: echarts + vega/vega-lite/vega-embed + recharts + react-grid-layout + framer-motion + jspdf together are several MB, so the 300KB cap fragmented each into 30-40 pieces. That's fine in isolation *if* the pieces are genuinely deferred until independently needed — but they aren't: **Dashboard is the very first route rendered after login** (not something visited later), and it statically imports all of its echarts-based widgets directly rather than lazy-loading each one individually. `React.lazy`'s Suspense boundary can't render anything until the *entire* dynamic-import dependency graph resolves, so every one of those ~80 fragments had to arrive before Dashboard could paint at all — and 80 simultaneous requests is worse than a handful of large ones, not better, once they're all needed at the same instant rather than deferred.

**Fix:** removed the blanket `maxSize`. Verified in the rebuilt `ui/dist`:

| | With `maxSize: 300000` (regression) | Without `maxSize` (fixed) |
|---|---|---|
| Eager login payload | ~570-610KB (unaffected either way) | ~543KB (unaffected either way) |
| `vendor-charts` | 35 files, largest 250KB | **1 file, 1.7MB** |
| `vendor-heavy` | 5 files, largest 725KB | **1 file, 1.16MB** |
| async `vendor-misc` | 39 files, largest 200KB | **1 file, 1.9MB** |
| Total chunk requests Dashboard needs | ~90 | ~7 |

The login page's eager payload is untouched (confirmed identical size), so Fix 1's original win is preserved. Dashboard's total byte weight is roughly unchanged (still needs the same charting libraries), but collapsing ~90 discrete round-trips into ~7 large parallel HTTP/2 streams should eliminate the connection-queuing tail that was driving the 28.5s worst-case chunk latency and the 35s `onLoad`. **Not yet re-verified against a live HAR** — this was diagnosed and fixed from the v2 capture but needs a third capture post-deploy to confirm the queuing tail is actually gone in production.

**Follow-up worth calling out (not addressed, larger scope):** the app ships three different charting libraries — `echarts`, `vega`/`vega-lite`/`vega-embed`, and `recharts` — which is most of why `vendor-charts` alone is 1.7MB. Standardizing on one would shrink Dashboard's *actual* byte requirement rather than just changing how the existing bytes are packaged, which is the more durable fix if dashboard load time is still not snappy enough after this correction lands.

**Result:** verified with `npx tsc --noEmit` (clean) and a full production build (clean, entrypoint size unaffected — react-query adds ~40KB). Not yet verified against a live backend/HAR recapture since that requires a real login session against `livereview.hexmos.com`; recommend re-running the HAR + Recorder capture from §1 once this ships to confirm each endpoint now fires once per page load instead of 2-4x.

## 7. The real fix — why Dashboard needed ~4.7MB, not just how it was packaged

The chunk-fragmentation correction in §6 fixed *how* bytes were delivered (few large files instead of ~90 queuing ones) but left the underlying byte count — `vendor-charts` 1.7MB + `vendor-heavy` 1.16MB + async `vendor-misc` 1.9MB ≈ 4.7MB — unchanged. Correctly called out as still not good enough: a dashboard shouldn't need that much code. A deeper investigation (3 parallel Explore agents cross-validated against `node_modules` internals, plus a Plan agent that verified every claim by reading the actual widget source) found three independent, fixable root causes, all now implemented in `ui/webpack.config.js`, `ui/src/components/Dashboard/widgets/*`, and `ui/src/pages/Reports/*`:

1. **echarts was imported wastefully.** All 6 Dashboard chart widgets used the default `echarts-for-react` export, which (confirmed by reading `node_modules/echarts-for-react/lib/index.js`) always `require()`s the *entire* `echarts` package internally regardless of app code — and `echartsTheme.ts` did the same full-package import just to register a theme. echarts' modular API (`echarts/core` + `echarts.use([...])` + `echarts-for-react/lib/core`) was unused anywhere in the repo. **Fixed:** `echartsTheme.ts` now imports `echarts/core` and registers exactly the 9 chart types (Heatmap, Gauge, Radar, Sunburst, Treemap, Sankey, Bar, Pie, Line) and 8 components (Tooltip, Grid, Calendar, VisualMap, Legend, Title, DataZoom, DataZoomSlider) the app actually uses, verified per-widget against each one's real `option` config (e.g. confirmed `CoverageGauge`'s `title` block is the gauge's own built-in center-label, not the global `TitleComponent`, so it correctly needs zero components). All 6 widgets now import `echarts-for-react/lib/core` and pass the configured core via a new `LR_ECHARTS_CORE` export.

2. **recharts was a fully redundant, single-consumer library.** Used only by one reused chart component (`TrendAreaChart`, 4 call sites, cosmetic prop variants only) in `src/pages/Reports/TaxonomyReports.tsx`. Investigation corrected an initial assumption that its `Brush` was purely decorative — it actually does real (if minor) in-chart pan/zoom internally even with no `onChange` wired — so the port preserves that via echarts' `dataZoom` slider rather than dropping it. **Fixed:** ported to a new `src/pages/Reports/TrendAreaChart.tsx` using the same `echarts-for-react/lib/core` pattern as the Dashboard widgets; deleted the inline recharts version and the `recharts` import; removed `recharts` from `package.json` (confirmed via `npm ls d3-shape` that its shared `d3-*` transitive deps are also independently required by `vega`, so nothing else broke); ran `npm install` to regenerate the lockfile (15 packages removed).

3. **Webpack cache groups over-merged unrelated libraries by static chunk name** — the same bug class fixed for the `vendor` fallback group in §6, but never applied to `charts`/`heavy`, and (newly discovered while implementing) present in the `vendor` fallback group too despite appearing fixed. A static `name:` string forces every module matching a cache group's regex into one physical file regardless of which route imports it. Confirmed: `vega`/`vega-lite`/`vega-embed` (Chatbot-only — Chatbot is its own `React.lazy()` route, confirmed never touched by Dashboard) were merged into the same file as echarts; `moment-timezone` (Settings/Licenses-only), `jspdf` (Reports-only), and `@tanstack/react-table` (DataTable-consumer routes, not Dashboard) were merged with Dashboard's real `react-grid-layout`/`framer-motion` needs; and even the `vendor` fallback (thought already fixed) was dumping `vega` (900KB), `moment` (685KB), `html2canvas`/`canvg`/`dompurify` (jspdf's rendering deps) together with `zrender` (610KB, echarts' own rendering engine — genuinely Dashboard's) into one shared 1.9MB file. **Fixed:** all three cache groups (`charts`, `heavy`, `vendor`) now name their output chunks by *(package name, consuming-chunk-set)* instead of a static string or bare chunk-set — implemented as a shared `usageGroupName()` helper in `webpack.config.js`. This needed one iteration: naming purely by consuming-chunk-set (matching the pattern already used for `vendor`) hit a hard webpack build error (`Cache group "heavy" conflicts with existing chunk`) because two *different* libraries' consuming-chunk signatures both collapsed to the same degenerate empty key at name-computation time; adding the package name into the key (extracted from each module's `resource` path) resolved it and is more precise besides.

### Verified results (`ui/dist`, production build)

| | Before this section's fixes | After |
|---|---|---|
| Eager login payload | ~543KB | ~501-543KB (unchanged, as expected) |
| `echarts` (Dashboard's own chunk) | 908KB-930KB (full package) | **624KB** (modular — remainder is mostly `zrender`, the mandatory rendering engine, not unused chart types) |
| `react-grid-layout` + `framer-motion` (Dashboard's own) | merged into 1.16MB `vendor-heavy` blob with unrelated libs | **25KB + 34KB**, isolated |
| `vega`/`vega-lite`/`vega-embed`/`d3-*` (Chatbot-only) | merged into Dashboard's `vendor-charts`/`vendor-misc` | **0 bytes** — structurally confirmed absent from Dashboard's chunk (see below) |
| `moment-timezone`/`moment`/`jspdf`/`html2canvas` (Settings/Licenses/Reports-only) | merged into Dashboard's `vendor-heavy`/`vendor-misc` | **0 bytes** — same |
| `recharts` | 262-268KB, single-consumer | **removed from the dependency tree entirely** |
| Dashboard's total real chart/heavy need | ~2.86MB (`vendor-charts` 1.7MB + `vendor-heavy` 1.16MB, both shared with unrelated routes) | **~683KB** (624KB echarts + 34KB framer-motion + 25KB react-grid-layout) |
| Build warnings | 2 (asset/entrypoint size limits exceeded) | **0** — every chunk now under the 244KB recommended threshold except the handful of large-but-correctly-isolated single-library files (echarts, moment-timezone, jspdf, vega-lite) |

**Structural verification** (not just file sizes — confirmed Dashboard's actual chunk-loading graph): read `stats.json`'s chunk-sibling data for Dashboard's own compiled chunk. Its siblings are exactly `vendor-charts-echarts-~~`, `vendor-heavy-react-grid-layout-~`, `vendor-heavy-framer-motion-~`, and Dashboard's own widget/component chunks — no `moment-timezone`, `jspdf`, `vega`, `recharts`, or `d3-*` anywhere in the list. This proves the browser will not fetch that unrelated weight when a user loads Dashboard, not just that the bytes exist in smaller files somewhere.

**Verified:** `npx tsc --noEmit` clean; production build clean (zero warnings, previously 2); obfuscated build (`OBFUSCATE=true`) clean, confirmed all `vendor-*` chunks remain correctly excluded from obfuscation (plain minified code) while `main.js` remains obfuscated; headless Chrome render of the built `dist/` confirms the app still boots with no console errors.

**Not yet verified:** actual chart rendering correctness for all 7 widgets + 4 `TrendAreaChart` call sites (this sandbox has no backend to log into and load real dashboard data against) and the `dataZoom` slider behavior on the ported trend chart — needs a manual pass after deploying. Also needs a fourth HAR capture to confirm the real-world load-time improvement matches the structural byte reduction.

## 8. Sequencing, redundant server work, and infra — why it was still ~1-1.5 minutes to first graph

A third HAR (`~/Downloads/lr-perf-debug/v3`) showed §7's byte-reduction work is landing (`onContentLoad` now 6.8s, down from 32.5s originally) but the user still measured ~1-1.5 minutes to a usable dashboard, with "4 different loaders." Three parallel investigations found the remaining bottleneck had shifted away from bundle size entirely, into a frontend sequencing bug, redundant backend work, and a production nginx gap:

**A. Dashboard's data-fetching was gated behind echarts' download.** `Dashboard.tsx` sat behind one `React.lazy()` boundary that statically imported all 7 echarts widgets (via `registry.ts`), so React couldn't start running `Dashboard`'s component — meaning none of its `useQuery` calls fired — until the *entire* graph, echarts included, finished downloading. HAR-confirmed: no dashboard-specific request fired until the exact millisecond echarts' download completed, ~8.7s of dead time for data-fetching that has zero actual dependency on echarts. **Fixed:** the 7 echarts-based widgets (`ReviewPipelineSankey`, `IssueCategoryRadar`, `ReviewVolumeBar`, `RepoHierarchySunburst`, `CoverageGauge`, `UsageShareDonut`, `ContributionCalendarHeatmap`) are now individually `React.lazy()`-wrapped in `registry.ts`, with a per-widget `<Suspense fallback={<ChartSkeleton />}>` in `DashboardGrid.tsx` (not the outer route-level one, which would otherwise unmount the whole dashboard when any one widget suspends). Verified via `stats.json`: Dashboard's own chunk dropped to ~75KB, and everything that must download before Dashboard's shell can mount and fire its data queries is now ~274KB total with **no echarts chunk in that list** — each widget now pulls echarts in independently, in parallel with (not blocking) the dashboard's data-fetching.

**B. The frontend was forcing an expensive backend recompute on every single page load, when a background job already keeps the data fresh.** Confirmed: `DashboardManager.Start()` (`internal/api/dashboard.go:319-349`) already runs a 5-minute background refresh cycle for every org, advisory-lock-protected across the multi-process deployment — `dashboard_cache` is genuinely kept fresh independent of user activity, exactly matching the user's own assumption ("all the graph data is precalculated"). `GET /api/v1/dashboard` is correspondingly cheap (in-memory cache + a few indexed reads). But `useDashboardQuery()`'s queryFn was calling `POST /api/v1/dashboard/refresh` (~25-30 sequential DB round trips, several scanning unbounded org history, 1-2+ seconds) before every GET — on every mount and every 5-minute client refetch, per open tab, pure redundant work. **Fixed:** `ui/src/api/dashboard.ts`'s `useDashboardQuery()` now calls `getDashboardData()` directly, no refresh. `refreshDashboardData()` is preserved for an explicit user action — `DashboardGrid.tsx`'s "Refresh widgets" button now uses a real `useMutation` that calls it and invalidates the dashboard query, instead of its previous no-op-ish `refetch()` calls (which, now that the queryFn is a plain GET, would have just silently re-read the same cache).

**C. Not 4 but up to 8 distinct loading UI states were stacking up**, several pure duplicates. Full audit in `App.tsx`/`index.tsx`/`Cloud.tsx`/`RecentActivity.tsx`/`ChartSkeleton`-consuming widgets found: a static HTML boot screen and a near-identical React `BootScreen` rendering back-to-back (pure duplication); a "Loading LiveReview..." auth-check screen that could inconsistently leak through depending on network speed; `RouteFallback` firing twice (once for the Login/Cloud chunk, once for the Dashboard chunk) plus Cloud.tsx's own "Logging you in..." spinner — three different full-screen spinner styles back-to-back for one conceptual "signing you in" wait; and `RouteFallback` (spinner+text) handing off to `ChartSkeleton` (pulse-bar grid) as two different loading grammars for one continuous wait. **Fixed the two safe, high-value ones this pass:** removed the duplicate React `BootScreen` entirely (`App.tsx`), and changed `index.tsx`'s static-boot-screen hide call to wait one `requestAnimationFrame` after `root.render()` instead of firing immediately, so a real paint happens first (avoids a flash-of-blank-content regression from removing the React overlay's safety-net timeout). **Left as-is, flagged for a future pass:** consolidating `RouteFallback`/Cloud's spinner/`ChartSkeleton` into one continuous visual language — touches the auth/routing flow more invasively, and Steps A+B already remove the two longest individual waits in that chain.

Also found and fixed a **separate, recurring bug**, not part of the login sequence but compounding the "too much spinning" impression: `RecentActivity.tsx`'s 30-second `setInterval` poll (confirmed via HAR: `/api/v1/activities` refetching at 16.6s/46.6s/76.6s, exactly 30s apart) was unconditionally setting `isLoading=true` before every fetch, including the silent background ones — tearing the whole "Recent Activity" card down to a full spinner and rebuilding it every 30 seconds for as long as the dashboard stayed open. **Fixed:** `loadActivities` now takes a `background` flag; the 30s interval passes it, so only the true first load (and error-retry) show the full-card spinner.

**D. Production nginx has no upstream keepalive.** The actual production config is in-repo at `docs/deployment/livereview.hexmos.com`. It proxies `/api/` → the API process (:8888) and `/` → the UI/static process (:8081), both single pm2 instances (`ecosystem.config.js`, no cluster mode). Confirmed by reading `cmd/ui.go` in full: static file serving has zero DB/auth/concurrency-limiting code (embedded `fs.FS`, no disk I/O per request) — yet the HAR showed static `.js`/`.css` requests stalling 3-5s in TTFB during a concurrent burst, identically to DB-backed API calls. Root cause: the nginx config had no `proxy_http_version 1.1`/`Connection` header override/`upstream{keepalive}` block, so nginx's default behavior opens a **brand-new TCP connection to the single backend process for every proxied request** — invisible under light load, but compounding across a burst of 10-15 simultaneous requests (exactly what fires right after login). **Fixed in-repo:** added `upstream livereview_api`/`upstream livereview_ui` blocks with `keepalive 32`, plus `proxy_http_version 1.1;` and `proxy_set_header Connection "";` on both `location` blocks (required for the keepalive pool to actually engage). **This requires manual deployment** — per `docs/deployment/README.md`, this file is copied to `/etc/nginx/sites-available/` and nginx is reloaded, not auto-synced; I edited the in-repo file but cannot deploy or reload production nginx myself.

**Also quantified but deliberately deferred to a future pass:** every authenticated API request still pays 6-9 fully sequential DB round trips in the auth middleware chain (`ValidateAccessToken`'s 3 queries including a synchronous `last_used_at` UPDATE, `BuildOrgContext` 1, `ValidateOrgAccess` 2, `EnforceSubscriptionLimits` 1-2) against an unchanged 25-connection pool (`internal/database/database.go:28-30`, `internal/api/server.go:273-275`) — the original session-1 Fix 3/4, still not implemented, now precisely quantified with file:line detail. With fix B removing the expensive refresh call, this middleware chain is now proportionally more significant for the many small calls (`quota/status`, `billing/status`, `system/info`) that still fire on every load. Flagged as core auth/security-path Go code warranting its own careful review pass rather than bundling into this change set.

**Verified:** `npx tsc --noEmit` clean; production build clean; obfuscated build clean (all `vendor-*` chunks still correctly un-obfuscated); `stats.json` confirms Dashboard's own chunk dropped to ~75KB with echarts no longer in its blocking dependency set; headless Chrome render confirms the app still boots with no console errors.

**Not yet verified:** the nginx config change requires the user to manually deploy + reload on the production host (I can't do this from here) before it takes effect. A fifth HAR + Recorder capture after deploying all of this is needed to confirm real-world timing now approaches the ≤15s target — this sandbox cannot reproduce production network/server conditions, same limitation as every round so far.

## 9. The remaining bottleneck was origin-server request serialization, not the frontend

A fourth HAR (`~/Downloads/lr-perf-debug/v4`) confirmed §8's fixes are live in production: no `dashboard/refresh` call appears anywhere, and the echarts chunk loads in parallel with (not blocking) `GET /api/v1/dashboard`. But the user still measured it as not snappy. A full timing/header decomposition of this HAR — cross-checked against `cmd/ui.go`, the tracked nginx config, and `webpack.config.js` — found the bottleneck had moved almost entirely off the frontend and onto how the origin serves requests under concurrency.

**Finding A: nginx was doing on-the-fly gzip compression on every request, and it serialized entire request bursts behind it — static files and tiny API JSON responses alike.** Evidence: protocol is confirmed HTTP/2 on every request (so the browser's 6-connections-per-host cap doesn't apply), client-side `blocked` time was near-zero for a 9-request static-asset burst (the browser wasn't rate-limiting), and `wait` (TTFB) for isolated requests was a healthy ~250-280ms baseline — but `wait` ballooned to **1.3s-4.4s** for every request fired as part of a same-tick burst of 9-19 concurrent requests, including 0.1-2.4KB JSON responses with no legitimate reason to be slow. This was bit-for-bit reproducible between the pre-login and post-login page loads for the same files — a deterministic server-side cost, not network jitter. Confirmed by reading `cmd/ui.go`: it served static files via plain `http.FileServer`/`http.ServeContent`, which never compress — so the `Content-Encoding: gzip` seen on every response (including tiny API JSON from the separate Go API backend) was nginx compressing dynamically, per request, competing for CPU/worker capacity and serializing the whole burst behind it.

**Finding B: hashed static assets carried zero cache headers.** `main.<hash>.js`, `vendor-framework.<hash>.js`, etc. all came back with no `Cache-Control`, no `ETag`, no `Last-Modified`, despite being content-hashed and 100% safe to cache forever. Root cause, confirmed in `cmd/ui.go`: files come from a `//go:embed ui/dist/*` filesystem, and embedded files report a zero mtime — Go's `http.ServeContent` only emits `Last-Modified` when it has a real one, so these headers were silently never emitted, and nobody set `Cache-Control` anywhere in the chain either. Combined with the Hexmos OAuth flow forcing a real full-page navigation back to `livereview.hexmos.com/` (confirmed: the entire ~700KB framework/vendor JS manifest was re-requested byte-for-byte on landing back, identical to the very first load), every login round-trip re-downloaded the whole shared bundle from network — this should be a zero-cost cache hit.

**Fixed (both A and B together) in `cmd/ui.go`:** at UI-server startup, walk the embedded dist filesystem once and gzip-compress every compressible file (`.js`/`.css`/`.svg`/`.json`, skipping already-compressed formats and the `public/` subtree) into an in-memory map (`buildCompressedAssets`). The request handler now serves pre-compressed bytes directly for any client advertising `Accept-Encoding: gzip` (`serveCompressedAsset`) — compression happens once at boot, never on the request path. Every static response (compressed or not, via `setCacheHeaders`) now gets `Cache-Control: public, max-age=31536000, immutable` if its filename matches webpack's content-hash pattern (`\.[0-9a-f]{8,20}\.(js|css)$`), or `no-cache` otherwise (unhashed `/assets/*` logos, `index.html` — these can change without a filename bump, so must not be cached long-lived). Also added `gzip off;` to the UI `location /` block in `docs/deployment/livereview.hexmos.com`, since the upstream now sends already-gzipped responses and nginx would otherwise (harmlessly, but wastefully) attempt to re-detect/skip re-compression per request — making the intent explicit.

**Verified locally** (built the actual `livereview` binary against the current `ui/dist` production build output and curl-tested it directly, since this sandbox has no access to the production DB/API but the UI static-serving path is fully self-contained):
- A hashed JS file (`main.<hash>.js`) with `Accept-Encoding: gzip`: `Cache-Control: public, max-age=31536000, immutable`, `Content-Encoding: gzip`, and the gunzip'd response body is byte-identical to the original uncompressed file.
- An unhashed asset (`favicon.svg`): `Cache-Control: no-cache` (correctly *not* immutable-cached).
- SPA fallback routes (`/dashboard`, `/`) still correctly serve `index.html`.
- A client without `Accept-Encoding: gzip`: still gets `200` with the correct `Cache-Control`, served uncompressed via the original `fileServer.ServeHTTP` fallback path (content-length matches the original file size exactly).
- `go build .` clean, `gofmt` clean.

**Finding C: `/api/v1/system/info` fired twice on every dashboard mount.** Traced to two independent call sites: `Navbar.tsx` used the shared, react-query-backed `useSystemInfo()` hook (dedup'd, cached), but `URLMismatchBanner.tsx` (always mounted in the authenticated shell) did its own raw, uncached `apiClient.get('/system/info')` in a `useEffect`, bypassing the shared cache entirely — both mounted at the same instant. **Fixed:** exported `SYSTEM_INFO_QUERY_KEY` from `useSystemInfo.ts`; `URLMismatchBanner.tsx` now uses `useQuery` with that same key (own richer TS type, same underlying `/system/info` endpoint) instead of a raw effect-based fetch, so the two hooks share one cached request regardless of which mounts first.

**Finding D: multiple visually-distinct full-screen loaders still stacked in sequence.** Traced in `App.tsx`: the static `#lr-boot` HTML overlay, then a Redux `Auth.isLoading` → "Loading LiveReview..." full-screen spinner (custom inline SVG spinner) while `fetchUser()` resolves, then a *visually different* `<Suspense fallback={<RouteFallback />}>` → "Loading…" spinner (different spinner markup/border style) while the Dashboard route chunk downloads. Each individually made sense, but two different spinner designs firing back-to-back read as "loaders flickering." **Fixed:** extracted one shared `FullScreenLoader` component (the `RouteFallback` markup, parameterized by text) and used it for both the `isLoading` gate and `RouteFallback`, so the two now render the identical visual and differ only in their label text — reading as one continuous loading screen instead of a design swap.

**Verified:** `npx tsc --noEmit` clean; production webpack build clean (same pre-existing size warnings as prior rounds — large single-library files like echarts/moment-timezone/vega-lite/jspdf, expected and previously analyzed, not a regression).

**Deliberately not implemented this round:** the plain login screen still pays the full app-shell bundle cost (~700KB of `vendor-framework`/`main.js`/`main.css`/vendor-misc-* chunks) before it can render a "Sign in with Hexmos" button — `App.tsx` (Redux store, react-query client, router) is one monolithic entry point regardless of auth state. `Login` itself is already `React.lazy()`-loaded, so this isn't a missed easy win, it's an inherent SPA-architecture cost. Since that 700KB should now transfer in well under a second (pre-compressed + cacheable) instead of ~5.5s, this may no longer be a real problem in practice — flagged to reassess after this round's fixes are deployed and measured, rather than restructuring the entry point speculatively.

**Not yet verified:** requires deploying the rebuilt `livereview` binary (`go build .`, restart the `livereview-ui` pm2 process) and reloading nginx with the updated config — both need to happen on the production host, which I can't do from here. A fifth HAR + Recorder capture after deploying is needed to confirm: hashed assets carry the new headers in production, a repeat page load (the post-OAuth-redirect reload) hits browser cache with zero network transfer, burst `wait` times drop to the ~250ms baseline, only one `/api/v1/system/info` call appears, and total time from OTP submit to a populated dashboard drops meaningfully toward the ≤10s login / ≤5s render target.

## 10. The dominant remaining cost was TCP throughput, not application code — plus brotli and a real skeleton animation

Round 5's fixes shipped and were confirmed live (checked response headers directly against prod before touching anything further). But a fifth HAR (`~/Downloads/lr-perf-debug/v5`) still showed a slow first load. Re-reading it correctly this time — using `_transferSize` (actual wire bytes) instead of `content.size` (decompressed size, a mistake in earlier rounds' quick reads) — showed the app is not shipping an unreasonable number of bytes: **~202KB** before the login button renders, **~669KB** before the first chart appears. Yet these transfers were taking 5-10+ seconds each.

Rather than keep guessing at application code, I measured directly against the production box (`ssh master`):
- A single 75.6KB file, downloaded from an independent sandbox with zero other load: **2-3.5 seconds** (~20-30KB/s).
- The same file over **loopback on the server itself**: instant (~100MB/s) — rules out the Go binary or nginx application logic as the cause.
- The server's own **outbound transfer to an unrelated external host** (10MB, ~14.6s): reached ~700KB/s — proved the network path has real capacity, it just needs time to ramp up.
- **4 parallel connections to the same file**: aggregate throughput barely 2x a single connection, each individually slower — consistent with every new TCP flow independently stuck in slow-start, not a simple bandwidth cap.
- Root cause: the server was running **`cubic`** (Linux default) with `fq_codel`; `tcp_bbr` was compiled and present as a kernel module but never loaded. CUBIC is well-documented to underperform badly on exactly this workload — many short-lived HTTPS flows (one page load's worth of JS/CSS/API requests) over any real-world RTT, never leaving slow-start before the transfer completes.

**Fixed:** enabled BBR + `fq` qdisc on `master` via `ssh` (`modprobe tcp_bbr`, persisted via `/etc/modules-load.d/bbr.conf`, `net.ipv4.tcp_congestion_control=bbr` + `net.core.default_qdisc=fq` persisted via `/etc/sysctl.d/99-bbr.conf`, applied live with no restart needed). This is a systemwide change — the box hosts many `hexmos.com` properties, not just LiveReview — applied only after explicit confirmation given that broader blast radius. **Result: real but modest** — repeated single-file throughput tests went from ~20-33KB/s to ~34-46KB/s, roughly 30-50% faster, not the dramatic multi-x win the diagnostic signature suggested. Kept it (purely reversible, zero downside, verified `hexmos.com`/`lm.hexmos.com`/the LiveReview API all stayed healthy through the change), but the honest read is that a meaningful chunk of this connection's slowness is likely closer to a genuine bandwidth ceiling on this path than a pure slow-start/algorithm problem — application-level byte reduction still matters a lot on top of this, not as a fallback.

**Fixed: Brotli compression alongside gzip** (`cmd/ui.go`). The browser already advertises `Accept-Encoding: gzip, deflate, br, zstd` in every request. `buildCompressedAssets` now produces both a gzip and a brotli payload per file at startup (`compressedAsset{gzipData, brotliData}`); `serveCompressedAsset` prefers brotli when the client supports it, falling back to gzip, falling back to the uncompressed path exactly as round 5. Measured ~15-17% smaller than gzip for the same files (e.g. one hashed JS file: 55,321 bytes gzip vs. 45,685 bytes brotli at quality 11). Content verified byte-identical after decompression (Python's `brotli` module, since the `brotli` CLI wasn't available locally).

**Follow-up fix, found during verification, not in the original plan:** brotli at `BestCompression` (quality 11) made `buildCompressedAssets` slow enough on this build's largest bundled libraries (`moment-timezone` ~714KB raw, `echarts` ~624KB) that startup took **8.3 seconds** before the HTTP listener even opened — since compression ran synchronously before `net.Listen`/`server.Serve`. That's 8+ seconds of real downtime on every `pm2 restart`, a regression this round would have quietly introduced. Fixed two ways: (1) parallelized the compression loop across a worker pool sized to `GOMAXPROCS` (previously fully sequential in the `fs.WalkDir` callback), and (2) benchmarked brotli quality levels directly on the largest file — quality 11 took 512ms and produced 26,035 bytes; quality 10 took 105ms (~5x faster) and produced 26,751 bytes (~2.7% larger). Switched to quality 10. Combined, startup dropped from 8.3s to **2.1s** measured with `GOMAXPROCS=2` (matching the production box's actual 2 cores), while keeping nearly all of brotli's byte-size win.

**Fixed: skeleton shimmer instead of opacity-pulse** (`ui/src/components/Dashboard/widgets/ChartSkeleton.tsx`, `ui/src/styles/custom.css`). The "looks stuck" complaint persisted across rounds even though `ChartSkeleton` already used Tailwind's `animate-pulse` — a slow, uniform opacity fade, apparently too subtle to read as "in progress." Replaced with a shimmer sweep: a `skeleton-shimmer` class + `@keyframes shimmer` (added to `custom.css` alongside the existing `fadeIn`/`slideUp` custom animations) that moves a lighter gradient band left-to-right via `background-position` — the standard, unambiguous skeleton-loader pattern. Confirmed compiled into the production CSS bundle.

**Investigated, deliberately not implemented: deferring the post-login API-call burst.** Traced all 9 small calls that fire in the same tick as the heavy JS chunks right after login (`quota/status`, `billing/status`, `billing/usage/me`, `billing/usage/members`, `billing/upgrade/request-status`, `system/info`, `production-url`, `license/status`, `auth/me`) to their source: 5 of them are fired by `Navbar.tsx` (always-mounted, for a nav-bar usage/quota chip) the instant `currentOrg?.id` resolves, and are *shared by queryKey* with `Dashboard.tsx`, `NewReview.tsx`, `TeamCheckout.tsx`, and `SubscriptionTab.tsx` — Dashboard's own quota-exceeded banners depend on the same data, so it's not purely decorative Navbar chrome. Deferring it cleanly would mean coordinating the same delay across 5 independent consumers of a shared cache key (real risk of a race where one consumer fires immediately anyway, defeating the point) for a combined payload under 10KB — not worth the risk now that Steps 1-2 address the actual dominant cost (large JS chunk transfer time), which these tiny calls were never a meaningful part of.

**Also investigated, confirmed correct to leave alone: the pre-login bundle.** `vendor-framework` (react + react-dom + react-router + redux + react-redux + @reduxjs) is genuinely required to render anything, including `Login` (which is already its own `React.lazy()` chunk, not bundled into `main.js`). Not pursuing a separate login-only entry point — high effort/risk for an expected remaining win of ~1-2s now that BBR+brotli are in place.

**Verified:** `npx tsc --noEmit` clean; production webpack build clean (same pre-existing size warnings, not a regression); `go build .`/`go vet` clean; local curl verification of brotli headers/content-integrity/fallback behavior (gzip-only client, both-encodings client preferring brotli) against the rebuilt binary, same pattern as round 5; startup-time measurement before/after the worker-pool + quality-10 fix; shimmer CSS confirmed present in the compiled production bundle.

**Not yet verified:** Step 2 (brotli + faster startup) needs `make raw-deploy` to reach production (BBR, Step 1, is already live — applied directly via SSH during this round). A sixth HAR after deploying is needed to see whether per-file transfer times and total time-to-first-chart improve meaningfully now that the dominant infra-level and byte-size levers are both addressed.

## 11. Postmortem: the brotli/parallelization fix above still caused a real 502 window on deploy

After `make raw-deploy` shipped round 6's changes, the user hit a `502 Bad Gateway`. Investigated via `ssh master` + `pm2 list`: all 4 processes were "online" with normal (cumulative, not a live crash loop) restart counts and exit_code 0 — the site had already recovered by the time I checked (confirmed via 5 consecutive `curl` checks, all 200, and direct localhost checks against both `livereview-ui` and `livereview-api`).

Root cause: `buildCompressedAssets` ran **synchronously before `net.Listen`**, so the ~2.1s startup compression cost from §10 meant the port didn't accept connections for ~2 seconds on every restart. `livereview-ui` runs in pm2 **fork mode** (single instance, not cluster), so `pm2 reload` cannot be truly zero-downtime for it — there's an inherent old-process-dies/new-process-starts gap, and my change widened that gap by ~2 seconds. The user's 502 was very likely someone (possibly the user themselves, testing right after triggering the deploy) hitting that exact window.

**Fixed:** moved `net.Listen`/`server.Serve` to no longer wait on compression at all. `buildCompressedAssets` now runs in a background goroutine; the compressed-asset map is held behind `atomic.Pointer[map[string]compressedAsset]`, starting nil. The request handler checks the pointer and, if not yet populated, **transparently falls through to the existing uncompressed `fileServer.ServeHTTP` path** — the same fallback already used and verified in round 5 for clients that don't advertise gzip/brotli support. No new code path, no partial-cache correctness risk (the swap is atomic and all-at-once, not per-file): a request that arrives during the startup window just gets a (still correct, just larger) uncompressed response instead of a 502, and once the background goroutine finishes, all subsequent requests get pre-compressed responses instantly, same as before. This closes the deploy-time gap entirely rather than continuing to just shrink it.

**Verified:** local build/curl confirms the listener now accepts and correctly serves requests immediately after process start, well before the "Pre-compressed N static assets" log line appears; `go build .`/`go vet` clean.

**Not yet verified:** needs another `make raw-deploy` to actually confirm no 502 window on the next restart — this fix responds to a real production incident but wasn't deployed at the time of writing.

## 12. Chart entrance animation was playing twice

User report: every echarts widget's draw-in animation (bars growing, lines/sankey/sunburst tracing in, etc.) visibly played twice on load instead of once.

Root cause: `DashboardGrid.tsx` measures its own width via a `ResizeObserver` (`gridWidth` state, used to size `react-grid-layout`'s `Responsive` component) — this genuinely fires more than once during initial layout (an immediate report when observation starts, then again as the page settles). Each firing re-renders `DashboardGrid`, which re-renders its 4 nested context providers (`DashboardPeriodProvider`, `ReviewLayersProvider`, `SystemOverviewProvider`, `PeopleProvider`, all mounted in `DashboardGrid.tsx`). All 4 built their `value` object as a plain object literal on every render, not memoized — so even though the *underlying data* (`reviewLayers`, `systemOverview`, `people`, `period`) hadn't actually changed, each re-render produced a *new* `value` object reference, which per React's Context semantics re-renders every consumer regardless of whether the data inside actually changed. Every one of the 7 echarts widgets consumes one of these contexts, and none of them memoized their own `option` object either — so each spurious re-render rebuilt `option` fresh and passed a new object reference to `ReactEChartsCore`. Combined with the `notMerge` prop already set on every widget's `<ReactECharts>` (present from when these were first built, to avoid stale-series artifacts on real data changes), a new `option` reference means echarts fully redraws from scratch, including its entrance animation — which is exactly what "twice" looked like.

**Fixed** in both layers, so the fix holds regardless of what specifically triggers a given widget's re-render:
- **4 context providers** (`DashboardPeriod.tsx`, `ReviewLayersData.tsx`, `SystemOverviewData.tsx`, `PeopleData.tsx`): wrapped each `value` object in `useMemo`, keyed on the actual underlying data (`data?.review_layers`/`data?.system_overview`/`data?.people`, `isLoading`, `error`, `refetch`, or `period` for the period provider) instead of recreating it every render.
- **7 echarts widgets** (`ReviewPipelineSankey`, `IssueCategoryRadar`, `ReviewVolumeBar`, `RepoHierarchySunburst`, `CoverageGauge`, `UsageShareDonut`, `ContributionCalendarHeatmap`): wrapped each `option` (and `sunburstOption`/`treemapOption` for the one widget with two view modes) in `useMemo`, keyed on the widget's actual data inputs rather than the derived-every-render intermediate values. Where a widget has early returns for loading/empty states *before* its original option computation, the computation was moved *above* those early returns (with internal guards for the not-yet-loaded case) so the `useMemo` hook call stays unconditional, matching React's rules of hooks — no behavior change to what's rendered, just reordering so the hook always runs.

**Verified:** `npx tsc --noEmit` clean; production webpack build clean (same pre-existing size warnings, not a regression); headless Chrome smoke test confirms the app still boots with no console errors.

**Not yet verified:** needs a real login + dashboard load to visually confirm the animation now plays exactly once — this sandbox has no backend to log into and load real dashboard data against, so the fix is verified structurally (the mechanism that caused the double-render is provably closed) but not yet observed end-to-end.
