package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livereview/internal/githubapp"
	"github.com/livereview/internal/policyengine"
	"github.com/livereview/internal/providers"
	providerazure "github.com/livereview/internal/providers/azuredevops"
	providerbitbucket "github.com/livereview/internal/providers/bitbucket"
	providergitea "github.com/livereview/internal/providers/gitea"
	providergithub "github.com/livereview/internal/providers/github"
	providergitlab "github.com/livereview/internal/providers/gitlab"
	"github.com/livereview/internal/prsync"
	storagejobqueue "github.com/livereview/storage/jobqueue"
	storagegithub "github.com/livereview/storage/providers/github"
	"github.com/livereview/storage/providers/pullrequests"
	"github.com/riverqueue/river"
)

// repositoryConnectorInfo is what RepoPRSyncWorker and PRStateSyncWorker need
// to know about a repository's owning connector to call the right provider API.
type repositoryConnectorInfo struct {
	RepositoryID   int64
	OrgID          int64
	ConnectorID    int64
	Provider       string
	ProviderRepoID string
	FullName       string
	LastSyncedAt   sql.NullTime
	ProviderURL    string
	PATToken       string
	AccessToken    string
	Email          string // Bitbucket basic-auth user, from connector metadata
	TokenType      string // 'GITHUB_APP' rows hold an hourly installation token in pat_token
}

func (info repositoryConnectorInfo) token() string {
	if info.PATToken != "" && info.PATToken != "NA" {
		return info.PATToken
	}
	return info.AccessToken
}

func loadRepositoryConnectorInfo(db *sql.DB, repositoryID int64) (*repositoryConnectorInfo, error) {
	var info repositoryConnectorInfo
	err := db.QueryRow(`
		SELECT r.id, r.org_id, r.connector_id, r.provider, r.provider_repo_id, r.full_name, r.last_synced_at,
		       i.provider_url, COALESCE(i.pat_token, ''), COALESCE(i.access_token, ''), COALESCE(i.metadata->>'email', ''), COALESCE(i.token_type, '')
		FROM repositories r
		JOIN integration_tokens i ON i.id = r.connector_id
		WHERE r.id = $1
	`, repositoryID).Scan(&info.RepositoryID, &info.OrgID, &info.ConnectorID, &info.Provider, &info.ProviderRepoID,
		&info.FullName, &info.LastSyncedAt, &info.ProviderURL, &info.PATToken, &info.AccessToken, &info.Email, &info.TokenType)
	if err != nil {
		return nil, err
	}
	// GitHub App rows cache an hourly installation token in pat_token; resolve a valid one so every caller of
	// token() (PR sync, policy publishing, PR description updates) gets a working token. Other rows are unchanged.
	resolveCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if info.PATToken, err = githubapp.ResolveIfApp(resolveCtx, db, info.ConnectorID, info.TokenType, info.PATToken); err != nil {
		return nil, fmt.Errorf("failed to resolve GitHub App token for connector %d: %w", info.ConnectorID, err)
	}
	return &info, nil
}

func isGitHubProvider(provider string) bool {
	return provider == "github" || provider == "github-com" || provider == "github-enterprise"
}

// appRepositoryID finds the repositories row of the same org's live GitHub App connector for the same repo on the
// same host, or sql.ErrNoRows. Only token connectors can be covered by an App.
func appRepositoryID(db *sql.DB, info *repositoryConnectorInfo) (int64, error) {
	if info.TokenType == storagegithub.TokenTypeGitHubApp || !isGitHubProvider(info.Provider) {
		return 0, sql.ErrNoRows
	}
	var id int64
	err := db.QueryRow(`
		SELECT r.id FROM repositories r JOIN integration_tokens it ON it.id = r.connector_id
		WHERE it.org_id = $1 AND it.token_type = $2 AND LOWER(r.full_name) = LOWER($3)
		  AND LOWER(TRIM(TRAILING '/' FROM it.provider_url)) = LOWER(TRIM(TRAILING '/' FROM $4::text))
		  AND COALESCE(it.metadata->>'removed', 'false') <> 'true'
		  AND COALESCE(it.metadata->>'suspended', 'false') <> 'true'
		ORDER BY it.created_at DESC LIMIT 1
	`, info.OrgID, storagegithub.TokenTypeGitHubApp, info.FullName, info.ProviderURL).Scan(&id)
	return id, err
}

// loadActingConnectorInfo is loadRepositoryConnectorInfo, except that when the org's GitHub App also lists the
// repository, the App's credentials are used, so posts come from the App and not a token owner's account.
func loadActingConnectorInfo(db *sql.DB, repositoryID int64) (*repositoryConnectorInfo, error) {
	info, err := loadRepositoryConnectorInfo(db, repositoryID)
	if err != nil {
		return nil, err
	}
	appRepoID, err := appRepositoryID(db, info)
	if errors.Is(err, sql.ErrNoRows) {
		return info, nil
	}
	var app *repositoryConnectorInfo
	if err == nil {
		app, err = loadRepositoryConnectorInfo(db, appRepoID)
	}
	if err != nil {
		// Better to post with the token than to drop the result.
		log.Printf("[WARN] acting connector: could not use the GitHub App for repository %d: %v", repositoryID, err)
		return info, nil
	}
	// Keep this PR copy's repository identity; only who we act as changes.
	info.ConnectorID, info.ProviderURL, info.PATToken, info.AccessToken, info.Email, info.TokenType =
		app.ConnectorID, app.ProviderURL, app.PATToken, app.AccessToken, app.Email, app.TokenType
	return info, nil
}

// RepoPRSyncJobArgs syncs all pull/merge requests for one repository from its
// provider into the pull_requests table. Enqueued both by the periodic
// reconciliation coordinator (see ReconciliationSweepWorker) and by the
// auto-installer on initial connector/repository discovery (InitialBackfill).
type RepoPRSyncJobArgs struct {
	RepositoryID    int64 `json:"repository_id"`
	InitialBackfill bool  `json:"initial_backfill"`
}

func (RepoPRSyncJobArgs) Kind() string { return "repo_pr_sync" }

type RepoPRSyncWorker struct {
	river.WorkerDefaults[RepoPRSyncJobArgs]
	db    *sql.DB
	store *pullrequests.Store
}

func (w *RepoPRSyncWorker) Work(ctx context.Context, job *river.Job[RepoPRSyncJobArgs]) error {
	args := job.Args
	info, err := loadRepositoryConnectorInfo(w.db, args.RepositoryID)
	if err != nil {
		if err == sql.ErrNoRows {
			// Repository row was deleted (e.g. connector removed) since this job
			// was enqueued - nothing to sync, not a failure.
			return nil
		}
		return fmt.Errorf("failed to load repository %d: %w", args.RepositoryID, err)
	}

	// The org's GitHub App syncs this repo already; a token copy would double every PR.
	if _, err := appRepositoryID(w.db, info); err == nil {
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to check GitHub App coverage for repository %d: %w", args.RepositoryID, err)
	}

	state := providers.PRListStateAll
	stopAt := time.Time{}
	if !args.InitialBackfill && info.LastSyncedAt.Valid {
		// Incremental sync: pages are sorted by updated-desc, so once we see a
		// PR whose provider_updated_at is no newer than our last sync, every
		// subsequent PR in this and further pages is already up to date -
		// avoids re-walking a repo's entire PR history every cycle.
		stopAt = info.LastSyncedAt.Time
	}

	syncSource := "poll"
	if args.InitialBackfill {
		syncSource = "backfill"
	}

	cursor := ""
	synced := 0
	for {
		var page *providers.PullRequestPage
		var err error
		switch info.Provider {
		case "github", "github-com", "github-enterprise":
			page, err = providergithub.ListPullRequests(ctx, info.ProviderURL, info.token(), info.FullName, state, cursor)
		case "gitlab", "gitlab-com", "gitlab-self-hosted":
			page, err = providergitlab.ListPullRequests(ctx, info.ProviderURL, info.token(), info.ProviderRepoID, state, cursor)
		default:
			return fmt.Errorf("repo sync not supported for provider %q", info.Provider)
		}
		if err != nil {
			var rlErr *providers.RateLimitedError
			if errors.As(err, &rlErr) {
				return river.JobSnooze(rlErr.RetryAfter)
			}
			w.markSyncError(args.RepositoryID, err)
			return fmt.Errorf("failed to list pull requests for repository %d: %w", args.RepositoryID, err)
		}

		reachedStopPoint := false
		for _, pr := range page.PullRequests {
			if !stopAt.IsZero() && !pr.ProviderUpdatedAt.IsZero() && !pr.ProviderUpdatedAt.After(stopAt) {
				reachedStopPoint = true
				break
			}
			if _, err := w.store.UpsertPullRequest(pullrequests.PullRequestUpsert{
				RepositoryID:      info.RepositoryID,
				OrgID:             info.OrgID,
				Provider:          info.Provider,
				ProviderPRID:      pr.ProviderPRID,
				Number:            pr.Number,
				Title:             pr.Title,
				Description:       pr.Description,
				State:             pr.State,
				AuthorID:          pr.AuthorID,
				AuthorUsername:    pr.AuthorUsername,
				AuthorName:        pr.AuthorName,
				AuthorAvatarURL:   pr.AuthorAvatarURL,
				SourceBranch:      pr.SourceBranch,
				TargetBranch:      pr.TargetBranch,
				WebURL:            pr.WebURL,
				ProviderCreatedAt: pr.ProviderCreatedAt,
				ProviderUpdatedAt: pr.ProviderUpdatedAt,
				LastSyncedSource:  syncSource,
				Metadata:          pr.Metadata,
			}); err != nil {
				w.markSyncError(args.RepositoryID, err)
				return fmt.Errorf("failed to upsert pull request %s for repository %d: %w", pr.ProviderPRID, args.RepositoryID, err)
			}
			synced++
		}

		if reachedStopPoint || page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}

	if _, err := w.db.ExecContext(ctx,
		`UPDATE repositories SET last_synced_at = now(), last_sync_status = 'ok', last_sync_error = NULL, updated_at = now() WHERE id = $1`,
		args.RepositoryID,
	); err != nil {
		return fmt.Errorf("failed to update repository sync status: %w", err)
	}

	log.Printf("[INFO] repo_pr_sync: repository_id=%d provider=%s synced=%d initial_backfill=%v",
		args.RepositoryID, info.Provider, synced, args.InitialBackfill)
	return nil
}

func (w *RepoPRSyncWorker) markSyncError(repositoryID int64, syncErr error) {
	_, err := w.db.Exec(
		`UPDATE repositories SET last_sync_status = 'error', last_sync_error = $2, updated_at = now() WHERE id = $1`,
		repositoryID, syncErr.Error(),
	)
	if err != nil {
		log.Printf("[WARN] failed to record repository sync error for repository %d: %v", repositoryID, err)
	}
}

// PRStateSyncJobArgs upserts a single PR/MR whose state was reported by an
// already-arriving-but-previously-unhandled webhook delivery (GitHub
// "pull_request", GitLab "merge_request" - see internal/api/webhook_orchestrator_v2.go).
type PRStateSyncJobArgs struct {
	OrgID       int64                        `json:"org_id"`
	ConnectorID int64                        `json:"connector_id"`
	Provider    string                       `json:"provider"`
	Event       prsync.PullRequestStateEvent `json:"event"`
	// SkipLinks: another org's connector for the same GitHub App installation writes the PR description links.
	SkipLinks bool `json:"skip_links,omitempty"`
}

func (PRStateSyncJobArgs) Kind() string { return "pr_state_sync" }

type PRStateSyncWorker struct {
	river.WorkerDefaults[PRStateSyncJobArgs]
	db           *sql.DB
	store        *pullrequests.Store
	webhookStore *storagejobqueue.WebhookStore
}

func (w *PRStateSyncWorker) Work(ctx context.Context, job *river.Job[PRStateSyncJobArgs]) error {
	args := job.Args
	event := args.Event

	var repositoryID int64
	err := w.db.QueryRowContext(ctx,
		`SELECT id FROM repositories WHERE connector_id = $1 AND provider_repo_id = $2`,
		args.ConnectorID, event.RepositoryProviderID,
	).Scan(&repositoryID)
	if err == sql.ErrNoRows {
		// This repo's discovery/backfill sync hasn't created a repositories row
		// yet - insert a minimal stub from the webhook payload itself so the PR
		// isn't dropped; the later backfill sync fills in the rest.
		repositoryID, err = w.store.EnsureRepositoryStub(pullrequests.RepositoryUpsert{
			OrgID:          args.OrgID,
			ConnectorID:    args.ConnectorID,
			Provider:       args.Provider,
			ProviderRepoID: event.RepositoryProviderID,
			FullName:       event.RepositoryFullName,
			Name:           event.RepositoryFullName,
			WebURL:         event.RepositoryWebURL,
		})
	}
	if err != nil {
		return fmt.Errorf("failed to resolve repository for connector=%d provider_repo_id=%s: %w",
			args.ConnectorID, event.RepositoryProviderID, err)
	}

	pullRequestID, err := w.store.UpsertPullRequest(pullrequests.PullRequestUpsert{
		RepositoryID:      repositoryID,
		OrgID:             args.OrgID,
		Provider:          args.Provider,
		ProviderPRID:      event.ProviderPRID,
		Number:            event.Number,
		Title:             event.Title,
		Description:       event.Description,
		State:             event.State,
		AuthorID:          event.AuthorID,
		AuthorUsername:    event.AuthorUsername,
		AuthorName:        event.AuthorName,
		AuthorAvatarURL:   event.AuthorAvatarURL,
		SourceBranch:      event.SourceBranch,
		TargetBranch:      event.TargetBranch,
		WebURL:            event.WebURL,
		ProviderCreatedAt: event.ProviderCreatedAt,
		ProviderUpdatedAt: event.ProviderUpdatedAt,
		LastSyncedSource:  "webhook",
		Metadata:          event.Metadata,
	})
	if err != nil {
		return fmt.Errorf("failed to upsert pull request from webhook event: %w", err)
	}

	if action, _ := event.Metadata["action"].(string); !args.SkipLinks && (action == "opened" || action == "open") {
		return w.postLinksComment(ctx, repositoryID, pullRequestID, event.Number)
	}
	return nil
}

// The LiveReview section in a PR description sits between these markers so it can be replaced in place.
// They are markdown comments, which every provider hides (Bitbucket shows HTML comments as text).
const (
	controlsStart = "[//]: # (livereview:controls)"
	controlsEnd   = "[//]: # (livereview:controls-end)"
	resultsStart  = "[//]: # (livereview:results)"
	resultsEnd    = "[//]: # (livereview:results-end)"
	// Old single section (controls and table together, at the bottom), removed when found.
	prSectionStart     = "[//]: # (livereview:links)"
	prSectionEnd       = "[//]: # (livereview:end)"
	legacySectionStart = "<!-- livereview:links -->"
)

// postLinksComment adds the LiveReview links (review, history, repo/org policy) to the end of a new PR/MR's description.
// The links_comment_posted_at claim stops duplicate webhook deliveries from adding it twice.
func (w *PRStateSyncWorker) postLinksComment(ctx context.Context, repositoryID, pullRequestID int64, number int) error {
	host, err := w.webhookStore.GetWebhookPublicEndpoint(ctx)
	if err != nil || host == "" {
		log.Printf("[WARN] pr links: skipping pull_request_id=%d, public URL not configured: %v", pullRequestID, err)
		return nil
	}

	res, err := w.db.ExecContext(ctx,
		`UPDATE pull_requests SET links_comment_posted_at = now() WHERE id = $1 AND links_comment_posted_at IS NULL`,
		pullRequestID)
	if err != nil {
		return fmt.Errorf("failed to claim pr links for pull_request_id=%d: %w", pullRequestID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}

	info, err := loadActingConnectorInfo(w.db, repositoryID)
	if err == nil {
		err = writePRSection(ctx, info, number, func(body string) string {
			return upsertControlsSection(body, buildControlsSection(host, pullRequestID, repositoryID))
		})
	}
	if err != nil {
		// Release the claim so River's retry can add it.
		if _, resetErr := w.db.ExecContext(ctx, `UPDATE pull_requests SET links_comment_posted_at = NULL WHERE id = $1`, pullRequestID); resetErr != nil {
			log.Printf("[WARN] pr links: failed to release claim for pull_request_id=%d: %v", pullRequestID, resetErr)
		}
		return fmt.Errorf("failed to add pr links for pull_request_id=%d: %w", pullRequestID, err)
	}
	return nil
}

// buildControlsSection renders the control panel (start review, history, policy links) shown at the top of the PR.
func buildControlsSection(host string, pullRequestID, repositoryID int64) string {
	host = strings.TrimSuffix(host, "/")
	return fmt.Sprintf(`%s
### 🔍 LiveReview

- ▶️ **[Start a review](%s/#/pr/%d/review)**: reviews all changes in this PR against its target branch
- 📜 **[Review history](%s/#/reviews?pull_request_id=%d)**: all past reviews of this PR

⚙️ Customize risk-based approval policy for [this repository](%s/#/explore/repositories/%d/settings#auto-approve) or [your organization](%s/#/settings#auto-approve).

---
%s
`, controlsStart, host, pullRequestID, host, pullRequestID, host, repositoryID, host, controlsEnd)
}

// buildResultsSection wraps the decisions table, which stays at the bottom of the PR.
func buildResultsSection(history string) string {
	return fmt.Sprintf("%s\n\n---\n%s%s\n", resultsStart, history, resultsEnd)
}

// refreshPRSection rewrites the LiveReview section with one history row per policy-checked review of this PR.
// Rows are rebuilt from the DB each time, so retries never duplicate them.
func refreshPRSection(ctx context.Context, db *sql.DB, info *repositoryConnectorInfo, host string, pullRequestID int64, number int) error {
	rows, err := db.QueryContext(ctx,
		// ponytail: only the last 10 rows, so the table stays under description size limits (Azure DevOps: 4000 chars).
		`SELECT id, created_at, policy, forced_reason, time_zone FROM (
		   SELECT r.id, r.created_at, r.metadata->'policy_result' AS policy,
		     (SELECT u.time_zone FROM users u WHERE lower(u.email) = lower(r.user_email) LIMIT 1) AS time_zone,
		     (SELECT o.reason FROM review_policy_overrides o
		      WHERE o.review_id = r.id AND o.org_id = r.org_id ORDER BY o.created_at DESC LIMIT 1) AS forced_reason
		   FROM reviews r
		   WHERE r.pull_request_id = $1 AND r.metadata ? 'policy_result' ORDER BY r.id DESC LIMIT 10
		 ) recent ORDER BY id DESC`, pullRequestID)
	if err != nil {
		return err
	}
	defer rows.Close()

	host = strings.TrimSuffix(host, "/")
	var b strings.Builder
	b.WriteString("#### LiveReview auto-approval decisions\n\n| Review | Time | Decision | Reason |\n|---|---|---|---|\n")
	for rows.Next() {
		var reviewID int64
		var createdAt time.Time
		var raw []byte
		var forcedReason, timeZone sql.NullString
		if err := rows.Scan(&reviewID, &createdAt, &raw, &forcedReason, &timeZone); err != nil {
			return err
		}
		var res policyengine.FunnelResult
		if err := json.Unmarshal(raw, &res); err != nil {
			continue
		}
		decision, reason := "✅ Approved", decisionReason(&res)
		if forcedReason.Valid {
			decision, reason = "⚠️ Force-approved", forcedReason.String
		} else if !res.Approved {
			decision = "❌ Not approved"
		}
		fmt.Fprintf(&b, "| [#%d](%s/#/reviews/%d) | %s | %s | %s |\n", reviewID, host, reviewID, formatReviewTime(createdAt, timeZone), decision, strings.ReplaceAll(reason, "|", "\\|"))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	b.WriteString("\n")
	controls := buildControlsSection(host, pullRequestID, info.RepositoryID)
	results := buildResultsSection(b.String())
	return writePRSection(ctx, info, number, func(body string) string {
		return upsertResultsSection(upsertControlsSection(body, controls), results)
	})
}

// formatReviewTime shows the time in the review starter's saved zone, or UTC if none is saved.
func formatReviewTime(t time.Time, timeZone sql.NullString) string {
	if timeZone.Valid {
		if loc, err := time.LoadLocation(timeZone.String); err == nil {
			return t.In(loc).Format("2 Jan 2006, 15:04 MST")
		}
	}
	return t.UTC().Format("2 Jan 2006, 15:04 UTC")
}

// decisionReason is the readable "why" for one decision row: each failing stage's reason, or that all passed.
func decisionReason(res *policyengine.FunnelResult) string {
	// Mode overrides skip the checks, so they fall through to the final stage's reason.
	if res.Approved && !res.ModeOverride {
		return "All policy checks passed"
	}
	var parts []string
	for _, st := range res.Stages {
		if st.Verdict != policyengine.VerdictFail {
			continue
		}
		reason, _ := st.Detail["reason"].(string)
		if reason != "" {
			reason = strings.ToUpper(reason[:1]) + reason[1:]
		}
		parts = append(parts, reason)
	}
	if len(parts) == 0 {
		// Mode overrides (e.g. never_approve) block in the final stage only.
		for _, st := range res.Stages {
			if st.Stage == policyengine.StageFinal {
				reason, _ := st.Detail["reason"].(string)
				return reason
			}
		}
	}
	return strings.Join(parts, "; ")
}

// replaceBetween swaps the text from start through end markers for section; ok is false if start is absent.
func replaceBetween(body, start, end, section string) (string, bool) {
	i := strings.Index(body, start)
	if i < 0 {
		return body, false
	}
	j := strings.Index(body[i:], end)
	if j < 0 {
		return body[:i] + strings.TrimRight(section, "\n"), true
	}
	return body[:i] + strings.TrimRight(section, "\n") + body[i+j+len(end):], true
}

// removeOldSection drops the old single section; the legacy one was always last, so it goes to the end of body.
func removeOldSection(body string) string {
	if next, ok := replaceBetween(body, prSectionStart, prSectionEnd, ""); ok {
		return strings.TrimRight(next, "\n") + "\n"
	}
	if i := strings.Index(body, legacySectionStart); i >= 0 {
		return strings.TrimRight(body[:i], "\n") + "\n"
	}
	return body
}

// upsertControlsSection puts the control panel at the top of body, replacing it in place if already there.
func upsertControlsSection(body, section string) string {
	body = removeOldSection(body)
	if next, ok := replaceBetween(body, controlsStart, controlsEnd, section); ok {
		return next
	}
	if strings.TrimSpace(body) == "" {
		return section
	}
	return section + "\n" + strings.TrimLeft(body, "\n")
}

// upsertResultsSection keeps the decisions table at the bottom of body, replacing it in place if already there.
func upsertResultsSection(body, section string) string {
	if next, ok := replaceBetween(body, resultsStart, resultsEnd, section); ok {
		return next
	}
	return strings.TrimRight(body, "\n") + "\n\n" + section
}

// writePRSection reads the PR/MR description and saves it after update changes the LiveReview sections.
func writePRSection(ctx context.Context, info *repositoryConnectorInfo, number int, update func(body string) string) error {
	var get func() (string, error)
	var set func(string) error
	switch info.Provider {
	case "github", "github-com", "github-enterprise":
		get = func() (string, error) {
			return providergithub.GetPRBody(ctx, info.ProviderURL, info.token(), info.FullName, number)
		}
		set = func(b string) error {
			return providergithub.SetPRBody(ctx, info.ProviderURL, info.token(), info.FullName, number, b)
		}
	case "gitlab", "gitlab-com", "gitlab-self-hosted":
		client := providergitlab.NewHTTPClient(info.ProviderURL, info.token())
		get = func() (string, error) {
			mr, err := client.GetMergeRequest(info.ProviderRepoID, number)
			if err != nil {
				return "", err
			}
			return mr.Description, nil
		}
		set = func(b string) error { return client.UpdateMRDescription(info.ProviderRepoID, number, b) }
	case "gitea":
		get = func() (string, error) {
			return providergitea.GetPRBody(ctx, info.ProviderURL, info.token(), info.FullName, number)
		}
		set = func(b string) error {
			return providergitea.SetPRBody(ctx, info.ProviderURL, info.token(), info.FullName, number, b)
		}
	case "bitbucket":
		get = func() (string, error) {
			return providerbitbucket.GetPRBody(ctx, info.Email, info.token(), info.FullName, number)
		}
		set = func(b string) error {
			return providerbitbucket.SetPRBody(ctx, info.Email, info.token(), info.FullName, number, b)
		}
	case "azuredevops":
		client, err := azureClient(info)
		if err != nil {
			return err
		}
		get = func() (string, error) { return client.GetPRDescription(ctx, info.FullName, number) }
		set = func(b string) error { return client.SetPRDescription(ctx, info.FullName, number, b) }
	default:
		return nil
	}

	body, err := get()
	if err != nil {
		return err
	}
	if next := update(body); next != body {
		return set(next)
	}
	return nil
}

func azureClient(info *repositoryConnectorInfo) (*providerazure.Provider, error) {
	return providerazure.NewProvider(providerazure.Config{BaseURL: info.ProviderURL, Token: info.token()})
}

// ReconciliationSweepJobArgs is the periodic coordinator job: it finds
// repositories whose PR/MR data hasn't been synced recently (either never, or
// staler than the configured threshold - which naturally excludes repos whose
// webhooks are keeping them fresh) and enqueues one RepoPRSyncJobArgs per repo.
type ReconciliationSweepJobArgs struct{}

func (ReconciliationSweepJobArgs) Kind() string { return "repo_sync_reconciliation_sweep" }

type ReconciliationSweepWorker struct {
	river.WorkerDefaults[ReconciliationSweepJobArgs]
	db                 *sql.DB
	pool               *pgxpool.Pool
	stalenessThreshold time.Duration
	jq                 *JobQueue
}

func (w *ReconciliationSweepWorker) Work(ctx context.Context, job *river.Job[ReconciliationSweepJobArgs]) error {
	threshold := w.stalenessThreshold
	if threshold <= 0 {
		threshold = 20 * time.Minute
	}

	rows, err := w.db.QueryContext(ctx,
		`SELECT id FROM repositories WHERE last_synced_at IS NULL OR last_synced_at < now() - make_interval(secs => $1)`,
		threshold.Seconds(),
	)
	if err != nil {
		return fmt.Errorf("failed to query stale repositories: %w", err)
	}
	defer rows.Close()

	var repositoryIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("failed to scan repository id: %w", err)
		}
		repositoryIDs = append(repositoryIDs, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range repositoryIDs {
		if err := w.jq.QueueRepoPRSyncJob(ctx, id, false); err != nil {
			log.Printf("[WARN] reconciliation sweep: failed to queue sync for repository %d: %v", id, err)
		}
	}

	log.Printf("[INFO] reconciliation sweep: queued %d stale repositories for sync", len(repositoryIDs))
	return nil
}

// QueueRepoPRSyncJob enqueues a PR/MR sync for one repository on the
// "repo_sync" queue.
func (jq *JobQueue) QueueRepoPRSyncJob(ctx context.Context, repositoryID int64, initialBackfill bool) error {
	_, err := jq.client.Insert(ctx, RepoPRSyncJobArgs{
		RepositoryID:    repositoryID,
		InitialBackfill: initialBackfill,
	}, &river.InsertOpts{Queue: "repo_sync", MaxAttempts: 5})
	if err != nil {
		return fmt.Errorf("failed to queue repo PR sync job: %w", err)
	}
	return nil
}

// QueuePRStateSyncJob enqueues an upsert for a single PR/MR whose state a
// webhook delivery just reported, on the "repo_sync" queue.
func (jq *JobQueue) QueuePRStateSyncJob(ctx context.Context, orgID, connectorID int64, provider string, event prsync.PullRequestStateEvent) error {
	return jq.queuePRStateSync(ctx, PRStateSyncJobArgs{OrgID: orgID, ConnectorID: connectorID, Provider: provider, Event: event})
}

// QueueSharedPRStateSyncJob updates one more org's copy of a PR whose GitHub App installation several orgs share;
// it never writes the PR description links (the installation's primary connector does that once).
func (jq *JobQueue) QueueSharedPRStateSyncJob(ctx context.Context, orgID, connectorID int64, provider string, event prsync.PullRequestStateEvent) error {
	return jq.queuePRStateSync(ctx, PRStateSyncJobArgs{OrgID: orgID, ConnectorID: connectorID, Provider: provider, Event: event, SkipLinks: true})
}

func (jq *JobQueue) queuePRStateSync(ctx context.Context, args PRStateSyncJobArgs) error {
	_, err := jq.client.Insert(ctx, args, &river.InsertOpts{Queue: "repo_sync", MaxAttempts: 5})
	if err != nil {
		return fmt.Errorf("failed to queue PR state sync job: %w", err)
	}
	return nil
}
