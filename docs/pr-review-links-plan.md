# PR Review Links — Implementation Plan

## Goal

When a new PR/MR is opened, LiveReview adds a controls section with two links to the top of its description:

1. **Start a review** — opens LiveReview, triggers a review for that PR, and redirects to the
   review details page.
2. **View review history** — opens the Reviews list filtered to that PR's reviews.

Policy-based auto-approval now has a **static stand-in**: reviews started from the link
are checked against "no critical findings" by the `pr_policy_check` job
(`internal/jobqueue/pr_policy_worker.go`), which posts a policy comment and approves the
PR, or requests changes (GitHub) / comments only (GitLab). Org-defined policies (likely
built on CI/CD Gates jq rulesets) remain a later feature.

## Decisions

| Topic | Decision |
|---|---|
| Providers | GitHub, GitLab, Bitbucket, Gitea, Azure DevOps |
| Which repos | Every repo with the LiveReview webhook installed |
| Which PRs | Only newly opened PRs (GitHub action `opened`, GitLab action `open`). Existing open PRs, reopen, push, and edit events are ignored. |
| Where links go | A controls section at the top of the PR description, between `[//]: # (livereview:controls)` and `[//]: # (livereview:controls-end)`, followed by a horizontal line (HTML comments show as text on Bitbucket). Added at PR open. A separate results section at the bottom, between `livereview:results` and `livereview:results-end`, is rewritten in place after each link-started review with a "LiveReview auto-approval decisions" table. The old single section (`livereview:links` / `livereview:end`, or the legacy `<!-- … -->` markers) is removed and replaced on the next review. The table (one row per review, latest first: Review, Time (starter's time zone, UTC fallback), Decision, Reason). The stage-by-stage policy breakdown is a separate PR comment. Replaced the earlier bot comment. |
| Duplicate webhooks | Guarded by a "links added" flag (`links_comment_posted_at`) on the `pull_requests` row |
| Auth | Both links require a LiveReview login. Logged-out users log in and are sent back to the link (existing `redirectAfterLogin` flow). |
| Org | The user is switched to the PR's org automatically. Non-members get "not found". |
| Trigger behavior | Opening the trigger link starts the review immediately (no confirm page, for now) |
| Repeat clicks | Each click starts a new review, **unless** a review for the PR is `created`/`in_progress` and started less than 10 minutes ago. Then it redirects to that review. |
| Existing triggers | Unchanged (reviewer assignment, @mentions) |
| History view | The existing Reviews list (`/reviews`) filtered by `pull_request_id` |

## Link format

The UI uses hash routing, so links look like:

```
<host>/#/pr/<pull_request_id>/review
<host>/#/reviews?pull_request_id=<pull_request_id>
```

- `<pull_request_id>` is LiveReview's internal `pull_requests.id` (primary key), **not** the
  provider PR number. It is unique across providers, repos, and orgs, so a GitHub #123 and a
  GitLab !123 never collide.
- `<host>` comes from the `livereview_prod_url` setting (the same public URL webhooks are
  installed against). If it is not set, the section is skipped with a logged warning.

## Section text (end of the PR description)

```
🔍 LiveReview
- ▶ Start a review: <host>/#/pr/<id>/review
- 📜 View review history: <host>/#/reviews?pull_request_id=<id>
```

## Backend changes

### 1. Migration — `db/migrations/<ts>_add_links_comment_posted_at_to_pull_requests.sql`

- Add `links_comment_posted_at TIMESTAMPTZ` (nullable) to `pull_requests`.
- Regenerate `db/schema.sql`.

### 2. Add the links section — `internal/jobqueue/repo_sync_worker.go` (`PRStateSyncWorker.Work`)

After `UpsertPullRequest` returns the PR row id:

1. Skip unless `event.Metadata["action"]` is `opened` (GitHub) or `open` (GitLab).
2. Claim the post atomically:
   `UPDATE pull_requests SET links_comment_posted_at = now() WHERE id = $1 AND links_comment_posted_at IS NULL`.
   If no row changed, the section was already added — skip.
3. Load the repo token via the existing `repositoryConnectorInfo` lookup.
4. Read the public URL via the webhook store's `GetWebhookPublicEndpoint` (inject the store
   into the worker in `jobqueue.go`).
5. Read the description, append the section if the marker is missing, and save it. On failure, reset `links_comment_posted_at` to `NULL` and return the
   error so River retries.

### 3. Description helpers

- `internal/providers/github/pr_comment.go` — `AppendToPRBody(ctx, baseURL, token, ownerRepo, number, marker, section)` (GET then PATCH the PR body),
  using the existing `apiBaseURL` (works for GitHub Enterprise).
- GitLab: `NewHTTPClient(baseURL, token).AppendToMRDescription(projectID, iid, marker, section)` (reuses `GetMergeRequest`, then PUTs `description`).

Not reusing `GitHubProvider.PostComment`: it hardcodes `api.github.com` and formats text as a
review finding.

### 4. Org lookup — `GET /api/v1/pull-requests/:id/org`

- Auth only (`RequireAuth`); the frontend doesn't know the PR's org yet, so the org
  middleware chain can't be used here (approved exception).
- Handler looks up `pull_requests.org_id` and verifies the user is an active member of that
  org. Returns `{ "org_id": ... }`, or **404** if the PR doesn't exist or the user isn't a
  member (no existence leak).
- Returns nothing besides the org id.

### 5. Reuse ongoing review — `internal/api/pull_request_review_trigger.go` (`createReview`)

- Add optional body field `reuse_ongoing` (bool, default false).
- When true: if a review exists for this `pull_request_id` + `org_id` with status
  `created`/`in_progress` and `created_at > now() - interval '10 minutes'`, return its id
  without creating a new review.
- Existing callers don't send the field, so their behavior is unchanged.
- The actual review still goes through `POST /api/v1/reviews` with the full middleware chain
  and LOC quota preflight.

### 6. Reviews list filter — `getReviews` in `internal/api/server.go`

- Add optional `pull_request_id` query param → `AND pull_request_id = $n` (still scoped by
  `org_id`).
- Add the field to the `ReviewsQuery` schema.

## Frontend changes

### 7. Trigger page — `ui/src/pages/Reviews/PullRequestReviewTrigger.tsx`

Route `/pr/:id/review` in `ui/src/App.tsx`. On load:

1. `GET /api/v1/pull-requests/:id/org`.
2. If the org differs from the current one, `switchToOrg(org_id)`.
3. `POST /api/v1/reviews { pull_request_id, reuse_ongoing: true }`.
4. Redirect (replace) to `/reviews/:reviewId`.

States: loading spinner, "PR not found or you don't have access" (404), quota-exceeded
message. Use a ref guard so React StrictMode doesn't fire the POST twice in dev.

### 8. Reviews list — `ui/src/pages/Reviews/Reviews.tsx`, `ui/src/api/reviews.ts`

- Read `pull_request_id` from the URL and pass it to the reviews API.
- Show a small "Filtered to one pull request · Clear" chip.
- Like the trigger page, switch to the PR's org first (shared hook
  `ui/src/hooks/useSwitchToPullRequestOrg.ts`), so multi-org users don't land on an empty list.

## Docs & navigation

- **Routes guide:** add `internal/docindex/docs/routes_guide/reviews/pr-review-link.md`, and
  update the Reviews list doc for the `pull_request_id` filter.
- **Mega menu:** no entry. The trigger page is only reached from the PR description link, and the
  filter is an option on the existing Reviews page.

## Known limitations

- Opening the trigger link starts a review, so chat/email link previewers (for a logged-in
  user) could start reviews. The 10-minute ongoing-review reuse limits this. Revisit with a
  confirm page if it becomes a problem.
- Bitbucket, Gitea, and Azure DevOps sync PRs into `pull_requests` from webhooks only (no
  periodic list sync), so PRs opened before the webhook was installed get no links.
- The description decisions table keeps the last 10 reviews (Azure DevOps caps descriptions
  at 4000 characters).
