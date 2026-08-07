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
   (`internal/blobstore`, default: local filesystem; optionally S3-compatible, covering
   both real AWS S3 and Backblaze B2) under key `org/:org_id/review/:review_id/artifacts/
   :artifact_type.json`, and serves it back via
   `GET /api/v1/diff-review/:review_id/artifacts/:artifact_type` (404 when absent). See
   `internal/api/diff_review.go`'s `getBlobBucket`/`PutDiffReviewArtifact`/
   `GetDiffReviewArtifact`. The storage backend itself is admin-configurable at runtime
   from Settings → Storage (`internal/api/storage_settings.go`, backed by a
   `system_settings` row named `blob_storage`, read fresh on every artifact request — no
   redeploy needed to switch backends or rotate credentials.

Adding a new artifact type is just a new entry in `diffReviewArtifactTypes` plus a
frontend renderer — no new tables, no new endpoint code, and it lands in whichever blob
store is already configured.


