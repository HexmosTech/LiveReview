## Secure Scoping & Tenant Isolation

### Core Philosophy

We maintain an absolute, unbreakable boundary between tenants (organizations) and user roles. A single leak or scoping mistake is not just a bug—it is a critical security failure. Every line of code we write must actively enforce this boundary, making security and isolation automatic rather than an afterthought.

### Scoping Layers & Access Hierarchies

#### Org-Level Isolation

Every resource in LiveReview—whether it is a review, an API key, a repository config, or billing status—belongs strictly to an organization. Cross-tenant data access is strictly forbidden.

- **Direct Context Filtering**: Every database query MUST explicitly filter by `org_id` resolved directly from the authenticated request context (e.g., using `PermissionContext`).
- **ID Scoping Guardrails**: Never trust resource IDs from path parameters (like `/reviews/:id`) blindly. You must always confirm the resource belongs to the user's active `org_id` before processing or returning it.
- **No Global Fallbacks**: Never write global queries that omit `org_id` filters, unless the resource is globally public (e.g. system configs).

#### Role-Level Scoping

Users are assigned specific roles (`super_admin`, `owner`, `member`) within an organization, each with clear boundaries of authorization.

##### Super Admin
Gated globally by `authMiddleware.RequireSuperAdmin()`. Super Admins can access all Owner and Member endpoints, plus:
- `GET /api/v1/admin/users` ➔ `s.userHandlers.ListAllUsers`
- `POST /api/v1/admin/orgs/:org_id/users` ➔ `s.userHandlers.CreateUserInAnyOrg`
- `PUT /api/v1/admin/users/:user_id/org` ➔ `s.userHandlers.TransferUserToOrg`
- `GET /api/v1/admin/analytics/users` ➔ `s.userHandlers.GetUserAnalytics`
- `DELETE /api/v1/admin/organizations/:org_id` ➔ `s.orgHandlers.DeactivateOrganization`

##### Organization Owner
Allowed full administrative controls within their organization. Can access all Member endpoints, plus:
- **User Management**:
  - `POST /api/v1/orgs/:org_id/users` ➔ `s.userHandlers.CreateUser`
  - `PUT /api/v1/orgs/:org_id/users/:user_id` ➔ `s.userHandlers.UpdateUser`
  - `DELETE /api/v1/orgs/:org_id/users/:user_id` ➔ `s.userHandlers.DeactivateUser`
  - `PUT /api/v1/orgs/:org_id/users/:user_id/role` ➔ `s.userHandlers.ChangeUserRole`
  - `POST /api/v1/orgs/:org_id/users/:user_id/force-password-reset` ➔ `s.userHandlers.ForcePasswordReset`
- **Org Management**:
  - `PUT /api/v1/orgs/:org_id` ➔ `s.orgHandlers.UpdateOrganization`
  - `PUT /api/v1/orgs/:org_id/members/:user_id/role` ➔ `s.orgHandlers.ChangeUserRole`
- **API Key Management**:
  - `POST /api/v1/orgs/:org_id/api-keys` ➔ `s.CreateAPIKeyHandler`
  - `GET /api/v1/orgs/:org_id/api-keys` ➔ `s.ListAPIKeysHandler`
  - `POST /api/v1/orgs/:org_id/api-keys/:id/revoke` ➔ `s.RevokeAPIKeyHandler`
  - `DELETE /api/v1/orgs/:org_id/api-keys/:id` ➔ `s.DeleteAPIKeyHandler`
- **Subscriptions & Billing**:
  - `POST /api/v1/subscriptions` ➔ `subscriptionsHandler.CreateSubscription`
  - `POST /api/v1/subscriptions/confirm-purchase` ➔ `subscriptionsHandler.ConfirmPurchase`
- **Learnings**:
  - `POST /api/v1/learnings` ➔ `learningsHandler.Upsert`
  - `PUT /api/v1/learnings/:id` ➔ `learningsHandler.Update`
  - `DELETE /api/v1/learnings/:id` ➔ `learningsHandler.Delete`

##### Organization Member
Restricted strictly to read-only views and review execution.
- **User Browsing**:
  - `GET /api/v1/orgs/:org_id/users` ➔ `s.orgHandlers.GetOrganizationMembers`
  - `GET /api/v1/orgs/:org_id/users/:user_id` ➔ `s.userHandlers.GetUser`
  - `GET /api/v1/orgs/:org_id/users/:user_id/audit-log` ➔ `s.userHandlers.GetUserAuditLog`
- **Org & Members**:
  - `GET /api/v1/organizations` ➔ `s.orgHandlers.GetUserOrganizations`
  - `GET /api/v1/organizations/:org_id` ➔ `s.orgHandlers.GetOrganization`
  - `GET /api/v1/orgs/:org_id/members` ➔ `s.orgHandlers.GetOrganizationMembers`
  - `GET /api/v1/orgs/:org_id/analytics` ➔ `s.orgHandlers.GetOrganizationAnalytics`
- **Reviews**:
  - `POST /api/v1/reviews` ➔ `s.createReview`
  - `GET /api/v1/reviews` ➔ `s.getReviews`
  - `GET /api/v1/reviews/:id` ➔ `s.getReviewByID`
  - `GET /api/v1/reviews/:id/events` ➔ `reviewEventsHandler.GetReviewEvents`
  - `GET /api/v1/reviews/:id/summary` ➔ `reviewEventsHandler.GetReviewSummary`
  - `GET /api/v1/reviews/:id/accounting` ➔ `reviewEventsHandler.GetReviewAccounting`
- **Learnings**:
  - `GET /api/v1/learnings` ➔ `learningsHandler.List`
  - `GET /api/v1/learnings/:id` ➔ `learningsHandler.Get`
- **Subscriptions**:
  - `GET /api/v1/subscriptions/:id` ➔ `subscriptionsHandler.GetSubscription`
  - `GET /api/v1/subscriptions/current` ➔ `subscriptionsHandler.GetCurrentSubscription`

#### Dynamic Role Checks & Middlewares

Rather than caching roles inside static tokens or sessions, LiveReview performs **Dynamic Role Checks** against the database on every request to ensure role updates and revocations react immediately.

To enforce this, all org-scoped endpoints MUST run through the standard **Echo Middleware Chain** in `server.go` to construct the `PermissionContext`:

1. **`authMiddleware.RequireAuth()` (or `RequireAuthOrAPIKey()`)**:
   Validates the user's Bearer JWT session token or `X-API-Key` header and registers the user model in the context.
2. **`authMiddleware.BuildOrgContext()` (or `BuildOrgContextFromHeader()`)**:
   Resolves the target `org_id` (either from the URL path parameter `:org_id` or the `X-Org-Context` header) and registers it in the request context.
3. **`authMiddleware.ValidateOrgAccess()`**:
   Hits the database to confirm the authenticated user is currently an active member of that specific organization. It retrieves their live role dynamically and registers it in `user_role`.
4. **`authMiddleware.BuildPermissionContext()`**:
   Constructs the full `PermissionContext` object and places it in the echo context under `permission_context`. 

#### API Key Scoping

API keys represent programmatic machine access and must follow a **strict least-privilege boundary** relative to the user who generated them.

- **Inherited Limits**: An API key automatically inherits the exact access boundaries of its creator. A key created by a `member` cannot perform `owner` actions.

- **Sensitive Operations Gate**: API keys are strictly for automation. Highly sensitive account changes (e.g. changing passwords, updating user emails, or deactivating members) are strictly prohibited via API keys and require an active user session (JWT).

### Security & Scoping Guardrails

Before writing any new endpoint, making database changes, or updating routing, check off the following rules:

1. **Explicit Scoping in Handlers** 
    Every endpoint that accesses organizational data (reviews, settings, members) MUST fetch `org_id` exclusively from `PermissionContext` (or equivalent verified request context). Do not query resources using client-supplied IDs without verifying ownership first.
    
2. **Strict Middleware Chains** 
    Always apply the Echo middleware chain (`BuildOrgContext`, `ValidateOrgAccess`, `BuildPermissionContext`) to any tenant-scoped routes. Do not bypass this chain under any circumstance.
    
3. **Session-Only Gating** 
    Endpoints that perform destructive actions, credential changes, or billing subscription alterations MUST reject API keys. Gating should explicitly check for JWT authentication.


## Porting from git-lrc

`git-lrc` (sibling repo, typically checked out at `../git-lrc`) is where LiveReview's
local CLI (`lrc`/`git-lrc`) lives, including a mature local review UI
(`internal/staticserve/static/`, Preact/htm, buildless) and a self-contained blast-radius
scoring engine (`blastradius/`, its own Go module `github.com/HexmosTech/blastradius`).
Capabilities built there sometimes need a corresponding home in LiveReview's hosted
review-details page (`ui/src/pages/Reviews/ReviewDetail.tsx`). See
`/home/shrsv/.claude/plans/piped-imagining-sky.md` for the design of the first port
(diff/findings viewer + blast radius).

### Porting convention

Any LiveReview file ported from a git-lrc source must carry a one-line header comment:

```
// Ported from git-lrc:<path>#L<start>-L<end> (as of <short-sha>)
```

This makes future re-syncs diffable: check the cited git-lrc path/commit against
git-lrc's current `HEAD` to see what changed upstream since the port, without having to
rediscover which LiveReview files came from where.

Because git-lrc's review UI is buildless Preact/htm/plain-CSS and LiveReview's is React
19 + Redux + Tailwind + `UIPrimitives.tsx`, ports are **not** file copies — treat
git-lrc's components as the functional spec (especially framework-agnostic pure-logic
`.mjs` files, which port ~1:1) and rebuild presentational components natively against
LiveReview's design system.

### Artifact sync channel

git-lrc's CLI computes some things locally that the LiveReview server has no way to
compute itself (e.g. blast radius requires a live `codebase-memory-mcp` graph index of
the repo, which only exists on the developer's machine). These sync to LiveReview
**opportunistically** — only reviews actually run through `git lrc review` will have
them; webhook- and web-UI-triggered reviews won't, and that's expected, not an error.

The reusable pattern any future git-lrc-computed artifact should follow:

1. CLI computes the artifact locally after (or alongside) submitting the review.
2. CLI POSTs it to `POST /api/v1/diff-review/:review_id/artifacts/:artifact_type`
   (fire-and-forget — log a warning on failure, never block or fail the review).
3. LiveReview writes the raw JSON body to whatever blob store is currently configured
   (`internal/blobstore`, default: local filesystem; optionally S3-compatible covering
   both real AWS S3 and Backblaze B2, Google Cloud Storage, or Azure Blob Storage) under
   key `org/:org_id/review/:review_id/artifacts/:artifact_type.json`, and serves it back
   via
   `GET /api/v1/diff-review/:review_id/artifacts/:artifact_type` (404 when absent). See
   `internal/api/diff_review.go`'s `getBlobBucket`/`PutDiffReviewArtifact`/
   `GetDiffReviewArtifact`. The storage backend itself is admin-configurable at runtime
   from Settings → Storage (`internal/api/storage_settings.go`, backed by a
   `system_settings` row named `blob_storage`, read fresh on every artifact request — no
   redeploy needed to switch backends or rotate credentials.

Adding a new artifact type is just a new entry in `diffReviewArtifactTypes` plus a
frontend renderer — no new tables, no new endpoint code, and it lands in whichever blob
store is already configured.

## Keeping the Navigation Mega Menu in Sync

The nav mega menu (`ui/src/components/Navbar/NavMegaMenu.tsx` + its data source
`ui/src/components/Navbar/megaMenuData.ts`) is the primary way users discover and reach
every section of the app. Whenever you add something new — a new page, route, settings
tab, or any feature section that a user should be able to navigate to — you MUST ensure it
is also reflected in the mega menu.

**Rule: no new section/page/tab ships without a corresponding mega-menu entry.**

- For a new top-level area, add a `MegaMenuSection` to `buildMegaMenuSections()` in
  `megaMenuData.ts`.
- For a new sub-page or settings tab under an existing area, add a `link(...)` (and, if
  grouped, a `group(...)`) node to the relevant section's `items` array. Reuse the same
  route the page actually lives on (e.g. `/settings#storage` for a settings tab) and the
  appropriate `Icons.*` glyph, and gate it with the matching `isVisible` /
  `requiresOwnerOrAdmin` / `requiresSuperAdmin` predicate.
- New settings tabs are typically registered in
  `ui/src/pages/Settings/Settings.tsx` (the `tabs` array, by permission) — make the mega
  menu entry mirror that same gating so both stay consistent.

If a new feature is not navigable from the mega menu (e.g. it is only reached from a
button inside an existing page), call that out explicitly rather than silently skipping
the entry. Keeping the mega menu complete is what makes new capabilities discoverable.

## Keeping `config/*-ssl.conf.example` in Sync with `lrops.sh`

`lrops.sh` is a self-contained installer script (curl-piped and run standalone
on remote servers), so it embeds its reverse-proxy templates inline — the
nginx one under `# === DATA:nginx.conf.example ===` in `lrops.sh`, uncommented
and filled in at runtime by `setup-ssl`'s `render_nginx_conf()`. Nobody reads
that embedded, commented-out template comfortably; `config/nginx-ssl.conf.example`
exists purely as a static, readable reference copy of what `lrops.sh setup-ssl
<domain>` actually produces on disk (for `livereview.example.com`) — it is
**not** read by `lrops.sh` itself.

**Rule: whenever a change to `lrops.sh` touches the nginx template
(`DATA:nginx.conf.example`) or `render_nginx_conf()`'s transform logic, regenerate
`config/nginx-ssl.conf.example` in the same change**, by actually running the
same `sed` transforms `render_nginx_conf()` runs (domain substitution,
HTTPS-block uncomment, `HTTP_ROUTES` marker replacement) against the updated
template — don't hand-edit the reference file, and don't guess at the output.
Verify the result (brace-balance, `server{}` block count, and ideally an actual
`nginx -t` / live-nginx check for anything touching the SSL/redirect logic) the
same way before trusting it.

The same rule applies to Caddy and Apache (`DATA:caddy.conf.example`,
`DATA:apache.conf.example`, and their respective `setup-ssl.sh` render
functions) once equivalent `config/caddy-ssl.conf.example` /
`config/apache-ssl.conf.example` reference files exist — they don't yet, and
creating them is a separate task, not implied by this rule.

## Route Documentation for RAG (`internal/docindex/docs/routes_guide/`)

`internal/docindex/docs/routes_guide/` holds one Markdown file per UI
route/page (mirroring `ui/src/pages/`, grouped into subfolders that match
the top-level route groups in `ui/src/App.tsx`: `reviews/`, `explore/`,
`git/`, `ai/`, `settings/`, `licenses/`, `reports/`, `chatbot/`, `auth/`).
It exists to train/feed the in-app chatbot (Livi) so it can answer both
data questions and "how do I do X in the UI" / navigation questions. It is
tracked directly in git (not fetched/generated) and embedded via
`//go:embed docs` in `internal/docindex/docs.go`. See
`internal/docindex/docs/routes_guide/README.md` for the file structure this
folder follows.

**Rule: whenever you make a UI change, update the relevant `.md` file(s) in
this folder in the same change.**

- New route/page added → add a new file for it (and link it from its
  group's parent page / the relevant `README.md`/overview file).
- Route removed → delete its file, and remove dangling links to it from
  other files.
- Page behavior, key actions, or access/permission gating changed → update
  the file's "Key actions" / "Who can access it" section to match.
- No extra step needed after editing — these files are committed directly
  and picked up by `//go:embed docs` on the next build, same as any other
  Go source file. (This replaced an older, fetch-and-hash-based scheme; see
  `docs/docs-sources-pinning-plan.md` for why.)

## Keeping `scripts/docs_sources.env` in Sync

`internal/docindex/docs/{lr_wiki,lrc_wiki}/` (the rest of the chatbot's RAG
corpus, alongside `routes_guide/` above) is synced from `git-lrc`, its wiki,
and `LiveReview`'s wiki via `scripts/sync_docs_sources.sh`, pinned to exact
commit SHAs in `scripts/docs_sources.env`. GitHub is the source of truth,
not any one machine's local fetch — see `docs/docs-sources-pinning-plan.md`
for the full design.

**Rule: whenever you (as an agent) commit a documentation change to
`git-lrc/docs/`, `git-lrc`'s wiki, or `LiveReview`'s wiki, bump the matching
`*_COMMIT` line in `LiveReview/scripts/docs_sources.env` to the new commit
SHA in that same task** — not as a separate follow-up. `make
check-docs-sources` is a read-only backstop (reports/fails CI when a pin
has fallen behind) for drift introduced outside agent-driven edits, but it
is not a substitute for bumping the pin yourself when you're the one making
the docs change.

## Chat UI (/chat and /chat-debug) Must Stay In Sync

`/chat` (`ui/src/pages/Chatbot/Chatbot.tsx`) and `/chat-debug`
(`ui/src/pages/Chatbot/ChatDebugPage.tsx`) are two routes over **one** shared
component: `ui/src/pages/Chatbot/ChatConversation.tsx`. Both page files are
thin wrappers - `<ChatConversation surface="chat" />` and
`<ChatConversation surface="chat_debug" />` - with no rendering logic of
their own.

**The only intentional difference between the two surfaces is the
debug-artifacts button/dialog**, gated inside `ChatConversation` by
`surface === 'chat_debug'` (see the `showDebug` flag, `DebugTrigger`,
`DebugModal`). Everything else - message rendering, chart cards (granularity
toggle, stat chips, expand/download), file cards, the input box, loading
states, empty state, header - must render identically for both surfaces
because they share the same code path.

**Rule: never duplicate chat page logic.** If you need to change how a
message, chart, or file renders, or fix a loading/layout bug, change it in
`ChatConversation.tsx` once - it applies to both routes automatically. Do
not add page-specific rendering back into `Chatbot.tsx` or
`ChatDebugPage.tsx`, and do not fork `ChatConversation.tsx` into two copies.
A new surface-specific feature must be explicitly gated by the `surface`
prop, not implemented by branching the component in two.

## PromptBook & LawBook Conventions

The MCP agent's prompts follow a two-layer architecture:

- **LawBook** (`internal/mcpagent/alaws_livi/`): Numbered, citable rules in `.md` files with alaws frontmatter. Each section has a unique `id` (e.g. `livi.interpreting.schema`). Laws are the single source of truth — if a rule exists only in a PromptBook template and not as a law, it is a bug.

- **PromptBook** (`alaws_livi/prompts/`): Templates that assemble laws into system prompts for specific model calls. Templates use `{{ref:livi.section.id}}` to embed laws — they never duplicate rule text inline.

### Rules

1. Every instruction the model receives MUST be a numbered law in the lawbook, not inline text in a prompt template.
2. PromptBook templates use `{{ref:}}` exclusively — no hardcoded rule text.
3. When adding a new rule, create it as a law in the appropriate lawbook section, then `{{ref:}}` it from the prompt template.
4. After lawbook changes, rebuild with `make prep-dbctx` and verify with `go test ./internal/mcpagent/...` (specifically `TestLawbookCompiles`).
5. Test the interpret pipeline end-to-end with: `curl -s -X POST localhost:8080/api/v1/test-chat -H 'Content-Type: application/json' -d '{"message":"how many reviews do we have?"}' | jq .`

## UI Builds Require Explicit User Approval

`npm run build` in `ui/` (or any other full UI production build) is highly
resource-intensive and has been reported to crash the user's machine.

**Rule: never run a full UI build without the user's explicit, per-instance
approval.** A prior approval does not carry forward to later changes or
later sessions - ask again each time a full build seems warranted.

For everyday verification after frontend changes, use `npx tsc --noEmit`
(fast, cheap, catches type errors) instead. Only reach for a full build if
something genuinely requires it (e.g. confirming a bundler-only error or
asset output), and only after the user says yes.

## Production Safety & Feature Gating

### Development-Only Features

The following features are gated and MUST be disabled in production:

| Feature | Gate | Default | Notes |
|---------|------|---------|-------|
| `/test-chat` endpoint | Build tag `production` | Included in dev | Excluded in production builds |
| `/chat-debug` route | `LIVI_DEBUG_LOG` | `false` | Debug artifacts UI |

### Build Tags

| Tag | Purpose | Usage |
|-----|---------|-------|
| `vendor_prompts` | Encrypted prompts | Docker builds |
| `production` | Production safety | Excludes test endpoints like `/test-chat` |

### Docker Dependency Versions (NOT the LiveReview app version)

Every third-party base image and binary baked into the Docker image (node,
golang, debian base images; dbmate; River CLI/UI; `vl-convert`;
`codebase-memory-mcp`; `dbctx`; `alaws`) is version-pinned in a single file:
**`docker/docker-deps.env`**. This is a completely separate system from
LiveReview's own application version (`scripts/lrops.py version`, Git tags,
`make version*`) - don't confuse the two.

Full docs: `docker/DOCKER-DEPS.md`. The rules an agent must follow:

- **Never hardcode a version or download URL directly in `Dockerfile` or
  `Dockerfile.crosscompile`.** Every version lives in `docker/docker-deps.env`
  as `SOME_TOOL_VERSION=...`, referenced in both Dockerfiles via a matching
  `ARG SOME_TOOL_VERSION=<same default>`, and injected as `--build-arg` by
  `scripts/lrops.py` (`_load_docker_dep_versions`/`_docker_dep_build_args`)
  for every real build. If you're about to type a version string or a
  `releases/download/vX.Y.Z/...` URL straight into a Dockerfile, stop - add
  it to `docker/docker-deps.env` instead.

- **`docker buildx build --check` only lints Dockerfile syntax - it never
  executes any `RUN` step.** It will happily pass on a Dockerfile whose
  install commands are broken (wrong URL, incompatible version pins,
  network-dependent failures). It's a fast sanity check after editing a
  Dockerfile, not proof anything works. The only real proof is an actual
  build (`docker buildx build -f Dockerfile.crosscompile -t
  livereview:localtest --load .`) followed by `make verify-docker-deps`.

- **river and riverui must be built in separate temp Go modules, never
  one shared module.** They're independent release trains - `RIVERUI_VERSION`
  does not depend on the same underlying `github.com/riverqueue/river`
  version as `RIVER_VERSION`/`go.mod`, and should not be forced to. Building
  both via `go get` into one shared `go mod init` lets Go's
  minimum-version-selection unify their transitive deps and silently pick
  an incompatible mix, breaking the build with a confusing compile error
  deep in an unrelated package. This exact bug happened once already after
  pinning `RIVER_VERSION` to match `go.mod` - see `docker/DOCKER-DEPS.md`
  for the full story. Both Dockerfiles now use two separate temp modules;
  keep it that way.

- **Adding a brand-new tool/binary dependency to the Docker image** (do all
  four, and don't consider it done until a real local build + 
  `make verify-docker-deps` passes):
  1. Add `SOME_TOOL_VERSION=vX.Y.Z` to `docker/docker-deps.env` (pick a
     real, current release - check the tool's GitHub releases page or
     Docker Hub tags first, the same way `scripts/check_docker_deps.py`
     would resolve "latest" for it).
  2. In both `Dockerfile` and `Dockerfile.crosscompile`, add
     `ARG SOME_TOOL_VERSION=vX.Y.Z` (same default, for standalone
     `docker build` to still work) in the stage that installs it, and use
     `${SOME_TOOL_VERSION}` in the download/`go install`/`go get` command -
     never the literal version string.
  3. Add an entry to the `DOCKER_DEPS` list in `scripts/check_docker_deps.py`
     with a `checker` function that resolves "latest" for it (reuse
     `check_github_release()` for a GitHub-releases tool, or
     `check_dockerhub_semver()`/`check_dockerhub_dated()` for a Docker Hub
     base image) - this is what makes the new dependency show up in
     `make check-docker-deps` / `update-docker-deps` and the automatic
     pre-build check, instead of silently drifting unmanaged.
  4. Add the tool's `--version`/`--help` invocation to the `CHECKS` array in
     `scripts/verify_docker_deps.sh`, then build a local image and run
     `make verify-docker-deps` to confirm it's actually present and runnable
     - `check-docker-deps` only validates version *numbers*, it never
     builds or runs anything.

- **Locking a dependency's version** (so `update-docker-deps`/
  `update-docker-deps-yes`/the pre-build check never touch it): add its KEY
  to the comma-separated `PINNED_DOCKER_DEPS=` line in
  `docker/docker-deps.env`. Never delete that line - it must stay present
  even when empty.

- Run `python3 scripts/check_docker_deps.py --check` (or
  `make check-docker-deps`) after any change to `docker/docker-deps.env` or
  the Dockerfiles to confirm the new/changed entry resolves and both
  Dockerfiles still parse (`docker buildx build --check -f Dockerfile[.crosscompile] .`).

### Docker Production Checklist

Before releasing a new Docker image:

1. [ ] Verify `LIVI_DEBUG_LOG=false` in `.env.selfhosted`
2. [ ] Verify `/test-chat` excluded with `production` tag
3. [ ] Verify `/chat-debug` gated by `LIVI_DEBUG_LOG`
4. [ ] Test `make docker-multiarch-dry` output
5. [ ] Run `make check-docker-deps` - see `docker/DOCKER-DEPS.md`
6. [ ] Run `make verify-docker-deps` against a built image to confirm
   `vl-convert`, `dbctx`, `alaws`, `codebase-memory-mcp`, `dbmate`, `river`,
   `riverui` are all present and actually invokable, not just downloaded

### Raw Deploy Safety

`make raw-deploy` uses the `production` build tag to exclude dev-only endpoints.
The `/test-chat` endpoint (unauthenticated, hardcoded org access) is automatically
excluded in production builds.


