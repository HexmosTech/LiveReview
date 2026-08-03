 ▐▛███▜▌   Claude Code v2.1.220
▝▜█████▛▘  Sonnet 5 · Claude Pro
  ▘▘ ▝▝    ~/bin/LiveReview


❯ /plan I want to add a unified way to list all the repositories more           
prominently in LiveReview (we already have repoistory listing code within each  
connector already, I think, so reuse that).                                     
                                                                                
And once we do that, I want a common way to list all the MR/PR in our UI for    
all the platforms.                                                              
                                                                                
The goal here is teams can look at MR/PR from all their connected git providers 
right within LiveReview, and check history of livereview reviews on it, and if  
necessary trigger a new review on them.                                         
                                                                                
Note that this system has to work across all the git providers we have, one.    
                                                                                
The other thing is - we should be able to sync the changes across these         
platforms in LiveReview.                                                        
                                                                                
We may not want to store all the data in those, but at least high level status  
should be synced, I guess, on what is happening.                                
                                                                                
The UI views will be more like Github or Gitlab itself. Repository view, global 
MR/PR view with filter options.                                                 
                                                                                
The detail view will be less rich than each platform one, but will bring in the 
desc, maybe fetch code on demand (by deafult) and show, etc                     
                                                                                
And see review history per MR/PR and be able to trigger new one on that MR/PR   
                                                                                
The challenge here are multifold                                                
                                                                                
1. Multiple platforms have different APIs, webhooks, etc                        
2. Need common structure in LR                                                  
3. Need simplest way to sync information/status                                 
                                                                                
What I want first and foremost is a Github and Gitlab integration to start with 
(two, so we don't overffit to one platform) as an API plus database changes     
which must be tested somehow                                                    
                                                                                
Come up with a plan for this. If there are trade offs, etc discuss with me on   
how these decisions can be made                                                 
                                                                                
Goal is to get a good implementation for these two platforms first. We will     
defer the UI for later, for now, just backend stuff is required                 
  ⎿  Enabled plan mode

● I'll start by exploring the codebase to understand the existing connector
  architecture, repository listing code, and database schema before proposing a
  plan.

  Searched for 1 pattern (ctrl+o to expand)

● 3 background agents launched (↓ to manage)
   ├ Explore connector/provider architecture
   ├ Explore database schema and models
   └ Explore auth, org, and testing patterns

● I've launched three exploration agents in parallel to investigate: (1) the
  provider/connector architecture and existing repo/PR listing code, (2) the
  database schema and models for repos/PRs/reviews, and (3) auth/credential
  storage, job queue, and testing patterns. I'll report back once they complete.

  Ran 1 shell command (ctrl+o to expand)

● I'll wait for the exploration agents to finish before proceeding.

● All three exploration agents are still running in the background — I'll
  continue as soon as they report back.

✻ Sautéed for 50s

● Agent "Explore connector/provider architecture" finished · 2m 55s

● The connector-architecture exploration finished — key finding: repo listing
  exists per-provider but only returns name strings (no rich metadata), and only
  GitLab has any MR-listing function; everything else is single-PR-by-URL
  fetch. Waiting on the other two agents (DB schema, auth/jobqueue/testing
  patterns) before synthesizing the plan.

● Still waiting on the remaining two exploration agents.

✻ Churned for 7s

● Agent "Explore database schema and models" finished · 3m 22s

● The database-schema exploration confirms there's no repositories or
  pull_requests table today — only integration_tokens (connector),
  webhook_registry, and reviews (which doubles as a review-run log). Waiting on
  the last agent covering auth/jobqueue/testing patterns.

● Still waiting on the third exploration agent.

✻ Cooked for 7s

● Agent "Explore auth, org, and testing patterns" finished · 3m 46s

● All three research agents are done. I now have a clear picture: no
  repositories or pull_requests table exists today; GitLab has a partial MR-list
  function but GitHub/others don't; PR/MR history is only informally tracked
  via string-matching on reviews.pr_mr_url; River job queue exists but periodic
  jobs aren't used yet; webhook infra exists but doesn't yet handle PR-opened/MR
  events for state sync.

  Before I lock in a design, I want to align with you on a few trade-offs
  central to "simplest way to sync":

● User declined to answer questions
  ⎿  · Which repos should LiveReview actively sync PR/MR data for? Fetching PR 
     lists for every repo a token can see (e.g. an entire GitHub org) is 
     expensive and rate-limit-risky. (Only 'tracked' repos (Recommended) / All 
     discovered repos)
     · How should PR/MR status be kept up to date in LiveReview's DB? (Webhooks 
     + periodic reconciliation (Recommended) / Periodic polling only)
     · Existing 'reviews' rows are only linked to a PR/MR by matching the raw 
     pr_mr_url string. Once we add a canonical pull_requests table, should we 
     backfill old reviews to link them? (Backfill via URL match (Recommended) / 
     No backfill, link going forward only)

● What would you like to clarify about these questions?

✻ Worked for 12m 44s

✻ Claude resuming /loop wakeup (Jul 27 2:03pm)  ⎿  Interrupted · What should Claude do instead?

❯ The thing is "tracked repositories" is a bit tricky to understand here. Right 
  now what happens is after it is connected, we trigger "enable all" or         
  something like that in connector details by default, which essentially        
  installs some webhook (what webhook exactly? do we get notified for new       
  MR/PR? MR/PR modifications?) for all the repos visible via the token          
                                                                                
  Also note that server can go down and sometimes we have to be able to sync    
  what's missing in between, etc.                                               
                                                                                
  So the idea of "tracked repositories" is "all" for now without exceptions     

● Good clarification — let me check the actual webhook installation code to
  answer precisely rather than guess at what events are already subscribed.

  Searched for 2 patterns, read 1 file, ran 1 shell command (ctrl+o to expand)

● That's a very useful finding: GitHub webhooks are already installed with the 
  pull_request event (opened/edited/closed/reopened/synchronize — covers all
  state changes including merges), and GitLab webhooks are already installed
  with merge_requests event. But looking at
  internal/provider_input/github/github_provider.go, the event-conversion switch
  only handles issue_comment, pull_request_review_comment, pull_request_review
  — a raw pull_request event is accepted by CanHandleWebhook but silently
  dropped since there's no case for it. Same story on GitLab's merge_request
  object_kind. So no new webhook re-registration is needed on existing
  connectors — we just need to add handling for events that are already arriving
  but currently thrown away.

  Combined with your point about downtime, the design is now:
  - Tracked repos = all, no exceptions (as you said).
  - Sync mechanism = webhook-driven primary (finally act on the
  pull_request/merge_requests events already being delivered) + periodic 
  reconciliation poll as a fallback for server downtime, missed events, and
  initial backfill when a repo is first discovered.

  I'll fold this into the design. Let me now spawn a Plan agent to work out the
  detailed schema, interfaces, and job design given all this context.

● Plan(Design GitHub+GitLab repo/PR sync backend)
  ⎿  Backgrounded agent (↓ to manage · ctrl+o to expand)

● I've launched a Plan agent with the full research context and your two
  decisions (all repos tracked; webhook-driven sync using the already-installed
  but currently-dropped pull_request/merge_requests events, plus periodic
  reconciliation for downtime). It's designing the concrete schema, interfaces,
  job design, and API endpoints now — I'll report back when it's done.

● Waiting on the Plan agent now.

✻ Worked for 2m 8s

✻ Claude resuming /loop wakeup (Jul 27 2:11pm)

● Let me check whether the Plan agent has finished.

● Task Output(non-blocking) a64fa3b6e00d4d9f4
  ⎿  Task is still running…

● The Plan agent is still working (partway through designing the schema and
  provider interfaces). I'll avoid polling its raw transcript again and just
  wait for the completion notification.

● Still waiting for the Plan agent to finish.

✻ Sautéed for 27s

● Agent "Design GitHub+GitLab repo/PR sync backend" finished · 6m 13s

● The Plan agent delivered a very detailed design. Let me verify a few of its
  most load-bearing claims against the actual code before finalizing —
  specifically the webhook orchestrator flow and the River job queue config,
  since the whole sync mechanism depends on those being accurate.

  Searched for 1 pattern, read 1 file (ctrl+o to expand)

● The webhook orchestrator confirms the plan's core premise exactly:
  convertToUnifiedEvent (line 495-508) tries ConvertCommentEvent then
  ConvertReviewerEvent, and returns a generic error if both fail — which the
  caller turns into an HTTP 400. This validates the design's central mechanism.
  I'm satisfied with the technical grounding; I'll now write the final plan.

● Updated plan
  ⎿  /plan to preview
● User approved Claude's plan
  ⎿  Plan saved to: ~/.claude/plans/linked-exploring-yao.md · /plan to edit
     Unified Repository + PR/MR Listing & Status Sync (Backend: GitHub + GitLab)

     Context

     LiveReview currently reviews PRs/MRs one at a time: a user pastes a PR/MR
     URL into POST /connectors/trigger-review, and that single URL becomes a
     reviews row. There is no place in LiveReview where a team can see "all our
     repos" or "all our open PRs/MRs" across their connected GitHub/GitLab
     accounts, and no persistent notion of a PR/MR as an entity — only a log of
     review runs, loosely grouped by matching the raw pr_mr_url string.

     The goal is to give teams a GitHub/GitLab-style view (deferred to a later
     phase for UI) backed now by real backend infrastructure: a canonical
     repositories and pull_requests table per org, kept in sync with the
     provider, so that review history can be grouped per PR/MR and a new review
     can be triggered directly from a known PR/MR record instead of a pasted
     URL. This plan covers GitHub and GitLab only, backend only (DB + Go),
     designed so Bitbucket/Gitea/Azure DevOps can be added later via the same
     interfaces without rework.

     Decisions locked in during planning discussion:
     - Tracked repos = all repos visible to a connector's token, no 
     opt-in/subset. Every repo discovered via a connector's PAT gets a
     repositories row and PR/MR syncing.
     - Sync = webhook-driven primary + periodic reconciliation fallback. Key
     finding: GitHub webhooks LiveReview already installs subscribe to the
     pull_request event, and GitLab webhooks already subscribe to merge_requests
     (internal/jobqueue/jobqueue.go webhook-install payloads). But today these
     events are silently unusable: WebhookOrchestratorV2.convertToUnifiedEvent
     (internal/api/webhook_orchestrator_v2.go:495-508) only tries
     ConvertCommentEvent then ConvertReviewerEvent; a bare GitHub pull_request
     event with action opened/synchronize/closed/etc. matches neither, so both
     fail and the endpoint returns HTTP 400 — a real, currently-broken webhook
     delivery, not just a wasted one. GitLab's merge_request object_kind event
     is accepted by CanHandleWebhook but ConvertCommentEvent unconditionally
     reads it as a comment, finds an empty body, and the downstream flow
     silently no-ops. So: no new webhook installation is needed — we add
     handling for events already arriving. Periodic polling (new to this
     codebase — River's PeriodicJobs, unused today) covers initial backfill,
     missed webhook deliveries, and catch-up after LiveReview downtime.
     - reviews.pull_request_id (new nullable FK) links review runs to a
     canonical PR/MR row, replacing the fragile pr_mr_url-string-matching
     grouping (TrackAICommentFromURL in storage/internal/review_processor).
     Existing reviews get a best-effort backfill by URL match; unmatched rows
     are left null (informational, not blocking).

     Architecture reuse (do not reinvent)

     - internal/providers.Provider interface (internal/providers/shared.go)
     stays untouched — new capabilities are added as additive optional 
     interfaces (RepositoryLister, PullRequestLister), type-asserted where
     needed. This mirrors an existing pattern already in this file
     (webhook_orchestrator_v2.go:208 does the same trick for
     signatureValidator).
     - Existing per-provider repo discovery
     (internal/providers/github/project_discovery.go DiscoverProjectsGitHub,
     internal/providers/gitlab/project_discovery.go DiscoverProjectsGitlab)
     returns only name strings — kept as-is for GET 
     /connectors/:id/repository-access (UI compatibility), not modified. New,
     richer list functions are added alongside.
     - internal/providers/gitlab/http_client.go:394 ListMergeRequests(projectID)
     has zero external call sites — safe to generalize in place (add state,
     page params) rather than duplicate.
     - integration_tokens (connector/credential) and webhook_registry (per-repo
     webhook status) tables are reused as-is.
     - River job queue (internal/jobqueue/jobqueue.go) is reused; this is the
     first use of River's PeriodicJobs feature in the codebase.
     - internal/retry backoff utilities are reused for transient-failure retry;
     GitHub/GitLab rate-limit headers are read directly in the new list
     functions (no such handling exists today except a Bitbucket-only
     client-side limiter).
     - Testing follows the existing convention in
     internal/api/bot_user_test_helpers_test.go: real dev Postgres,
     stubHTTPTransport (swaps http.DefaultTransport), insertIntegrationToken
     fixture helper with t.Cleanup. No testcontainers/mock-server library in
     this codebase — don't introduce one.

     1. Database migrations (db/migrations/)

     Migration A — new tables:

     CREATE TABLE repositories (
         id                  BIGSERIAL PRIMARY KEY,
         org_id              BIGINT NOT NULL REFERENCES orgs(id),
         connector_id        BIGINT NOT NULL REFERENCES integration_tokens(id)
     ON DELETE CASCADE,
         provider            TEXT NOT NULL,               -- 'github' | 'gitlab'
         provider_repo_id    TEXT NOT NULL,                -- stable external id
     as string
         full_name           TEXT NOT NULL,                -- 'owner/repo' or 
     'group/subgroup/project'
         name                TEXT NOT NULL,
         web_url             TEXT NOT NULL,
         clone_url           TEXT,
         ssh_url             TEXT,
         default_branch      TEXT,
         is_private          BOOLEAN NOT NULL DEFAULT true,
         description         TEXT,
         last_synced_at      TIMESTAMPTZ,
         last_sync_status    TEXT NOT NULL DEFAULT 'pending' CHECK
     (last_sync_status IN ('pending','ok','error')),
         last_sync_error     TEXT,
         metadata            JSONB NOT NULL DEFAULT '{}'::jsonb,
         created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
         updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
         CONSTRAINT uq_repositories_connector_provider_repo UNIQUE
     (connector_id, provider_repo_id)
     );
     CREATE INDEX idx_repositories_org_id         ON repositories(org_id);
     CREATE INDEX idx_repositories_connector_id   ON repositories(connector_id);
     CREATE INDEX idx_repositories_org_provider   ON repositories(org_id,
     provider);
     CREATE INDEX idx_repositories_last_synced_at ON
     repositories(last_synced_at);

     CREATE TABLE pull_requests (
         id                   BIGSERIAL PRIMARY KEY,
         repository_id        BIGINT NOT NULL REFERENCES repositories(id) ON
     DELETE CASCADE,
         org_id               BIGINT NOT NULL REFERENCES orgs(id),        -- 
     denormalized, matches reviews/ai_comments precedent
         provider             TEXT NOT NULL,
         provider_pr_id       TEXT NOT NULL,               -- stable external id
         number               INTEGER NOT NULL,            -- human-facing #123 
     / !45
         title                TEXT NOT NULL DEFAULT '',
         description          TEXT,
         state                TEXT NOT NULL CHECK (state IN
     ('open','closed','merged')),
         author_id            TEXT,
         author_username      TEXT,
         author_name          TEXT,
         author_avatar_url    TEXT,
         source_branch        TEXT,
         target_branch        TEXT,
         web_url              TEXT NOT NULL,
         provider_created_at  TIMESTAMPTZ,
         provider_updated_at  TIMESTAMPTZ,
         last_synced_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
         last_synced_source   TEXT NOT NULL DEFAULT 'poll' CHECK
     (last_synced_source IN ('webhook','poll','backfill')),
         metadata             JSONB NOT NULL DEFAULT '{}'::jsonb,         -- 
     labels, draft flag, raw extras (free to capture, not queried yet)
         created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
         updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
         CONSTRAINT uq_pull_requests_repo_provider_pr UNIQUE (repository_id,
     provider_pr_id)
     );
     CREATE UNIQUE INDEX uq_pull_requests_repo_number  ON
     pull_requests(repository_id, number);
     CREATE INDEX idx_pull_requests_org_id             ON pull_requests(org_id);
     CREATE INDEX idx_pull_requests_repository_state   ON
     pull_requests(repository_id, state);
     CREATE INDEX idx_pull_requests_org_state          ON pull_requests(org_id,
     state);
     CREATE INDEX idx_pull_requests_last_synced_at     ON
     pull_requests(last_synced_at);

     repositories uniqueness is scoped to (connector_id, provider_repo_id), not
     (org_id, provider, full_name): two connectors in one org could see the same
     physical repo, and per-connector rows avoid a cross-connector write race
     and match how webhook_registry already scopes by integration_token_id. If
     this becomes a real duplication problem later, dedupe at the API layer, not
     the DB layer.

     state is normalized (open/closed/merged) via one shared mapping function
     used identically by the poll path and the webhook path, so they can never
     disagree (see §2).

     Migration B — link reviews to PRs:
     ALTER TABLE reviews ADD COLUMN pull_request_id BIGINT REFERENCES
     pull_requests(id) ON DELETE SET NULL;
     CREATE INDEX idx_reviews_pull_request_id ON reviews(pull_request_id);

     One-time backfill (Go script, cmd/backfill-review-pr-links/main.go, run
     manually after initial sync populates pull_requests): UPDATE reviews r SET 
     pull_request_id = pr.id FROM pull_requests pr WHERE r.pull_request_id IS 
     NULL AND r.pr_mr_url IS NOT NULL AND pr.web_url = r.pr_mr_url; —
     informational/best-effort, log unmatched count, don't fail on mismatches
     (URL normalization differences are expected and acceptable to miss).

     2. Shared normalization package: internal/prsync/

     New leaf package (no dependents among
     internal/api/internal/jobqueue/internal/providers, avoiding the import
     cycle those packages would otherwise create with each other):
     - PullRequestStateEvent, RepositorySummary, PullRequestSummary structs —
     the common shape both the bulk-list provider functions and the
     webhook-conversion code populate.
     - NormalizeGitHubState(state string, merged bool) string and
     NormalizeGitLabState(state string) string — table-driven, unit tested
     (GitHub: merged=true always wins regardless of state; GitLab: opened→open,
     merged→merged, closed/locked→closed).

     3. Provider interface additions

     In internal/providers/shared.go, add (additive, doesn't touch existing
     Provider interface):
     type RepositoryLister interface {
         ListRepositories(ctx context.Context, cursor string) (*RepositoryPage,
     error)
     }
     type PullRequestLister interface {
         ListPullRequests(ctx context.Context, providerRepoID string, state
     PullRequestListState, cursor string) (*PullRequestPage, error)
     }
     (paged via an opaque cursor string so GitHub's Link-header pagination and
     GitLab's X-Next-Page header pagination share one call shape.)

     - New internal/providers/github/repo_list.go: ListRepositories (GET 
     /user/repos) and new internal/providers/github/pr_list.go: ListPullRequests
     (GET /repos/{owner}/{repo}/pulls?state=all&sort=updated&direction=desc),
     both properly following the Link: rel="next" header (existing
     DiscoverProjectsGitHub assumes "short page = last page," which is incorrect
     and should not be copied). Read X-RateLimit-Remaining/X-RateLimit-Reset on
     every response; on exhaustion, return a typed rate-limit error the sync
     worker can catch to reschedule the job (River JobSnooze) instead of
     blocking.
     - New internal/providers/gitlab/repo_list.go: ListRepositories (GET 
     /api/v4/projects?membership=true), reading X-Next-Page header. Extend 
     existing ListMergeRequests in http_client.go in place with state/page
     params (safe — no other call sites).
     - Both use internal/prsync's state-normalization functions so poll-derived
     and webhook-derived state can never diverge.

     4. Webhook handling: act on events already arriving

     Do not route PR/MR state sync through
     ConvertCommentEvent/ConvertReviewerEvent — those exist for the AI-response
     pipeline (bot-info lookup, response-warrant check, LOC preflight) which is
     irrelevant overhead for a state upsert. Instead, add a new branch in
     WebhookOrchestratorV2.ProcessWebhookEvent
     (internal/api/webhook_orchestrator_v2.go), inserted right after provider
     detection (line ~129) and before convertToUnifiedEvent (line ~132), that
     short-circuits to a dedicated lightweight flow via a new optional interface
     (prStateConverter, checked with the same type-assertion pattern already
     used at line 208 for signatureValidator):

     type prStateConverter interface {
         ConvertPullRequestStateEvent(headers map[string]string, body []byte)
     (*prsync.PullRequestStateEvent, bool, error)
     }

     - internal/provider_input/github/github_provider.go: add
     ConvertPullRequestStateEvent, matching only X-GitHub-Event: pull_request.
     Critical compatibility guard: must return matched=false for action in 
     {review_requested, review_request_removed} so those keep flowing through
     the existing (working) ConvertReviewerEvent path unchanged — this is a
     required regression test, not optional.
     - internal/provider_input/gitlab/gitlab_provider_v2.go: add
     ConvertPullRequestStateEvent, matching only object_kind: merge_request
     (must return matched=false for note/other kinds so comment handling is
     untouched).
     - On match, enqueue a new River job (PRStateSyncJobArgs, new "repo_sync"
     queue — kept separate from the "review" queue so sync traffic can't starve
     AI-review job concurrency) and return 200 immediately — fixing today's live
     GitHub-webhook-returns-400 bug as a side effect.

     5. Periodic reconciliation (River PeriodicJobs — new to this codebase)

     Two-tier design in internal/jobqueue/jobqueue.go:
     1. A periodic coordinator job (registered via river.Config.PeriodicJobs,
     interval ~15 min) queries repositories for rows where last_synced_at IS 
     NULL OR last_synced_at < now() - interval '20 minutes', and enqueues one
     RepoPRSyncJobArgs{RepositoryID} per stale repo onto "repo_sync".
     2. RepoPRSyncWorker calls the provider's ListPullRequests (sorted by
     updated desc, early-stopping pagination once provider_updated_at <= 
     repositories.last_synced_at to avoid re-walking a repo's entire PR history
     every cycle), upserts each into pull_requests, updates
     repositories.last_synced_at/last_sync_status.

     Because webhook-driven upserts always set last_synced_at = now() and
     last_synced_source = 'webhook', a repo with working webhooks naturally
     drops out of the stale-repo query and costs near-zero poll traffic —
     polling only picks up the slack for quiet repos or missed deliveries.
     Interval/staleness thresholds are exposed via the existing QueueConfig
     env-var pattern (internal/jobqueue/queue_config.go) so they're tunable
     without a code change.

     Initial backfill: extend AutoWebhookInstaller.processAutoInstallation
     (internal/api/auto_webhook_installer.go) — after project discovery, upsert
     repositories rows (using the new ListRepositories call, not the name-only
     discovery function) and enqueue one uncapped
     RepoPRSyncJobArgs{RepositoryID, InitialBackfill: true} per new repo,
     alongside the existing webhook-install job queuing.

     6. Idempotency & concurrency

     Single shared upsert (new storage/providers/pullrequests/upsert.go,
     following the existing storage/<domain> package convention), used
     identically by both the webhook path and the poll path:

     INSERT INTO pull_requests (...) VALUES (...)
     ON CONFLICT (repository_id, provider_pr_id) DO UPDATE SET
         title = EXCLUDED.title, state = EXCLUDED.state, ... , last_synced_at =
     now(), updated_at = now()
     WHERE pull_requests.provider_updated_at IS NULL
        OR EXCLUDED.provider_updated_at >= pull_requests.provider_updated_at
     RETURNING id;

     The WHERE guard prevents a stale/delayed webhook delivery from clobbering
     newer state already recorded by a faster delivery or a poll — no explicit
     row locking needed, Postgres's ON CONFLICT already takes the necessary lock
     atomically. repositories upsert is a plain ON CONFLICT ... DO UPDATE with
     no staleness guard (discovery snapshots are always a full, self-consistent
     picture). If a webhook for a PR arrives before its repo's backfill has run,
     the worker inserts a minimal repositories row on the fly from the
     webhook's own repository payload fields (ON CONFLICT DO NOTHING), and the
     later backfill fills in the rest.

     7. API endpoints (backend only, curl-testable)

     New file internal/api/repositories_pr_routes.go, mounted under the existing
     org-auth middleware chain (same as connectorGroup):

     GET  /api/v1/repositories?connector_id=&provider=&search=&page=&per_page=
     GET  /api/v1/repositories/:repoId
     POST /api/v1/repositories/:repoId/sync                       -- resync one
     repo's PRs now
     GET  /api/v1/repositories/:repoId/pull-requests?state=open|closed|merged|al
     l&page=&per_page=
     GET  /api/v1/repositories/:repoId/pull-requests/:prId         -- includes
     review history joined
     POST /api/v1/connectors/:connectorId/repositories/sync        -- resync a
     whole connector
     POST /api/v1/reviews  {"pull_request_id": 123}                 -- replaces
     today's stub (server.go:1739)

     POST /api/v1/reviews reuses TriggerReviewV2's internals
     (internal/api/review_service.go): a new setupReviewContextForPullRequest
     resolves web_url/connector_id directly via a join on
     pull_requests/repositories (skipping the URL-host-matching step used by the
     URL-based flow), sets reviews.pull_request_id at creation, then reuses the
     existing authentication, metadata-enrichment, and async-job-launch phases
     unchanged.

     8. Testing

     Follow internal/api/bot_user_test_helpers_test.go conventions throughout
     (real dev Postgres, stubHTTPTransport, insertIntegrationToken + t.Cleanup):
     - Provider list functions: multi-page pagination correctness (Link header /
     X-Next-Page), state-mapping edge cases (merged=true overrides state),
     rate-limit-header handling.
     - internal/prsync normalization: table-driven tests covering every raw
     state value for both providers.
     - Webhook conversion: pull_request/merge_request payloads convert
     correctly; critical regression test: review_requested action must still
     return matched=false and flow through the existing reviewer-assignment path
     unchanged — run the existing reviewer/comment webhook test suite after
     this change and confirm no diffs in behavior.
     - Upsert idempotency: running the same upsert twice produces one row; an
     older provider_updated_at payload doesn't overwrite newer state; concurrent
     upserts don't deadlock or duplicate.
     - Periodic job: NewJobQueue starts cleanly with PeriodicJobs configured;
     coordinator only enqueues sync jobs for genuinely stale repos.
     - API handlers: pagination/filtering correctness, and org-scoping (a
     repo/PR from another org must 404, not leak).
     - Manual end-to-end script (documented, run against a local server +
     sandbox GitHub/GitLab PAT): create connector → verify
     repositories/pull_requests populate → open/close a real PR and verify
     near-real-time update without waiting for the poll interval → trigger a
     review via pull_request_id → verify it appears in that PR's review history.

     9. Build order

     1. Migrations (§1) — establishes schema everything else targets.
     2. internal/prsync (§2) — zero new dependencies, needed by everything
     downstream.
     3. Provider list functions (§3) — independently testable, no DB/queue
     coupling.
     4. Upsert storage layer (§6) — testable against real Postgres once
     migrations exist; built once, shared by both sync paths so they can't
     diverge.
     5. Sync worker + River PeriodicJobs wiring (§5) — now has a proven data
     source (step 3) and persistence layer (step 4).
     6. Webhook handling additions (§4) — done after the sync worker exists (its
     job is to call into it) and last among the risky changes, since it touches
     a shared, currently-working code path — paired with the regression tests
     before moving on.
     7. Auto-installer extension for initial backfill (§5) — additive change,
     low risk, depends on step 5.
     8. API endpoints (§7) — last, so integration tests exercise a fully-wired
     system rather than stubbed data.
     9. One-time reviews backfill script (§1) — run manually, last, once real
     data exists to match against.

     Notes on adjustable defaults (flagged, not blocking)

     - Reconciliation interval (15 min coordinator / 20 min staleness) is a
     starting guess balancing GitHub's rate limit against freshness — made
     env-var tunable rather than hardcoded.
     - GitHub's PR-list endpoint has no since filter; the
     early-stop-on-provider_updated_at optimization (§5) is treated as required
     from day one, not a later optimization, since retrofitting it after
     reconciliation is live in production is riskier than building it now.
     - pull_requests.metadata captures whatever the list API gives for free
     (labels, draft flag) since it costs nothing extra, but no dedicated
     columns/query support for those fields yet — matches the "defer UI, don't
     over-build" instruction.

● The plan is approved. I'll start implementing, following the build order. Let
  me set up task tracking and check the current repo state first.

  Searched for 1 pattern, read 1 file, ran 1 shell command (ctrl+o to expand)

● Confirmed: dbmate up runs migrations, module is github.com/livereview. Let me
  set up a task list for the build order, then start with the migrations.

● Now starting with step 1: the database migrations.

  Listed 1 directory (ctrl+o to expand)

● No collisions. Let me write the two migration files.

● Write(db/migrations/20260727120000_create_repositories_and_pull_requests.sql)
  ⎿  Wrote 75 lines to
     db/migrations/20260727120000_create_repositories_and_pull_requests.sql
       1 -- migrate:up
       3 CREATE TABLE repositories (
       4     id                  BIGSERIAL PRIMARY KEY,
       5     org_id              BIGINT NOT NULL REFERENCES orgs(id),
         okens(id) ON DELETE CASCADE,
       7     provider            TEXT NOT NULL,               -- 'github'
       8     provider_repo_id    TEXT NOT NULL,                -- stable 
         external id as string
       9     full_name           TEXT NOT NULL,                -- 'owner/
      10     name                TEXT NOT NULL,
     … +95 lines (ctrl+o to expand)

● Write(db/migrations/20260727120001_add_pull_request_id_to_reviews.sql)
  ⎿  Wrote 13 lines to
     db/migrations/20260727120001_add_pull_request_id_to_reviews.sql
       1 -- migrate:up
       3 ALTER TABLE reviews
       4     ADD COLUMN pull_request_id BIGINT REFERENCES pull_requests(i
         d) ON DELETE SET NULL;
       6 CREATE INDEX idx_reviews_pull_request_id ON reviews(pull_request
         _id);
       8 COMMENT ON COLUMN reviews.pull_request_id IS 'Canonical pull_req
         uests row this review run belongs to, if known. Nullable for rev
         iews triggered by raw URL or predating this column.';
      10 -- migrate:down
     … +7 lines (ctrl+o to expand)

● Now let's check that dbmate is available and run the migrations against the
  dev DB.

  Ran 4 shell commands (ctrl+o to expand)
● Migrations applied and verified cleanly. Let's check db/schema.sql diff to
  confirm nothing unexpected changed, then move to step 2.

  Ran 2 shell commands (ctrl+o to expand)

● That's concerning — the schema.sql regeneration is removing
  public._seed_backup and public.mcp_authorizations table definitions, meaning
  my local dev DB is missing tables that git's checked-in schema expects. Let me
  investigate before this becomes a bad commit.

  Searched for 2 patterns, read 1 file, ran 1 shell command (ctrl+o to expand)

● This is pre-existing drift unrelated to my change — a migration file
  (20260509195524, creating mcp_authorizations/_seed_backup) was removed from
  the repo at some point without regenerating schema.sql, and my local dev DB
  never had it applied. I shouldn't let my migration run silently delete those
  table definitions from the tracked schema. Let me restore them.

  Ran 2 shell commands (ctrl+o to expand)

● Good, dbmate dump exists. Let me restore the missing tables/constraints/index
  on my local dev DB and the schema_migrations bookkeeping row, then re-dump.

  Ran 1 shell command (ctrl+o to expand)

● Now let's re-dump the schema so it reflects both the restored tables and my
  new ones correctly.

  Ran 3 shell commands (ctrl+o to expand)

● The schema.sql diff is now correctly scoped to just my new tables (plus a
  harmless pg_dump version banner difference from local tooling). Migration step
  is verified and clean.

  Ran 1 shell command (ctrl+o to expand)

● Write(internal/prsync/types.go)
  ⎿  Wrote 68 lines to internal/prsync/types.go
       1 // Package prsync holds provider-agnostic types and state-normal
       2 // between the bulk repository/PR listing code (internal/provide
         rs/{github,gitlab})
         nternal/jobqueue),
       4 // so those two packages don't need to depend on each other.
       6 
       7 import "time"
       9 // RepositorySummary is the normalized shape returned by a provi
         der's bulk
         s row.
     … +69 lines (ctrl+o to expand)

● Write(internal/prsync/normalize.go)
  ⎿  Wrote 33 lines to internal/prsync/normalize.go
       1 package prsync
       2 
         onical
       4 // "open" | "closed" | "merged" values stored in pull_requests.s
         tate.
         d boolean;
       6 // merged always wins regardless of the state field.
       7 func NormalizeGitHubState(state string, merged bool) string {
       9     return "merged"
      10   }
     … +30 lines (ctrl+o to expand)

● Write(internal/prsync/normalize_test.go)
  ⎿  Wrote 49 lines to internal/prsync/normalize_test.go
       1 package prsync
       2 
       3 import "testing"
       4 
       5 func TestNormalizeGitHubState(t *testing.T) {
       6   cases := []struct {
       7     name   string
       8     state  string
       9     merged bool
      10     want   string
     … +42 lines (ctrl+o to expand)

  Ran 1 shell command (ctrl+o to expand)

● Package builds and tests pass.

● Now onto step 3: provider list functions. Let me examine the existing code
  closely before extending it.

  Read 3 files, ran 1 shell command (ctrl+o to expand)
  ⎿  Found 29 new diagnostic issues in 14 files (ctrl+o to expand)
● The only relevant diagnostic is a style lint on my new types.go (interface{} →
  any); the rest are pre-existing unrelated files. Let me check codebase
  convention before fixing.

  Searched for 1 pattern (ctrl+o to expand)

● interface{} dominates the codebase 585:76 over any, so I'll keep it as-is for
  consistency rather than chase the linter nit. Let me check for existing
  Link-header parsing or rate-limit handling patterns before writing new code.

  Searched for 3 patterns, read 1 file (ctrl+o to expand)

● Now let's implement. First, additions to internal/providers/shared.go:

● Update(internal/providers/shared.go)
  ⎿  Added 3 lines
       2  
       3  import (
       4    "context"
       5 +  "fmt"                                                        
       6 +  "time"                                                       
       7  
       8 +  "github.com/livereview/internal/prsync"                      
       9    "github.com/livereview/pkg/models"
      10  )
      11  

● Update(internal/providers/shared.go)
  ⎿  Added 35 lines
      48    HeadSHA  string
      49    StartSHA string
      50  }
      51 +                                                               
      52 +// PullRequestListState is the state filter passed to a bulk PR
         +/MR listing call.                                              
      53 +type PullRequestListState string                               
      54 +                                                               
      55 +const (                                                        
      56 +  PRListStateAll    PullRequestListState = "all"               
      57 +  PRListStateOpen   PullRequestListState = "open"              
      58 +  PRListStateClosed PullRequestListState = "closed"            
      59 +)                                                              
      60 +                                                               
      61 +// RepositoryPage is one page of a bulk repository-listing call
         +, with an opaque                                               
      62 +// cursor for the next page (empty string means no more pages).
      63 +type RepositoryPage struct {                                   
      64 +  Repositories []prsync.RepositorySummary                      
      65 +  NextCursor   string                                          
      66 +}                                                              
      67 +                                                               
      68 +// PullRequestPage is one page of a bulk PR/MR-listing call, wi
         +th an opaque                                                   
      69 +// cursor for the next page (empty string means no more pages).
      70 +type PullRequestPage struct {                                  
      71 +  PullRequests []prsync.PullRequestSummary                     
      72 +  NextCursor   string                                          
      73 +}                                                              
      74 +                                                               
      75 +// RateLimitedError signals that a provider API call was reject
         +ed or should be                                                
      76 +// deferred due to rate limiting. Callers (e.g. the repo/PR syn
         +c worker) can                                                  
      77 +// check for this via errors.As and reschedule the work instead
         + of failing it.                                                
      78 +type RateLimitedError struct {                                 
      79 +  Provider   string                                            
      80 +  RetryAfter time.Duration                                     
      81 +}                                                              
      82 +                                                               
      83 +func (e *RateLimitedError) Error() string {                    
      84 +  return fmt.Sprintf("%s rate limit exceeded, retry after %s", 
         +e.Provider, e.RetryAfter)                                      
      85 +}                                                              
  ⎿  Found 1 new diagnostic issue in 1 file (ctrl+o to expand)

  Ran 1 shell command (ctrl+o to expand)

● Builds clean. Now the GitHub repo/PR listing functions.

  Ran 1 shell command (ctrl+o to expand)

● Write(internal/providers/github/base_url.go)
  ⎿  Wrote 13 lines to internal/providers/github/base_url.go
       1 package github
       2 
       3 import "strings"
       5 // apiBaseURL returns the GitHub REST API base URL for a given c
         onnector base
       6 // URL: github.com uses api.github.com, while GitHub Enterprise 
         instances serve
       8 func apiBaseURL(baseURL string) string {
       9   if baseURL == "" || baseURL == "https://github.com" {
      10     return "https://api.github.com"
     … +5 lines (ctrl+o to expand)

● Write(internal/providers/github/pagination.go)
  ⎿  Wrote 23 lines to internal/providers/github/pagination.go
       1 package github
       2 
       3 import (
       4   "net/http"
       5   "regexp"
       7 
       8 var linkNextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)
       9 
      10 // nextPageFromLinkHeader extracts the "next" page URL from a Gi
         tHub API
     … +15 lines (ctrl+o to expand)

● Write(internal/providers/github/repo_list.go)
  ⎿  Wrote 113 lines to internal/providers/github/repo_list.go
       1 package github
       2 
       3 import (
       4   "context"
       5   "encoding/json"
       6   "fmt"
       7   "io"
       8   "net/http"
       9   "net/url"
      10   "strconv"
     … +119 lines (ctrl+o to expand)

● Write(internal/providers/github/pr_list.go)
  ⎿  Wrote 128 lines to internal/providers/github/pr_list.go
       1 package github
       2 
       3 import (
       4   "context"
       5   "encoding/json"
       6   "fmt"
       7   "io"
       8   "net/http"
       9   "net/url"
      10   "strconv"
     … +129 lines (ctrl+o to expand)

  Ran 1 shell command (ctrl+o to expand)

● Builds clean. Now GitLab: repo listing + extending ListMergeRequests.

● Write(internal/providers/gitlab/repo_list.go)
  ⎿  Wrote 111 lines to internal/providers/gitlab/repo_list.go
       1 package gitlab
       2 
       3 import (
       4   "context"
       5   "encoding/json"
       6   "fmt"
       7   "io"
       8   "net/http"
       9   "net/url"
      10   "strconv"
     … +115 lines (ctrl+o to expand)

  Searched for 1 pattern (ctrl+o to expand)
● Confirmed zero call sites. Safe to modify in place.

● Update(internal/providers/gitlab/http_client.go)
  ⎿  Added 2 lines
      55    SourceBranch string `json:"source_branch"`
      56    TargetBranch string `json:"target_branch"`
      57    WebURL       string `json:"web_url"`
      58 +  CreatedAt    string `json:"created_at"`                      
      59 +  UpdatedAt    string `json:"updated_at"`                      
      60    Author       struct {
      61      ID        int    `json:"id"`
      62      Username  string `json:"username"`

● Update(internal/providers/gitlab/http_client.go)
  ⎿  Added 29 lines, removed 9 lines
      392    return notes, nil
      393  }
      394  
      395 -// ListMergeRequests lists merge requests for a project       
      396 -func (c *GitLabHTTPClient) ListMergeRequests(projectID string)
          - ([]GitLabMergeRequest, error) {                              
      395 +// ListMergeRequests lists merge requests for a project. state
          + is the GitLab                                                
      396 +// API state filter ("opened", "closed", "merged", or "all"); 
          +page/perPage                                                  
      397 +// control pagination. Returns the parsed page, the value of t
          +he response's                                                 
      398 +// X-Next-Page header (empty string if there is no next page),
          + and any error.                                               
      399 +func (c *GitLabHTTPClient) ListMergeRequests(projectID string,
          + state string, page, perPage int) ([]GitLabMergeRequest, strin
          +g, error) {                                                   
      400 +  if state == "" {                                            
      401 +    state = "opened"                                          
      402 +  }                                                           
      403 +  if perPage <= 0 {                                           
      404 +    perPage = 100                                             
      405 +  }                                                           
      406 +  if page <= 0 {                                              
      407 +    page = 1                                                  
      408 +  }                                                           
      409 +                                                              
      410 +  params := url.Values{}                                      
      411 +  params.Add("state", state)                                  
      412 +  params.Add("page", strconv.Itoa(page))                      
      413 +  params.Add("per_page", strconv.Itoa(perPage))               
      414 +  params.Add("order_by", "updated_at")                        
      415 +  params.Add("sort", "desc")                                  
      416 +                                                              
      417    // Create the correct URL with plural 'merge_requests'
      398 -  requestURL := fmt.Sprintf("%s/projects/%s/merge_requests?sta
          -te=opened",                                                   
      399 -    c.baseURL, url.PathEscape(projectID))                     
      418 +  requestURL := fmt.Sprintf("%s/projects/%s/merge_requests?%s"
          +,                                                             
      419 +    c.baseURL, url.PathEscape(projectID), params.Encode())    
      420  
      421    // Make the request
      422    req, err := networkgitlab.NewRequest("GET", requestURL, nil)
      423    if err != nil {
      404 -    return nil, fmt.Errorf("failed to create request: %w", err
          -)                                                             
      424 +    return nil, "", fmt.Errorf("failed to create request: %w",
          + err)                                                         
      425    }
      426  
      427    // Add authentication
     ...
      430    // Execute the request
      431    resp, err := networkgitlab.Do(c.client, req)
      432    if err != nil {
      413 -    return nil, fmt.Errorf("failed to execute request: %w", er
          -r)                                                            
      433 +    return nil, "", fmt.Errorf("failed to execute request: %w"
          +, err)                                                        
      434    }
      435    defer resp.Body.Close()
      436  
      437    // Check for errors
      438    if resp.StatusCode != http.StatusOK {
      439      body, _ := io.ReadAll(resp.Body)
      420 -    return nil, fmt.Errorf("API request failed with status %d:
          - %s", resp.StatusCode, string(body))                          
      440 +    return nil, "", fmt.Errorf("API request failed with status
          + %d: %s", resp.StatusCode, string(body))                      
      441    }
      442  
      443    // Parse the response
      444    var mrs []GitLabMergeRequest
      445    if err := json.NewDecoder(resp.Body).Decode(&mrs); err != ni
           l {
      426 -    return nil, fmt.Errorf("failed to decode response: %w", er
          -r)                                                            
      446 +    return nil, "", fmt.Errorf("failed to decode response: %w"
          +, err)                                                        
      447    }
      448  
      429 -  return mrs, nil                                             
      449 +  return mrs, resp.Header.Get("X-Next-Page"), nil             
      450  }
      451  
      452  // CreateMRComment creates a comment on a merge request
  ⎿  Found 2 new diagnostic issues in 1 file (ctrl+o to expand)

● Need to add the strconv import.

● Update(internal/providers/gitlab/http_client.go)
  ⎿  Added 1 line
       6    "io"
       7    "net/http"
       8    "net/url"
       9 +  "strconv"                                                    
      10    "strings"
      11    "time"
      12  

● Now let's write the pr_list.go wrapper for GitLab that produces the normalized
  providers.PullRequestPage.

● Write(internal/providers/gitlab/pr_list.go)
  ⎿  Wrote 65 lines to internal/providers/gitlab/pr_list.go
       1 package gitlab
       2 
       3 import (
       4   "context"
       5   "strconv"
       6   "time"
       7 
       8   "github.com/livereview/internal/prsync"
       9   "github.com/livereview/internal/providers"
      10 )
     … +62 lines (ctrl+o to expand)

  Ran 1 shell command (ctrl+o to expand)

● Clean build. Let's do a full project build to make sure nothing else broke,
  then write unit tests for these new functions.

  Ran 4 shell commands (ctrl+o to expand)

● Whole project (minus two pre-existing broken standalone dirs unrelated to my
  changes) builds cleanly. Now let's write unit tests for the new provider list
  functions using the existing stubHTTPTransport convention.

  Searched for 2 patterns, listed 1 directory (ctrl+o to expand)

● There's already a project_discovery_test.go — let me check its stubbing
  pattern to match convention exactly.

  Read 1 file (ctrl+o to expand)

● Write(internal/providers/github/repo_list_test.go)
  ⎿  Wrote 125 lines to internal/providers/github/repo_list_test.go
       1 package github
       2 
       3 import (
       4   "bytes"
       5   "context"
       6   "io"
       7   "net/http"
       8   "strings"
       9   "testing"
      10 )
     … +140 lines (ctrl+o to expand)

● Let me fix that last test — I referenced a helper that doesn't exist. Should
  use errors.As directly.

● Update(internal/providers/github/repo_list_test.go)
  ⎿  Added 3 lines
       3  import (
       4    "bytes"
       5    "context"
       6 +  "errors"                                                     
       7    "io"
       8    "net/http"
       9    "strings"
      10    "testing"
      11 +                                                               
      12 +  "github.com/livereview/internal/providers"                   
      13  )
      14  
      15  type roundTripFunc func(*http.Request) (*http.Response, error)

● Update(internal/providers/github/repo_list_test.go)
  ⎿  Added 6 lines, removed 3 lines
      121    if err == nil {
      122      t.Fatal("expected an error")
      123    }
      124 -  var rlErr *ratelimitedErrorForTest                          
      125 -  if !asRateLimited(err, &rlErr) {                            
      126 -    t.Fatalf("expected a rate-limited error, got: %v", err)   
      124 +  var rlErr *providers.RateLimitedError                       
      125 +  if !errors.As(err, &rlErr) {                                
      126 +    t.Fatalf("expected a *providers.RateLimitedError, got: %v"
          +, err)                                                        
      127    }
      128 +  if rlErr.RetryAfter <= 0 {                                  
      129 +    t.Errorf("expected a positive RetryAfter, got %v", rlErr.R
          +etryAfter)                                                    
      130 +  }                                                           
      131  }

● Write(internal/providers/github/pr_list_test.go)
  ⎿  Wrote 59 lines to internal/providers/github/pr_list_test.go
      1 package github
      2 
      3 import (
      4   "context"
      5   "net/http"
      6   "testing"
      7 
      8   "github.com/livereview/internal/providers"
      9 )
     … +65 lines (ctrl+o to expand)

● Now let's run these GitHub tests.

  Ran 1 shell command (ctrl+o to expand)

● All GitHub tests pass. Now let's run the full package test suite for github to
  make sure I didn't break anything existing, then move to GitLab tests.
  Ran 1 shell command (ctrl+o to expand)

● Now GitLab tests.

● Write(internal/providers/gitlab/repo_list_test.go)
  ⎿  Wrote 117 lines to internal/providers/gitlab/repo_list_test.go
      1 package gitlab
      2 
      3 import (
      4   "bytes"
      5   "context"
      6   "errors"
      7   "io"
      8   "net/http"
      9   "testing"
     … +129 lines (ctrl+o to expand)

● Write(internal/providers/gitlab/pr_list_test.go)
  ⎿  Wrote 62 lines to internal/providers/gitlab/pr_list_test.go
      1 package gitlab
      2 
      3 import (
      4   "context"
      5   "net/http"
      6   "testing"
      7 
      8   "github.com/livereview/internal/providers"
      9 )
     … +67 lines (ctrl+o to expand)

  Ran 1 shell command (ctrl+o to expand)

● All pass. Let's run the full GitLab package test suite to ensure the
  ListMergeRequests signature change didn't break anything else.

  Ran 1 shell command (ctrl+o to expand)

● All existing tests still pass. Step 3 is done.

● Now step 4: the upsert storage layer. Let me check an existing
  storage/providers/* package for convention.

  Ran 2 shell commands (ctrl+o to expand)

● Write(storage/providers/pullrequests/upsert.go)
  ⎿  Wrote 192 lines to storage/providers/pullrequests/upsert.go
       1 // Package pullrequests provides the single, shared upsert path 
         for the
         e webhook-driven
       3 // sync path and the periodic-reconciliation poll path so the tw
         o can never
       5 package pullrequests
       6 
       7 import (
       9   "encoding/json"
      10   "time"
     … +220 lines (ctrl+o to expand)

  Searched for 1 pattern, read 1 file, ran 4 shell commands (ctrl+o to expand)
  ⎿  Found 3 new diagnostic issues in 1 file (ctrl+o to expand)

● Good, org_id=1 exists by default. Now let's write the storage test using
  internal/database.NewDB() and cleanup-safe fixtures.

● Write(storage/providers/pullrequests/upsert_test.go)
  ⎿  Wrote 260 lines to storage/providers/pullrequests/upsert_test.go
       1 package pullrequests
       2 
       3 import (
       4   "database/sql"
       5   "sync"
       6   "testing"
       7   "time"
       8 
       9   "github.com/livereview/internal/database"
      10 )
     … +290 lines (ctrl+o to expand)

  Ran 1 shell command (ctrl+o to expand)

● All 5 tests pass, including the critical stale-write-guard and concurrency
  tests. Let's verify no leftover test data and mark this step complete.

  Ran 1 shell command (ctrl+o to expand)

● Clean — no leftover fixtures.

● Now step 5: the sync worker and River PeriodicJobs wiring. Let me examine the
  existing job queue structure closely.

  Searched for 8 patterns, read 2 files, ran 1 shell command (ctrl+o to expand)
● Good, confirmed. Now let's check queue_config.go for the RiverQueueConfig()
  method and QueueConfig struct to add a "repo_sync" queue.

  Searched for 2 patterns, ran 1 shell command (ctrl+o to expand)

● No cycle risk. Now let me check the pull_requests/repositories schema field
  names once more and write the new worker file.

  Searched for 2 patterns (ctrl+o to expand)

● Good. Now let's write the new worker file.

● Write(internal/jobqueue/repo_sync_worker.go)
  ⎿  Wrote 337 lines to internal/jobqueue/repo_sync_worker.go
      1 package jobqueue
      2 
      3 import (
      4   "context"
      5   "database/sql"
      6   "errors"
      7   "fmt"
      8   "log"
      9   "time"
     … +380 lines (ctrl+o to expand)

  Read 1 file (ctrl+o to expand)

● Update(internal/jobqueue/repo_sync_worker.go)
  ⎿  Added 2 lines, removed 2 lines
      278    }
      279  
      280    rows, err := w.db.QueryContext(ctx,
      281 -    `SELECT id FROM repositories WHERE last_synced_at IS NULL 
          -OR last_synced_at < now() - $1::interval`,                    
      282 -    threshold.String(),                                       
      281 +    `SELECT id FROM repositories WHERE last_synced_at IS NULL 
          +OR last_synced_at < now() - make_interval(secs => $1)`,       
      282 +    threshold.Seconds(),                                      
      283    )
      284    if err != nil {
      285      return fmt.Errorf("failed to query stale repositories: %w"
           , err)

● Let's build this file now.

  Ran 1 shell command (ctrl+o to expand)

● Now let's add the repo_sync queue and reconciliation config to
  queue_config.go.

  Read 1 file (ctrl+o to expand)

● Update(internal/jobqueue/queue_config.go)
  ⎿  Added 20 lines
      103  
      104    // Webhook Configuration
      105    WebhookConfig WebhookConfig
      106 +                                                              
      107 +  // Repository/PR sync Configuration                         
      108 +  RepoSyncConfig RepoSyncConfig                               
      109  }
      110  
      111 +// RepoSyncConfig controls the periodic reconciliation sweep t
          +hat catches PR/MR                                             
      112 +// updates missed by webhooks (or backfills repos discovered b
          +efore webhooks                                                
      113 +// existed for them). Tunable via env vars so ops can trade of
          +f freshness                                                   
      114 +// against provider API rate-limit usage without a code change
          +.                                                             
      115 +type RepoSyncConfig struct {                                  
      116 +  // CoordinatorInterval is how often the sweep looks for stal
          +e repositories.                                               
      117 +  CoordinatorInterval time.Duration // default: 15 minutes    
      118 +                                                              
      119 +  // StalenessThreshold is how old a repository's last_synced_
          +at must be                                                    
      120 +  // before the sweep re-syncs it. Repos kept fresh by webhook
          +-driven upserts                                               
      121 +  // naturally stay below this threshold and are skipped.     
      122 +  StalenessThreshold time.Duration // default: 20 minutes     
      123 +                                                              
      124 +  // MaxWorkers is the concurrency of the "repo_sync" queue.  
      125 +  MaxWorkers int // default: 5                                
      126 +}                                                             
      127 +                                                              
      128  // RetryPolicy defines how failed jobs are retried
      129  type RetryPolicy struct {
      130    // InitialInterval is the time to wait before the first retr
           y

● Now let's add defaults and env-var overrides to DefaultQueueConfig() and
  RiverQueueConfig().

  Read 1 file (ctrl+o to expand)

● Update(internal/jobqueue/queue_config.go)
  ⎿  Added 32 lines
      210          PipelineEvents:      false, // Not needed for code rev
           iew triggers
      211        },
      212      },
      213 +                                                              
      214 +    // Repository/PR sync configuration - overridable via env 
          +vars, see                                                     
      215 +    // repoSyncConfigFromEnv.                                 
      216 +    RepoSyncConfig: repoSyncConfigFromEnv(),                  
      217    }
      218  }
      219  
      220 +// repoSyncConfigFromEnv builds RepoSyncConfig from defaults, 
          +overridable via                                               
      221 +// LIVEREVIEW_REPO_SYNC_INTERVAL_MINUTES / LIVEREVIEW_REPO_SYN
          +C_STALENESS_MINUTES                                           
      222 +// / LIVEREVIEW_REPO_SYNC_MAX_WORKERS so the reconciliation ca
          +dence can be tuned                                            
      223 +// per-deployment without a code change.                      
      224 +func repoSyncConfigFromEnv() RepoSyncConfig {                 
      225 +  config := RepoSyncConfig{                                   
      226 +    CoordinatorInterval: 15 * time.Minute,                    
      227 +    StalenessThreshold:  20 * time.Minute,                    
      228 +    MaxWorkers:          5,                                   
      229 +  }                                                           
      230 +  if v := os.Getenv("LIVEREVIEW_REPO_SYNC_INTERVAL_MINUTES"); 
          +v != "" {                                                     
      231 +    if mins, err := strconv.Atoi(v); err == nil && mins > 0 { 
      232 +      config.CoordinatorInterval = time.Duration(mins) * time.
          +Minute                                                        
      233 +    }                                                         
      234 +  }                                                           
      235 +  if v := os.Getenv("LIVEREVIEW_REPO_SYNC_STALENESS_MINUTES");
          + v != "" {                                                    
      236 +    if mins, err := strconv.Atoi(v); err == nil && mins > 0 { 
      237 +      config.StalenessThreshold = time.Duration(mins) * time.M
          +inute                                                         
      238 +    }                                                         
      239 +  }                                                           
      240 +  if v := os.Getenv("LIVEREVIEW_REPO_SYNC_MAX_WORKERS"); v != 
          +"" {                                                          
      241 +    if n, err := strconv.Atoi(v); err == nil && n > 0 {       
      242 +      config.MaxWorkers = n                                   
      243 +    }                                                         
      244 +  }                                                           
      245 +  return config                                               
      246 +}                                                             
      247 +                                                              
      248  // ProductionQueueConfig returns a configuration optimized for
            production use
      249  func ProductionQueueConfig() *QueueConfig {
      250    config := DefaultQueueConfig()

● Now add the repo_sync queue to RiverQueueConfig():

● Update(internal/jobqueue/queue_config.go)
  ⎿  Added 10 lines
      335      }
      336    }
      337  
      338 +  repoSyncWorkers := c.RepoSyncConfig.MaxWorkers              
      339 +  if repoSyncWorkers <= 0 {                                   
      340 +    repoSyncWorkers = 5                                       
      341 +  }                                                           
      342 +                                                              
      343    return map[string]river.QueueConfig{
      344      river.QueueDefault: {
      345        MaxWorkers: c.MaxWorkers,
     ...
      347      "review": {
      348        MaxWorkers: reviewWorkers,
      349      },
      350 +    // Repo/PR discovery and sync jobs run on their own queue 
          +so they can                                                   
      351 +    // never starve AI-review job concurrency on the "review" 
          +queue.                                                        
      352 +    "repo_sync": {                                            
      353 +      MaxWorkers: repoSyncWorkers,                            
      354 +    },                                                        
      355    }
      356  }

● Now let's check os/strconv are already imported in queue_config.go.

  Searched for 1 pattern, ran 1 shell command (ctrl+o to expand)

● Already imported. Now build to check for errors.

  Ran 1 shell command (ctrl+o to expand)

● Now let's wire the new workers and PeriodicJobs into NewJobQueue.

● Update(internal/jobqueue/jobqueue.go)
  ⎿  Added 28 lines
      2301    webhookWorker := &WebhookReviewWorker{}
      2302    manualWorker := &ManualReviewWorker{}
      2303    diffWorker := &DiffReviewWorker{db: db, pool: pool}
      2304 +  prStore := pullrequests.NewStore(db)                       
      2305 +  repoPRSyncWorker := &RepoPRSyncWorker{db: db, store: prStor
           +e}                                                           
      2306 +  prStateSyncWorker := &PRStateSyncWorker{db: db, store: prSt
           +ore}                                                         
      2307 +  reconciliationWorker := &ReconciliationSweepWorker{db: db, 
           +pool: pool, stalenessThreshold: config.RepoSyncConfig.Stalene
           +ssThreshold}                                                 
      2308    river.AddWorker(workers, &WebhookInstallWorker{pool: pool, 
            config: config, store: store, httpClient: httpClient})
      2309    river.AddWorker(workers, &WebhookRemovalWorker{pool: pool, 
            config: config, store: store, httpClient: httpClient})
      2310    river.AddWorker(workers, diffWorker)
      2311    river.AddWorker(workers, webhookWorker)
      2312    river.AddWorker(workers, manualWorker)
      2313    river.AddWorker(workers, &UpdateOrgUsageWorker{db: db, pool
            : pool})
      2314 +  river.AddWorker(workers, repoPRSyncWorker)                 
      2315 +  river.AddWorker(workers, prStateSyncWorker)                
      2316 +  river.AddWorker(workers, reconciliationWorker)             
      2317  
      2318 +  coordinatorInterval := config.RepoSyncConfig.CoordinatorInt
           +erval                                                        
      2319 +  if coordinatorInterval <= 0 {                              
      2320 +    coordinatorInterval = 15 * time.Minute                   
      2321 +  }                                                          
      2322 +                                                             
      2323    client, err := river.NewClient(riverpgxv5.New(pool), &river
            .Config{
      2324      Queues:                      config.RiverQueueConfig(),
      2325      Workers:                     workers,
      2326      CompletedJobRetentionPeriod: 365 * 24 * time.Hour,
      2327      CancelledJobRetentionPeriod: 365 * 24 * time.Hour,
      2328      DiscardedJobRetentionPeriod: 365 * 24 * time.Hour,
      2329 +    PeriodicJobs: []*river.PeriodicJob{                      
      2330 +      river.NewPeriodicJob(                                  
      2331 +        river.PeriodicInterval(coordinatorInterval),         
      2332 +        func() (river.JobArgs, *river.InsertOpts) {          
      2333 +          return ReconciliationSweepJobArgs{}, &river.InsertO
           +pts{                                                         
      2334 +            Queue:       "repo_sync",                        
      2335 +            MaxAttempts: 3,                                  
      2336 +            UniqueOpts: river.UniqueOpts{                    
      2337 +              ByPeriod: coordinatorInterval - time.Minute,   
      2338 +            },                                               
      2339 +          }                                                  
      2340 +        },                                                   
      2341 +        &river.PeriodicJobOpts{RunOnStart: false},           
      2342 +      ),                                                     
      2343 +    },                                                       
      2344    })
      2345    if err != nil {
      2346      return nil, fmt.Errorf("failed to create River client: %w
            ", err)
     ...
      2355    webhookWorker.jq = jq
      2356    manualWorker.jq = jq
      2357    diffWorker.jq = jq
      2358 +  reconciliationWorker.jq = jq                               
      2359  
      2360    return jq, nil
      2361  }
● Now let's add the missing import.

● Update(internal/jobqueue/jobqueue.go)
  ⎿  Added 1 line
      37    "github.com/livereview/internal/providers/gitea"
      38    networkjobqueue "github.com/livereview/network/jobqueue"
      39    storagejobqueue "github.com/livereview/storage/jobqueue"
      40 +  "github.com/livereview/storage/providers/pullrequests"       
      41    "github.com/riverqueue/river"
      42    "github.com/riverqueue/river/riverdriver/riverpgxv5"
      43  )

  Ran 1 shell command (ctrl+o to expand)

● Let's build the whole project to check for regressions.

  Ran 1 shell command (ctrl+o to expand)

● Clean build across the project. Now let's write tests for the periodic job
  registration and the coordinator's staleness query.

  Listed 1 directory, ran 1 shell command (ctrl+o to expand)

● Write(internal/jobqueue/repo_sync_worker_test.go)
  ⎿  Wrote 140 lines to internal/jobqueue/repo_sync_worker_test.go
      1 package jobqueue
      2 
      3 import (
      4   "context"
      5   "database/sql"
      6   "encoding/json"
      7   "os"
      8   "testing"
      9   "time"
     … +150 lines (ctrl+o to expand)

● Let me clean up that awkward helper function.

● Update(internal/jobqueue/repo_sync_worker_test.go)
  ⎿  Added 7 lines, removed 10 lines
      13    "github.com/riverqueue/river"
      14  )
      15  
      16 +// testDBAndURL returns a live *sql.DB plus the raw connection 
         +string River's                                                 
      17 +// pgxpool needs. Unlike internal/database.NewDB() (which also 
         +accepts a                                                      
      18 +// .env-file fallback), NewJobQueue's pgxpool.New requires the 
         +URL as a plain                                                 
      19 +// string, so this test helper requires DATABASE_URL to be expo
         +rted directly.                                                 
      20  func testDBAndURL(t *testing.T) (*sql.DB, string) {
      21    t.Helper()
      22    dbURL := os.Getenv("DATABASE_URL")
      23 +  if dbURL == "" {                                             
      24 +    t.Skip("skipping: DATABASE_URL must be exported for River c
         +lient construction")                                           
      25 +  }                                                            
      26    db, err := database.NewDB()
      27    if err != nil {
      28      t.Skipf("skipping: no database available: %v", err)
      29    }
      30    t.Cleanup(func() { db.Close() })
      24 -  if dbURL == "" {                                             
      25 -    // database.NewDB() itself resolved the URL from .env; re-d
         -erive it the                                                   
      26 -    // same way NewJobQueue expects it (a plain connection stri
         -ng).                                                           
      27 -    var connStr string                                         
      28 -    if err := db.QueryRow(`SELECT current_setting('livereview.t
         -est_no_such_setting', true)`).Scan(&connStr); err != nil {     
      29 -      // Expected: the setting doesn't exist. Fall back to requ
         -iring                                                          
      30 -      // DATABASE_URL to be exported for the pgxpool-based Rive
         -r client.                                                      
      31 -    }                                                          
      32 -    t.Skip("skipping: DATABASE_URL must be exported for River c
         -lient construction")                                           
      33 -  }                                                            
      31    return db, dbURL
      32  }
      33  

● Now let's build and run these tests with DATABASE_URL exported.

  Ran 2 shell commands (ctrl+o to expand)

● Both pass — the coordinator correctly queued the never-synced and stale repos
  while skipping the fresh one. Let's run the full jobqueue test suite to check
  for regressions.

  Ran 2 shell commands (ctrl+o to expand)

● Clean. Step 5 complete.

● Now step 6: webhook handling. Let me examine the GitHub webhook provider in
  detail.

  Searched for 2 patterns, read 1 file (ctrl+o to expand)

● Now let's find GitHubV2WebhookPayload struct definition to see what fields
  already exist (PullRequest, Repository, Action, etc.).

  Searched for 1 pattern, read 1 file (ctrl+o to expand)

● Now let's add Merged and Draft fields to GitHubV2PullRequest.

● Update(internal/provider_input/github/github_types.go)
  ⎿  Added 2 lines
      19    Title              string         `json:"title"`
      20    Body               string         `json:"body"`
      21    State              string         `json:"state"`
      22 +  Merged             bool           `json:"merged"`            
      23 +  Draft              bool           `json:"draft"`             
      24    HTMLURL            string         `json:"html_url"`
      25    CreatedAt          string         `json:"created_at"`
      26    UpdatedAt          string         `json:"updated_at"`

  Searched for 1 pattern (ctrl+o to expand)

● No existing helper — I'll write one locally. Now let's write the GitHub
  ConvertPullRequestStateEvent.

● Write(internal/provider_input/github/github_pr_state_event.go)
  ⎿  Wrote 74 lines to
     internal/provider_input/github/github_pr_state_event.go
       1 package github
       2 
       3 import (
       4   "encoding/json"
       5   "fmt"
       6   "time"
       7 
       8   "github.com/livereview/internal/prsync"
       9   "github.com/livereview/internal/webhookutils"
      10 )
     … +81 lines (ctrl+o to expand)

  Ran 1 shell command (ctrl+o to expand)
● Now let's look at the GitLab side.

  Searched for 1 pattern, read 1 file (ctrl+o to expand)

● Write(internal/provider_input/gitlab/gitlab_pr_state_event.go)
  ⎿  Wrote 80 lines to
     internal/provider_input/gitlab/gitlab_pr_state_event.go
      1 package gitlab
      2 
      3 import (
      4   "encoding/json"
      5   "fmt"
      6   "time"
      7 
      8   "github.com/livereview/internal/prsync"
      9 )
     … +83 lines (ctrl+o to expand)

  Ran 1 shell command (ctrl+o to expand)

● Good, builds clean. Now let's wire the short-circuit branch into
  WebhookOrchestratorV2.ProcessWebhookEvent. Let me check the relevant imports
  and auth package functions already used.

  Searched for 2 patterns (ctrl+o to expand)

● Update(internal/api/webhook_orchestrator_v2.go)
  ⎿  Added 44 lines
      128  
      129    log.Printf("[INFO] Detected provider: %s", providerName)
      130  
      131 +  // Phase 1.5: PR/MR state-sync short-circuit. GitHub's "pull
          +_request" and                                                 
      132 +  // GitLab's "merge_request" webhook events are already deliv
          +ered to this                                                  
      133 +  // endpoint today (LiveReview's webhook installer subscribes
          + to them), but                                                
      134 +  // convertToUnifiedEvent below only recognizes comment/revie
          +wer-shaped                                                    
      135 +  // events, so these currently either fail outright (GitHub: 
          +HTTP 400) or                                                  
      136 +  // get silently swallowed as an empty-body no-op (GitLab). H
          +andle them here                                               
      137 +  // instead, bypassing the AI-response pipeline entirely (bot
          +-info lookup,                                                 
      138 +  // response-warrant check, LOC preflight are all irrelevant 
          +to a state                                                    
      139 +  // upsert), and return before any of that runs.             
      140 +  if converter, ok := provider.(prStateConverter); ok {       
      141 +    if stateEvent, matched, convErr := converter.ConvertPullRe
          +questStateEvent(headers, bodyBytes); matched {                
      142 +      if convErr != nil {                                     
      143 +        log.Printf("[ERROR] Failed to convert PR/MR state even
          +t (provider=%s): %v", providerName, convErr)                  
      144 +        return c.JSON(http.StatusBadRequest, map[string]string
          +{                                                             
      145 +          "error":    "failed to process pr state webhook",   
      146 +          "provider": providerName,                           
      147 +        })                                                    
      148 +      }                                                       
      149 +      connectorID, ok := auth.GetConnectorIDFromContext(c)    
      150 +      if !ok {                                                
      151 +        log.Printf("[ERROR] Connector ID not found in context 
          +- webhook route configuration error")                         
      152 +        return c.JSON(http.StatusInternalServerError, map[stri
          +ng]string{"error": "internal_error"})                         
      153 +      }                                                       
      154 +      orgID, ok := auth.GetOrgIDFromContext(c)                
      155 +      if !ok {                                                
      156 +        log.Printf("[ERROR] Org ID not found in context - midd
          +leware configuration error")                                  
      157 +        return c.JSON(http.StatusInternalServerError, map[stri
          +ng]string{"error": "internal_error"})                         
      158 +      }                                                       
      159 +      if wo.server.jobQueue == nil {                          
      160 +        log.Printf("[ERROR] Job queue not initialized, cannot 
          +process PR state event")                                      
      161 +        return c.JSON(http.StatusInternalServerError, map[stri
          +ng]string{"error": "internal_error"})                         
      162 +      }                                                       
      163 +      if err := wo.server.jobQueue.QueuePRStateSyncJob(c.Reque
          +st().Context(), orgID, connectorID, providerName, *stateEvent)
          +; err != nil {                                                
      164 +        log.Printf("[ERROR] Failed to queue PR state sync job:
          + %v", err)                                                    
      165 +        return c.JSON(http.StatusInternalServerError, map[stri
          +ng]string{"error": "failed to queue pr state sync"})          
      166 +      }                                                       
      167 +      return c.JSON(http.StatusOK, map[string]string{         
      168 +        "status":     "accepted",                             
      169 +        "provider":   providerName,                           
      170 +        "event_type": "pr_state_sync",                        
      171 +      })                                                      
      172 +    }                                                         
      173 +  }                                                           
      174 +                                                              
      175    // Phase 2: Convert to Unified Event Structure
      176    event, err := wo.convertToUnifiedEvent(provider, headers, bo
           dyBytes)
      177    if err != nil {

  Searched for 1 pattern (ctrl+o to expand)
  ⎿  Found 1 new diagnostic issue in 1 file (ctrl+o to expand)

● Update(internal/api/webhook_orchestrator_v2.go)
  ⎿  Added 1 line
      21    "github.com/livereview/internal/lrcfetch"
      22    "github.com/livereview/internal/prompts"
      23    gitlabinput "github.com/livereview/internal/provider_input/gi
          tlab"
      24 +  "github.com/livereview/internal/prsync"                      
      25    storagelicense "github.com/livereview/storage/license"
      26  )
      27  

● Now add the prStateConverter interface type definition near
  ProcessWebhookEvent.

● Update(internal/api/webhook_orchestrator_v2.go)
  ⎿  Added 9 lines
       90    return orchestrator
       91  }
       92  
       93 +// prStateConverter is implemented by webhook providers that c
          +an recognize a                                                
       94 +// PR/MR lifecycle event (GitHub "pull_request", GitLab "merge
          +_request") and                                                
       95 +// convert it to a normalized prsync.PullRequestStateEvent. ma
          +tched=false (with                                             
       96 +// a nil error) means this payload isn't one of those events, 
          +so the caller                                                 
       97 +// should fall through to the normal comment/reviewer conversi
          +on path.                                                      
       98 +type prStateConverter interface {                             
       99 +  ConvertPullRequestStateEvent(headers map[string]string, body
          + []byte) (*prsync.PullRequestStateEvent, bool, error)         
      100 +}                                                             
      101 +                                                              
      102  // ProcessWebhookEvent is the main entry point for webhook pro
           cessing (replaces individual handlers)
      103  func (wo *WebhookOrchestratorV2) ProcessWebhookEvent(c echo.Co
           ntext) error {
      104    startTime := time.Now()

  Ran 1 shell command (ctrl+o to expand)

● Clean build. Now let's build the whole project and run the existing
  webhook-related test suites to check for regressions.

  Ran 1 shell command (ctrl+o to expand)

● Whole project builds clean. Now let's write the regression/unit tests for the
  two ConvertPullRequestStateEvent implementations.

  Searched for 1 pattern, listed 1 directory (ctrl+o to expand)

● Write(internal/provider_input/github/github_pr_state_event_test.go)
  ⎿  Wrote 115 lines to
     internal/provider_input/github/github_pr_state_event_test.go
       1 package github
       2 
       3 import "testing"
       4 
       5 func pullRequestPayload(action, state string, merged bool) []byt
       6   merged_str := "false"
       7   if merged {
       8     merged_str = "true"
       9   }
      10   return []byte(`{
     … +115 lines (ctrl+o to expand)

● Update(internal/provider_input/github/github_pr_state_event_test.go)
  ⎿  Added 2 lines, removed 2 lines
       3  import "testing"
       4  
       5  func pullRequestPayload(action, state string, merged bool) []by
          te {
       6 -  merged_str := "false"                                        
       6 +  mergedStr := "false"                                         
       7    if merged {
       8 -    merged_str = "true"                                        
       8 +    mergedStr = "true"                                         
       9    }
      10    return []byte(`{
      11      "action": "` + action + `",

● Update(internal/provider_input/github/github_pr_state_event_test.go)
  ⎿  Added 1 line, removed 1 line
      16        "title": "Add feature",
      17        "body": "description here",
      18        "state": "` + state + `",
      19 -      "merged": ` + merged_str + `,                            
      19 +      "merged": ` + mergedStr + `,                             
      20        "draft": false,
      21        "html_url": "https://github.com/acme/repo/pull/42",
      22        "created_at": "2026-01-01T00:00:00Z",
● Now the GitLab test file.

● Write(internal/provider_input/gitlab/gitlab_pr_state_event_test.go)
  ⎿  Wrote 32 lines to
     internal/provider_input/gitlab/gitlab_pr_state_event_test.go
       1 package gitlab
       2 
       3 func mergeRequestWebhookBody(action, state string) []byte {
       4   return []byte(`{
       5     "object_kind": "merge_request",
       7     "project": {"id": 77, "path_with_namespace": "group/repo", "
         web_url": "https://gitlab.com/group/repo"},
       8     "object_attributes": {
       9       "id": 888,
      10       "iid": 5,
     … +24 lines (ctrl+o to expand)

● I forgot the actual test functions — let me fix that.

● Write(internal/provider_input/gitlab/gitlab_pr_state_event_test.go)
  ⎿  Added 65 lines
      1  package gitlab
      2  
      3 +import "testing"                                                
      4 +                                                                
      5  func mergeRequestWebhookBody(action, state string) []byte {
      6    return []byte(`{
      7      "object_kind": "merge_request",
     ...
      32      "object_attributes": {"id": 1, "note": "a comment", "noteab
          le_type": "MergeRequest"}
      33    }`)
      34  }
      35 +                                                               
      36 +func TestConvertPullRequestStateEvent_Opened(t *testing.T) {   
      37 +  p := &GitLabV2Provider{}                                     
      38 +  event, matched, err := p.ConvertPullRequestStateEvent(       
      39 +    map[string]string{"X-Gitlab-Event": "Merge Request Hook"}, 
      40 +    mergeRequestWebhookBody("open", "opened"),                 
      41 +  )                                                            
      42 +  if err != nil {                                              
      43 +    t.Fatalf("unexpected error: %v", err)                      
      44 +  }                                                            
      45 +  if !matched {                                                
      46 +    t.Fatal("expected matched=true for merge_request object_kin
         +d")                                                            
      47 +  }                                                            
      48 +  if event.State != "open" {                                   
      49 +    t.Errorf("expected state open, got %s", event.State)       
      50 +  }                                                            
      51 +  if event.RepositoryProviderID != "77" || event.RepositoryFull
         +Name != "group/repo" {                                         
      52 +    t.Errorf("unexpected repository identity: %+v", event)     
      53 +  }                                                            
      54 +  if event.Number != 5 || event.ProviderPRID != "888" {        
      55 +    t.Errorf("unexpected MR identity: %+v", event)             
      56 +  }                                                            
      57 +  if event.SourceBranch != "feature-branch" || event.TargetBran
         +ch != "main" {                                                 
      58 +    t.Errorf("unexpected branches: %+v", event)                
      59 +  }                                                            
      60 +  if event.AuthorID != "9" {                                   
      61 +    t.Errorf("expected author id 9, got %s", event.AuthorID)   
      62 +  }                                                            
      63 +}                                                              
      64 +                                                               
      65 +func TestConvertPullRequestStateEvent_Merged(t *testing.T) {   
      66 +  p := &GitLabV2Provider{}                                     
      67 +  event, matched, err := p.ConvertPullRequestStateEvent(       
      68 +    map[string]string{"X-Gitlab-Event": "Merge Request Hook"}, 
      69 +    mergeRequestWebhookBody("merge", "merged"),                
      70 +  )                                                            
      71 +  if err != nil {                                              
      72 +    t.Fatalf("unexpected error: %v", err)                      
      73 +  }                                                            
      74 +  if !matched {                                                
      75 +    t.Fatal("expected matched=true")                           
      76 +  }                                                            
      77 +  if event.State != "merged" {                                 
      78 +    t.Errorf("expected state merged, got %s", event.State)     
      79 +  }                                                            
      80 +}                                                              
      81 +                                                               
      82 +// TestConvertPullRequestStateEvent_IgnoresNoteEvents is the cr
         +itical                                                         
      83 +// regression guard: comment ("note") events must NOT be interc
         +epted here, so                                                 
      84 +// ConvertCommentEvent continues to handle them unchanged.     
      85 +func TestConvertPullRequestStateEvent_IgnoresNoteEvents(t *test
         +ing.T) {                                                       
      86 +  p := &GitLabV2Provider{}                                     
      87 +  event, matched, err := p.ConvertPullRequestStateEvent(       
      88 +    map[string]string{"X-Gitlab-Event": "Note Hook"},          
      89 +    noteWebhookBody(),                                         
      90 +  )                                                            
      91 +  if err != nil {                                              
      92 +    t.Fatalf("unexpected error: %v", err)                      
      93 +  }                                                            
      94 +  if matched {                                                 
      95 +    t.Fatalf("expected matched=false for a note event, got even
         +t=%+v", event)                                                 
      96 +  }                                                            
      97 +}                                                              

● Now let's run these tests.

  Ran 2 shell commands (ctrl+o to expand)

● All pass. Now let's run the full existing test suites for both packages and
  internal/api to confirm no regressions in existing comment/reviewer flows.

  Ran 5 shell commands (ctrl+o to expand)

● Confirmed pre-existing and unrelated — those Bitbucket capture fixtures aren't
  tracked in git at all, so this failure predates my session. My github/gitlab
  packages pass cleanly. Let's check the internal/api package tests for
  webhook-orchestrator regressions.

  Ran 4 shell commands (ctrl+o to expand)

● Both failures are unrelated to my changes — one is a billing/proration test
  (numeric mismatch suggesting stale pricing fixture data), the other fails due
  to a missing Bitbucket integration token row in this dev DB. Neither touches
  code I modified. All tests for the code I changed (GitHub, GitLab, jobqueue,
  prsync, providers, storage) pass cleanly. Step 6 is done.

● Now step 7: extending the auto-installer for initial backfill. Let me view the
  full file.

  Read 1 file (ctrl+o to expand)

● Now let's make the edits.

● Update(internal/api/auto_webhook_installer.go)
  ⎿  Added 3 lines
       9    "strings"
      10    "time"
      11  
      12 +  "github.com/livereview/internal/providers"                   
      13    "github.com/livereview/internal/providers/azuredevops"
      14    "github.com/livereview/internal/providers/bitbucket"
      15    "github.com/livereview/internal/providers/gitea"
      16    "github.com/livereview/internal/providers/github"
      17    "github.com/livereview/internal/providers/gitlab"
      18 +  "github.com/livereview/storage/providers/pullrequests"       
      19  )
      20  
      21  // AutoWebhookInstaller handles automatic webhook installation 
          for new connectors
     ...
      28  // JobQueueInterface allows for easier testing and decoupling
      29  type JobQueueInterface interface {
      30    QueueWebhookInstallJob(ctx context.Context, connectorID int, 
          projectPath, provider, baseURL, pat string) error
      31 +  QueueRepoPRSyncJob(ctx context.Context, repositoryID int64, i
         +nitialBackfill bool) error                                     
      32  }
      33  
      34  // NewAutoWebhookInstaller creates a new auto webhook installer

● Now add OrgID to ConnectorDetails and its query:

● Update(internal/api/auto_webhook_installer.go)
  ⎿  Added 3 lines, removed 1 line
       99  // ConnectorDetails holds the essential connector information
      100  type ConnectorDetails struct {
      101    ID          int
      102 +  OrgID       int64                                           
      103    Provider    string
      104    ProviderURL string
      105    PATToken    string
     ...
      112    var metadataBytes []byte
      113  
      114    query := `
      114 -    SELECT id, provider, provider_url, pat_token, COALESCE(met
          -adata, '{}')                                                  
      115 +    SELECT id, org_id, provider, provider_url, pat_token, COAL
          +ESCE(metadata, '{}')                                          
      116      FROM integration_tokens
      117      WHERE id = $1
      118    `
      119  
      120    err := awi.db.QueryRow(query, connectorID).Scan(
      121      &connector.ID,
      122 +    &connector.OrgID,                                         
      123      &connector.Provider,
      124      &connector.ProviderURL,
      125      &connector.PATToken,

● Now let's add the new backfill method and wire it into
  processAutoInstallation:

● Update(internal/api/auto_webhook_installer.go)
  ⎿  Added 72 lines
       92  
       93    log.Printf("Discovered %d projects for connector %d, startin
           g webhook installation", len(projects), connectorID)
       94  
       95 +  // Step 3.5: Populate the repositories table and queue an in
          +itial PR/MR                                                   
       96 +  // backfill sync for each repo (GitHub/GitLab only - the uni
          +fied repo/PR                                                  
       97 +  // listing feature is not yet built out for other providers)
          +. Best-effort:                                                
       98 +  // failures here are logged but never fail the connector set
          +up, since the                                                 
       99 +  // pre-existing webhook installation flow (Step 4) must not 
          +regress.                                                      
      100 +  if strings.HasPrefix(connector.Provider, "github") || string
          +s.HasPrefix(connector.Provider, "gitlab") {                   
      101 +    if err := awi.syncRepositoriesAndQueueBackfill(ctx, connec
          +torID, connector); err != nil {                               
      102 +      log.Printf("Warning: repository/PR backfill setup failed
          + for connector %d: %v", connectorID, err)                     
      103 +    }                                                         
      104 +  }                                                           
      105 +                                                              
      106    // Step 4: Queue webhook installation jobs for all projects
      107    return awi.queueWebhookInstallations(ctx, connectorID, proje
           cts, connector)
      108  }
      109  
      110 +// syncRepositoriesAndQueueBackfill discovers repositories wit
          +h full metadata                                               
      111 +// (unlike the name-only discoverAndCacheProjects), upserts th
          +em into the                                                   
      112 +// repositories table, and queues an initial PR/MR backfill sy
          +nc job for each                                               
      113 +// one - so a newly connected org/group is immediately populat
          +ed instead of                                                 
      114 +// waiting for the first periodic reconciliation sweep.       
      115 +func (awi *AutoWebhookInstaller) syncRepositoriesAndQueueBackf
          +ill(ctx context.Context, connectorID int, connector *Connector
          +Details) error {                                              
      116 +  store := pullrequests.NewStore(awi.db)                      
      117 +                                                              
      118 +  cursor := ""                                                
      119 +  total := 0                                                  
      120 +  for {                                                       
      121 +    var page *providers.RepositoryPage                        
      122 +    var err error                                             
      123 +    switch {                                                  
      124 +    case strings.HasPrefix(connector.Provider, "github"):     
      125 +      page, err = github.ListRepositories(ctx, connector.Provi
          +derURL, connector.PATToken, cursor)                           
      126 +    case strings.HasPrefix(connector.Provider, "gitlab"):     
      127 +      page, err = gitlab.ListRepositories(ctx, connector.Provi
          +derURL, connector.PATToken, cursor)                           
      128 +    default:                                                  
      129 +      return fmt.Errorf("unsupported provider for repository s
          +ync: %s", connector.Provider)                                 
      130 +    }                                                         
      131 +    if err != nil {                                           
      132 +      return fmt.Errorf("failed to list repositories: %w", err
          +)                                                             
      133 +    }                                                         
      134 +                                                              
      135 +    for _, repo := range page.Repositories {                  
      136 +      repositoryID, err := store.UpsertRepository(pullrequests
          +.RepositoryUpsert{                                            
      137 +        OrgID:          connector.OrgID,                      
      138 +        ConnectorID:    int64(connectorID),                   
      139 +        Provider:       connector.Provider,                   
      140 +        ProviderRepoID: repo.ProviderRepoID,                  
      141 +        FullName:       repo.FullName,                        
      142 +        Name:           repo.Name,                            
      143 +        WebURL:         repo.WebURL,                          
      144 +        CloneURL:       repo.CloneURL,                        
      145 +        SSHURL:         repo.SSHURL,                          
      146 +        DefaultBranch:  repo.DefaultBranch,                   
      147 +        IsPrivate:      repo.IsPrivate,                       
      148 +        Description:    repo.Description,                     
      149 +      })                                                      
      150 +      if err != nil {                                         
      151 +        log.Printf("Warning: failed to upsert repository %s fo
          +r connector %d: %v", repo.FullName, connectorID, err)         
      152 +        continue                                              
      153 +      }                                                       
      154 +      if err := awi.jobQueue.QueueRepoPRSyncJob(ctx, repositor
          +yID, true); err != nil {                                      
      155 +        log.Printf("Warning: failed to queue initial PR backfi
          +ll for repository %s (id=%d): %v", repo.FullName, repositoryID
          +, err)                                                        
      156 +        continue                                              
      157 +      }                                                       
      158 +      total++                                                 
      159 +    }                                                         
      160 +                                                              
      161 +    if page.NextCursor == "" {                                
      162 +      break                                                   
      163 +    }                                                         
      164 +    cursor = page.NextCursor                                  
      165 +  }                                                           
      166 +                                                              
      167 +  log.Printf("Queued initial PR/MR backfill for %d repositorie
          +s (connector %d)", total, connectorID)                        
      168 +  return nil                                                  
      169 +}                                                             
      170 +                                                              
      171  // ConnectorDetails holds the essential connector information
      172  type ConnectorDetails struct {
      173    ID          int
● Now let's build.

  Searched for 1 pattern, read 1 file, ran 3 shell commands (ctrl+o to expand)

● Write(internal/api/auto_webhook_installer_repo_sync_test.go)
  ⎿  Wrote 79 lines to
     internal/api/auto_webhook_installer_repo_sync_test.go
       1 package api
       2 
       3 import (
       5   "net/http"
       6   "testing"
       7 )
       8 
         g a real River
      10 // queue, so syncRepositoriesAndQueueBackfill can be tested in i
         solation.
     … +86 lines (ctrl+o to expand)

  Searched for 2 patterns (ctrl+o to expand)
  ⎿  Found 1 new diagnostic issue in 1 file (ctrl+o to expand)

● Let me inline it directly, matching the existing convention.

● Update(internal/api/auto_webhook_installer_repo_sync_test.go)
  ⎿  Added 2 lines
       2  
       3  import (
       4    "context"
       5 +  "io"                                                         
       6    "net/http"
       7 +  "strings"                                                    
       8    "testing"
       9  )
      10  

● Update(internal/api/auto_webhook_installer_repo_sync_test.go)
  ⎿  Added 5 lines, removed 1 line
      34        t.Fatalf("unexpected request: %s %s", req.Method, req.URL
          .String())
      35      }
      36      body := `[{"id":501,"name":"repo-a","full_name":"acme/repo-
          a","html_url":"https://github.com/acme/repo-a","default_branch"
          :"main","private":true}]`
      37 -    return jsonResponseForAPITest(200, body, nil), nil         
      37 +    return &http.Response{                                     
      38 +      StatusCode: http.StatusOK,                               
      39 +      Body:       io.NopCloser(strings.NewReader(body)),       
      40 +      Header:     make(http.Header),                           
      41 +    }, nil                                                     
      42    })
      43  
      44    var connector ConnectorDetails

● Now let's build and run this test.

  Ran 2 shell commands (ctrl+o to expand)

● Test passes. Let's run the broader internal/api suite once more to confirm the
  two pre-existing unrelated failures are the only ones (no new regressions
  from this step).

  Ran 1 shell command (ctrl+o to expand)

● Only the same two pre-existing unrelated failures. Step 7 complete.

● Now step 8: API endpoints. Let me examine the route registration and existing
  patterns closely.

  Searched for 2 patterns, read 1 file (ctrl+o to expand)

● Let me extract the LOC-preflight block into a shared helper so I can reuse it
  without duplicating billing logic, then build the new PR-based review trigger.

  Read 1 file (ctrl+o to expand)

● Update(internal/api/review_service.go)
  ⎿  Added 2 lines, removed 37 lines
       68    log.Printf("[DEBUG] TriggerReviewV2: Starting review request
            handling")
       69  
       70    // LOC Quota preflight check — block before creating any DB 
           records
       71 -  // Only run LOC quota preflight in Cloud Mode               
       72 -  if apimiddleware.IsCloudMode() {                            
       73 -    orgID, orgOK := c.Get("org_id").(int64)                   
       74 -    planCode := license.PlanFree30K                           
       75 -    if planCtx, ok := c.Get(apimiddleware.PlanContextKey).(api
          -middleware.PlanContext); ok && planCtx.PlanType != "" {       
       76 -      planCode = planCtx.PlanType                             
       77 -    }                                                         
       78 -    if orgOK && orgID > 0 {                                   
       79 -      accountingService := license.NewLOCAccountingService(s.d
          -b)                                                            
       80 -      preflightResult, pfErr := accountingService.CheckPreflig
          -ht(context.Background(), license.LOCPreflightInput{           
       81 -        OrgID:       orgID,                                   
       82 -        RequiredLOC: 0, // unknown at this point, just check c
          -urrent state                                                  
       83 -        PlanCode:    planCode,                                
       84 -      })                                                      
       85 -      if pfErr != nil {                                       
       86 -        log.Printf("[WARN] LOC preflight check failed for org=
          -%d: %v", orgID, pfErr)                                        
       87 -      } else {                                                
       88 -        applyPreflightToEnvelopeContext(c, preflightResult)   
       89 -        if preflightResult.Blocked {                          
       90 -          errorCode := "quota_exceeded"                       
       91 -          errorMessage := "monthly LOC quota exceeded for this
          - organization"                                                
       92 -          if preflightResult.BlockReason == "trial_readonly" {
       93 -            errorCode = "trial_readonly"                      
       94 -            errorMessage = "trial period ended; review operati
          -ons are read-only until plan update"                          
       95 -          }                                                   
       96 -          log.Printf("[INFO] TriggerReviewV2: LOC quota blocke
          -d for org=%d, used=%d, limit=%d",                             
       97 -            orgID, preflightResult.LOCUsedMonth, preflightResu
          -lt.LOCLimitMonth)                                             
       98 -          return JSONWithEnvelope(c, http.StatusForbidden, map
          -[string]interface{}{                                          
       99 -            "error":         errorMessage,                    
      100 -            "error_code":    errorCode,                       
      101 -            "loc_remaining": preflightResult.LOCRemainingMonth
          -,                                                             
      102 -            "usage_percent": preflightResult.UsagePercent,    
      103 -            "upgrade_url":   defaultUpgradeURL,               
      104 -          })                                                  
      105 -        }                                                     
      106 -      }                                                       
      107 -    }                                                         
       71 +  if blocked, pfErr := s.preflightLOCQuota(c); blocked {      
       72 +    return pfErr                                              
       73    }
       74  
       75    // Phase 1: Setup review context (org_id, parse request, cre
           ate DB record, init logger)

● Now add the extracted preflightLOCQuota method:

● Update(internal/api/review_service.go)
  ⎿  Added 51 lines
      172    return JSONWithEnvelope(c, http.StatusOK, response)
      173  }
      174  
      175 +// preflightLOCQuota runs the LOC quota preflight check (Cloud
          + Mode only) for                                               
      176 +// the org in the request context. If the org is blocked, it w
          +rites the 403                                                 
      177 +// JSON response itself and returns blocked=true; the caller s
          +hould return                                                  
      178 +// the accompanying error value (nil on a successfully-written
          + response)                                                    
      179 +// without further processing. Shared by TriggerReviewV2 and  
      180 +// createReviewForPullRequest so both review-trigger entrypoin
          +ts enforce                                                    
      181 +// billing identically.                                       
      182 +func (s *Server) preflightLOCQuota(c echo.Context) (blocked bo
          +ol, err error) {                                              
      183 +  if !apimiddleware.IsCloudMode() {                           
      184 +    return false, nil                                         
      185 +  }                                                           
      186 +  orgID, orgOK := c.Get("org_id").(int64)                     
      187 +  if !orgOK || orgID <= 0 {                                   
      188 +    return false, nil                                         
      189 +  }                                                           
      190 +  planCode := license.PlanFree30K                             
      191 +  if planCtx, ok := c.Get(apimiddleware.PlanContextKey).(apimi
          +ddleware.PlanContext); ok && planCtx.PlanType != "" {         
      192 +    planCode = planCtx.PlanType                               
      193 +  }                                                           
      194 +                                                              
      195 +  accountingService := license.NewLOCAccountingService(s.db)  
      196 +  preflightResult, pfErr := accountingService.CheckPreflight(c
          +ontext.Background(), license.LOCPreflightInput{               
      197 +    OrgID:       orgID,                                       
      198 +    RequiredLOC: 0, // unknown at this point, just check curre
          +nt state                                                      
      199 +    PlanCode:    planCode,                                    
      200 +  })                                                          
      201 +  if pfErr != nil {                                           
      202 +    log.Printf("[WARN] LOC preflight check failed for org=%d: 
          +%v", orgID, pfErr)                                            
      203 +    return false, nil                                         
      204 +  }                                                           
      205 +  applyPreflightToEnvelopeContext(c, preflightResult)         
      206 +  if !preflightResult.Blocked {                               
      207 +    return false, nil                                         
      208 +  }                                                           
      209 +                                                              
      210 +  errorCode := "quota_exceeded"                               
      211 +  errorMessage := "monthly LOC quota exceeded for this organiz
          +ation"                                                        
      212 +  if preflightResult.BlockReason == "trial_readonly" {        
      213 +    errorCode = "trial_readonly"                              
      214 +    errorMessage = "trial period ended; review operations are 
          +read-only until plan update"                                  
      215 +  }                                                           
      216 +  log.Printf("[INFO] LOC quota blocked for org=%d, used=%d, li
          +mit=%d", orgID, preflightResult.LOCUsedMonth, preflightResult.
          +LOCLimitMonth)                                                
      217 +  return true, JSONWithEnvelope(c, http.StatusForbidden, map[s
          +tring]interface{}{                                            
      218 +    "error":         errorMessage,                            
      219 +    "error_code":    errorCode,                               
      220 +    "loc_remaining": preflightResult.LOCRemainingMonth,       
      221 +    "usage_percent": preflightResult.UsagePercent,            
      222 +    "upgrade_url":   defaultUpgradeURL,                       
      223 +  })                                                          
      224 +}                                                             
      225 +                                                              
      226  func optionalString(value string) *string {
      227    v := strings.TrimSpace(value)
      228    if v == "" {

  Ran 1 shell command (ctrl+o to expand)
● Let's verify this refactor didn't break the existing trigger-review flow.

  Searched for 1 pattern, ran 2 shell commands (ctrl+o to expand)

● Now let's write the new handler file that replaces the createReview stub.

● Write(internal/api/pull_request_review_trigger.go)
  ⎿  Wrote 89 lines to internal/api/pull_request_review_trigger.go
       1 package api
       2 
       3 import (
       4   "database/sql"
       5   "fmt"
       6   "log"
       7   "net/http"
       8 
       9   "github.com/labstack/echo/v4"
      10 )
     … +101 lines (ctrl+o to expand)

● Now let's remove the old stub createReview from server.go since it's now
  defined in the new file.

  Searched for 1 pattern, read 1 file (ctrl+o to expand)
  ⎿  Found 1 new diagnostic issue in 1 file (ctrl+o to expand)

● Update(internal/api/server.go)
  ⎿  Added 2 lines, removed 9 lines
      1735    return c.JSON(http.StatusOK, response)
      1736  }
      1737  
      1738 -// createReview handles POST /api/v1/reviews (trigger review 
           -creation)                                                    
      1739 -func (s *Server) createReview(c echo.Context) error {        
      1740 -  // This should delegate to the existing TriggerReviewV2 fun
           -ctionality                                                   
      1741 -  // For now, return a placeholder that explains how to trigg
           -er reviews                                                   
      1742 -  return c.JSON(http.StatusOK, map[string]string{            
      1743 -    "message": "Use POST /api/v1/connectors/trigger-review to
           - create reviews",                                            
      1744 -    "note":    "Direct review creation will be implemented in
           - a future update",                                           
      1745 -  })                                                         
      1746 -}                                                            
      1738 +// createReview handles POST /api/v1/reviews (trigger review 
           +creation from a                                              
      1739 +// known pull_requests row) - see pull_request_review_trigger
           +.go.                                                         
      1740  
      1741  // getReviewByID handles GET /api/v1/reviews/:id
      1742  func (s *Server) getReviewByID(c echo.Context) error {

  Read 1 file, ran 1 shell command (ctrl+o to expand)

● Write(internal/api/repositories_pr_routes.go)
  ⎿  Wrote 460 lines to internal/api/repositories_pr_routes.go
      1 package api
      2 
      3 import (
      4   "database/sql"
      5   "encoding/json"
      6   "fmt"
      7   "net/http"
      8   "strconv"
      9   "time"
     … +531 lines (ctrl+o to expand)

● Let me clean up that unused import hack.

● Update(internal/api/repositories_pr_routes.go)
  ⎿  Removed 1 line
      2  
      3  import (
      4    "database/sql"
      5 -  "encoding/json"                                               
      5    "fmt"
      6    "net/http"
      7    "strconv"

● Update(internal/api/repositories_pr_routes.go)
  ⎿  Removed 2 lines
      455    }
      456    return (total + perPage - 1) / perPage
      457  }
      458 -                                                              
      459 -var _ = json.Marshal // keep encoding/json import if response 
          -helpers change                                                

  Ran 3 shell commands (ctrl+o to expand)

● Now let's wire the new routes into server.go.

  Read 1 file (ctrl+o to expand)

● Update(internal/api/server.go)
  ⎿  Added 18 lines
       978    connectorGroup.GET("/:connectorId/repository-access", s.Get
            RepositoryAccess)
       979    connectorGroup.POST("/:connectorId/enable-manual-trigger", 
            s.EnableManualTriggerForAllProjects)
       980    connectorGroup.POST("/:connectorId/disable-manual-trigger",
             s.DisableManualTriggerForAllProjects)
       981 +  connectorGroup.POST("/:connectorId/repositories/sync", s.Tr
           +iggerRepositorySync)                                         
       982    connectorGroup.POST("/trigger-review", s.TriggerReviewV2, s
            elfHostedLicenseMiddleware)
       983  
       984 +  // Unified repository + PR/MR listing endpoints (organizati
           +on-scoped via                                                
       985 +  // headers, same middleware chain as connectorGroup)       
       986 +  repositoriesGroup := v1.Group("/repositories")             
       987 +  repositoriesGroup.Use(RequireAuthOrAPIKey(s.tokenService, s
           +.db))                                                        
       988 +  repositoriesGroup.Use(authMiddleware.BuildOrgContextFromHea
           +der())                                                       
       989 +  repositoriesGroup.Use(authMiddleware.ValidateOrgAccess())  
       990 +  repositoriesGroup.Use(authMiddleware.BuildPermissionContext
           +())                                                          
       991 +  repositoriesGroup.Use(authMiddleware.EnforceSubscriptionLim
           +its())                                                       
       992 +  repositoriesGroup.Use(apimiddleware.BuildOrgBillingPlanCont
           +ext(s.db, s.licenseService()))                               
       993 +  repositoriesGroup.Use(apimiddleware.BuildPlanContext())    
       994 +                                                             
       995 +  repositoriesGroup.GET("", s.ListRepositories)              
       996 +  repositoriesGroup.GET("/:repoId", s.GetRepository)         
       997 +  repositoriesGroup.POST("/:repoId/sync", s.TriggerRepository
           +PRSync)                                                      
       998 +  repositoriesGroup.GET("/:repoId/pull-requests", s.ListPullR
           +equestsForRepo)                                              
       999 +  repositoriesGroup.GET("/:repoId/pull-requests/:prId", s.Get
           +PullRequest)                                                 
      1000 +                                                             
      1001    // GitLab profile validation endpoint
      1002    v1.POST("/gitlab/validate-profile", s.ValidateGitLabProfile
            )
      1003  

  Ran 2 shell commands (ctrl+o to expand)

● Whole project builds clean. Now let's write API handler tests using
  httptest/the existing setupBotUserTestServer convention.

  Searched for 1 pattern, read 1 file (ctrl+o to expand)

● Write(internal/api/repositories_pr_routes_test.go)
  ⎿  Wrote 197 lines to internal/api/repositories_pr_routes_test.go
      1 package api
      2 
      3 import (
      4   "encoding/json"
      5   "net/http"
      6   "net/http/httptest"
      7   "strconv"
      8   "testing"
      9   "time"
     … +223 lines (ctrl+o to expand)
  Ran 3 shell commands (ctrl+o to expand)

● All 4 pass. Let's also add a quick test for the new createReview (PR-ID-based
  trigger) endpoint, then run the full suite once more.

  Searched for 1 pattern (ctrl+o to expand)

● Write(internal/api/pull_request_review_trigger_test.go)
  ⎿  Wrote 57 lines to internal/api/pull_request_review_trigger_test.go
       1 package api
       2 
       3 import (
       5   "strings"
       6   "testing"
       7 )
       8 
         llRequestNotFound
      10 // cover the org-scoped lookup logic unique to this handler (bef
         ore it
     … +59 lines (ctrl+o to expand)

● Let me simplify — construct the request directly with a body reader instead of
  the awkward post-hoc reassignment.

● Write(internal/api/pull_request_review_trigger_test.go)
  ⎿  Added 15 lines, removed 18 lines
       2  
       3  import (
       4    "net/http"
       5 +  "net/http/httptest"                                          
       6    "strings"
       7    "testing"
       8 +                                                               
       9 +  "github.com/labstack/echo/v4"                                
      10  )
      11  
      12  // TestCreateReview_MissingPullRequestID and TestCreateReview_P
          ullRequestNotFound
     ...
      17  // TriggerReviewV2 itself - the more mature, longer-standing en
          dpoint this
      18  // handler reuses - has no full end-to-end test coverage today,
           since doing
      19  // so requires a live River job queue and a stubbed provider AP
          I.
      20 +func newCreateReviewContext(orgID int64, body string) (echo.Con
         +text, *httptest.ResponseRecorder) {                            
      21 +  req := httptest.NewRequest(http.MethodPost, "/api/v1/reviews"
         +, strings.NewReader(body))                                     
      22 +  req.Header.Set("Content-Type", "application/json")           
      23 +  rec := httptest.NewRecorder()                                
      24 +  e := echo.New()                                              
      25 +  c := e.NewContext(req, rec)                                  
      26 +  c.Set("org_id", orgID)                                       
      27 +  return c, rec                                                
      28 +}                                                              
      29 +                                                               
      30  func TestCreateReview_MissingPullRequestID(t *testing.T) {
      31    server := setupBotUserTestServer(t)
      32    orgID := getAnyOrgID(t, server.db)
      33  
      21 -  c, rec := newTestEchoContext(http.MethodPost, "/api/v1/review
         -s", orgID, nil, nil)                                           
      22 -  c.Request().Header.Set("Content-Type", "application/json")   
      23 -  c.Request().Body = httpBodyFromString(`{}`)                  
      24 -                                                               
      34 +  c, rec := newCreateReviewContext(orgID, `{}`)                
      35    if err := server.createReview(c); err != nil {
      36      t.Fatalf("createReview: %v", err)
      37    }
     ...
      44    server := setupBotUserTestServer(t)
      45    orgID := getAnyOrgID(t, server.db)
      46  
      37 -  c, rec := newTestEchoContext(http.MethodPost, "/api/v1/review
         -s", orgID, nil, nil)                                           
      38 -  c.Request().Header.Set("Content-Type", "application/json")   
      39 -  c.Request().Body = httpBodyFromString(`{"pull_request_id": 99
         -999999}`)                                                      
      40 -                                                               
      47 +  c, rec := newCreateReviewContext(orgID, `{"pull_request_id": 
         +99999999}`)                                                    
      48    if err := server.createReview(c); err != nil {
      49      t.Fatalf("createReview: %v", err)
      50    }
     ...
      52      t.Fatalf("expected 404 for nonexistent pull_request_id, got
           %d: %s", rec.Code, rec.Body.String())
      53    }
      54  }
      48 -                                                               
      49 -func httpBodyFromString(s string) *bodyReadCloser {            
      50 -  return &bodyReadCloser{Reader: strings.NewReader(s)}         
      51 -}                                                              
      52 -                                                               
      53 -type bodyReadCloser struct {                                   
      54 -  *strings.Reader                                              
      55 -}                                                              
      56 -                                                               
      57 -func (b *bodyReadCloser) Close() error { return nil }          

  Ran 2 shell commands (ctrl+o to expand)

● Both pass. Let's run the full internal/api suite once more and the whole
  project build to confirm everything is still clean.

  Ran 1 shell command (ctrl+o to expand)

● Only the same two pre-existing unrelated failures, and the full build is
  clean. Step 8 complete.

● Now step 9: the one-time backfill script. Let me check an existing cmd/
  one-off script for convention.

  Searched for 4 patterns, listed 1 directory, ran 1 shell command (ctrl+o to expand)

● Write(cmd/backfill-review-pr-links/main.go)
  ⎿  Wrote 64 lines to cmd/backfill-review-pr-links/main.go
       1 // Command backfill-review-pr-links is a one-time maintenance sc
       2 // links pre-existing reviews rows to their canonical pull_reque
         sts row by
       4 //
         ature has had
       6 // a chance to populate pull_requests for the org's connected re
       7 // (either via the initial per-connector backfill or a few perio
         dic
         ll empty will
      10 //
     … +70 lines (ctrl+o to expand)

  Ran 1 shell command (ctrl+o to expand)

● Let's actually run it against the dev DB to validate it end-to-end (safe —
  it's an idempotent, informational UPDATE).

  Ran 1 shell command (ctrl+o to expand)

● Let's run a final gofmt check across everything I've created, then a
  comprehensive test pass.

  Ran 2 shell commands (ctrl+o to expand)

● All formatting is clean now. Let's do a full rebuild and comprehensive test
  run across everything I've touched.

  Ran 4 shell commands (ctrl+o to expand)
● Confirmed pre-existing and unrelated (no git changes in that directory; it's a
  whitespace mismatch in Azure DevOps comment parsing, nothing to do with my
  work). Let's run go vet as a final check across everything I touched.

  Ran 1 shell command (ctrl+o to expand)

● Only the same pre-existing, unrelated go vet finding (already confirmed
  earlier). Everything I've built is clean. Let's do a final review of all
  changes.

  Ran 1 shell command (ctrl+o to expand)

● There's a stray compiled binary at the repo root — let me check and clean it
  up.

  Listed 1 directory, ran 1 shell command (ctrl+o to expand)

● Clean now. Step 9 complete — all 9 build-order steps done and verified.

● All 9 build steps are complete and verified. Here's a summary of what was
  built:

  Database (db/migrations/2026072712000{0,1}_*.sql): new repositories and
  pull_requests tables (normalized across GitHub/GitLab, staleness-guarded
  upserts), plus a nullable reviews.pull_request_id FK.

  Provider layer (internal/providers/{github,gitlab}/{repo_list,pr_list}.go):
  new paginated ListRepositories/ListPullRequests functions with proper
  Link-header/X-Next-Page pagination and rate-limit detection
  (providers.RateLimitedError), normalized via a new internal/prsync package
  shared by both the poll and webhook paths.

  Sync engine (internal/jobqueue/repo_sync_worker.go): a River-based
  RepoPRSyncWorker (per-repo sync with incremental early-stop),
  PRStateSyncWorker (webhook-driven upserts), and ReconciliationSweepWorker —
  the first use of River's PeriodicJobs in this codebase, running every 15 min
  and only re-polling repos whose last_synced_at is stale (repos kept fresh by
  webhooks are skipped automatically).

  Webhook fix: discovered and fixed a real bug — GitHub's pull_request events
  were already being delivered but returned HTTP 400 (unhandled), and GitLab's
  merge_request events were silently swallowed as empty no-op comments. Both now
  update pull_requests in near-real-time, with a regression-tested guard so the
  existing reviewer-assignment flow is untouched.

  API (internal/api/repositories_pr_routes.go, pull_request_review_trigger.go):
  GET /repositories, GET/POST /repositories/:id[/pull-requests[/:id]][/sync],
  POST /connectors/:id/repositories/sync, and POST /reviews {pull_request_id}
  (replacing the old stub, reusing TriggerReviewV2's phases including LOC
  billing enforcement).

  Testing: unit tests for pagination/state-mapping, real-Postgres tests for
  upsert idempotency and the stale-write guard (including a concurrent-upsert
  race test), a periodic-job registration smoke test, webhook regression tests,
  and org-scoped API handler tests — all passing. Four pre-existing, unrelated
  test failures (billing proration, two Bitbucket capture fixtures, an Azure
  DevOps whitespace mismatch) were verified via git status/git log to predate
  this work.

  Bitbucket/Gitea/Azure DevOps and the UI remain out of scope per the plan, and
  the design doesn't preclude adding them later via the same pattern.

✻ Crunched for 53m 29s

✻ Claude resuming /loop wakeup (Jul 27 2:58pm)

● That work already finished — the Plan agent completed, the plan was written
  and approved, and I've since implemented and verified the entire backend (all
  9 build steps: migrations, provider list functions, sync workers, webhook
  fixes, API endpoints, and tests). Nothing is pending from that request.

  Is there something new you'd like me to look at — e.g., extending this to
  Bitbucket/Gitea, building the UI, or reviewing/adjusting something in what was
  built?

✻ Crunched for 6s