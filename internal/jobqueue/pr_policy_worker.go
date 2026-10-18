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

	"github.com/livereview/internal/policyengine"
	providerbitbucket "github.com/livereview/internal/providers/bitbucket"
	providergitea "github.com/livereview/internal/providers/gitea"
	providergithub "github.com/livereview/internal/providers/github"
	providergitlab "github.com/livereview/internal/providers/gitlab"
	storagejobqueue "github.com/livereview/storage/jobqueue"
	"github.com/riverqueue/river"
)

// PRPolicyCheckJobArgs runs the risk-based approval policy on a finished review
// that was started from the PR comment's "Start a review" link
// (metadata.pr_link_policy). See docs/risk-policy-engine.md.
type PRPolicyCheckJobArgs struct {
	OrgID    int64 `json:"org_id"`
	ReviewID int64 `json:"review_id"`
}

func (PRPolicyCheckJobArgs) Kind() string { return "pr_policy_check" }

type PRPolicyCheckWorker struct {
	river.WorkerDefaults[PRPolicyCheckJobArgs]
	db           *sql.DB
	webhookStore *storagejobqueue.WebhookStore
	engine       *policyengine.Engine
	jq           *JobQueue
}

func (w *PRPolicyCheckWorker) Work(ctx context.Context, job *river.Job[PRPolicyCheckJobArgs]) error {
	args := job.Args

	var status string
	var pullRequestID sql.NullInt64
	var flagged bool
	err := w.db.QueryRowContext(ctx, `
		SELECT status, pull_request_id,
		       COALESCE((metadata->>'pr_link_policy')::boolean, false)
		FROM reviews WHERE id = $1 AND org_id = $2`,
		args.ReviewID, args.OrgID,
	).Scan(&status, &pullRequestID, &flagged)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to load review %d: %w", args.ReviewID, err)
	}
	if status != "completed" || !pullRequestID.Valid || !flagged {
		return nil
	}

	// Claim: the policy_result key doubles as an idempotency guard so River
	// retries never evaluate (or approve) twice.
	claim := `{"status":"evaluating"}`
	res0, err := w.db.ExecContext(ctx,
		`UPDATE reviews SET metadata = metadata || jsonb_build_object('policy_result', $2::jsonb)
		 WHERE id = $1 AND NOT (metadata ? 'policy_result')`,
		args.ReviewID, claim)
	if err != nil {
		return fmt.Errorf("failed to claim policy check for review %d: %w", args.ReviewID, err)
	}
	if n, _ := res0.RowsAffected(); n == 0 {
		return nil // already evaluated
	}
	release := func() {
		if _, e := w.db.ExecContext(ctx, `UPDATE reviews SET metadata = metadata - 'policy_result' WHERE id = $1`, args.ReviewID); e != nil {
			log.Printf("[WARN] pr policy: failed to release claim for review %d: %v", args.ReviewID, e)
		}
	}

	res, err := w.engine.Run(ctx, args.OrgID, args.ReviewID, false)
	if errors.Is(err, policyengine.ErrPolicyDisabled) {
		release()
		return nil // feature off; nothing to post
	}
	if err != nil {
		release()
		return fmt.Errorf("policy evaluation failed for review %d: %w", args.ReviewID, err)
	}

	// Persist the funnel result (the policy-log table is the rendering source of
	// truth; this JSON is what the landing-delay publish job re-reads).
	resultJSON, _ := json.Marshal(res)
	if _, err := w.db.ExecContext(ctx,
		`UPDATE reviews SET metadata = metadata || jsonb_build_object('policy_result', $2::jsonb) WHERE id = $1`,
		args.ReviewID, string(resultJSON)); err != nil {
		release()
		return fmt.Errorf("failed to store policy result for review %d: %w", args.ReviewID, err)
	}

	// Landing delay: instead of publishing immediately, queue a follow-up publish
	// job scheduled after the human-rejection window.
	if res.Approved && res.LandingDelaySeconds > 0 {
		if err := w.queuePublish(ctx, args.OrgID, args.ReviewID, time.Duration(res.LandingDelaySeconds)*time.Second); err != nil {
			release()
			return err
		}
		return nil
	}

	if err := w.publish(ctx, args, pullRequestID.Int64, res); err != nil {
		release()
		return err
	}
	return nil
}

// queuePublish enqueues the landing-delay publish follow-up.
func (w *PRPolicyCheckWorker) queuePublish(ctx context.Context, orgID, reviewID int64, delay time.Duration) error {
	if w.jq == nil {
		return fmt.Errorf("job queue unavailable for landing-delay publish")
	}
	_, err := w.jq.client.Insert(ctx, PRPolicyPublishJobArgs{OrgID: orgID, ReviewID: reviewID},
		&river.InsertOpts{
			Queue:       "repo_sync",
			MaxAttempts: 5,
			ScheduledAt: time.Now().Add(delay),
		})
	if err != nil {
		return fmt.Errorf("failed to queue landing-delay publish: %w", err)
	}
	return nil
}

// publish posts the policy comment and approves (or requests changes on) the PR/MR.
func (w *PRPolicyCheckWorker) publish(ctx context.Context, args PRPolicyCheckJobArgs, pullRequestID int64, res *policyengine.FunnelResult) error {
	var repositoryID int64
	var number int
	if err := w.db.QueryRowContext(ctx,
		`SELECT repository_id, number FROM pull_requests WHERE id = $1 AND org_id = $2`,
		pullRequestID, args.OrgID,
	).Scan(&repositoryID, &number); err != nil {
		return fmt.Errorf("failed to load pull request %d: %w", pullRequestID, err)
	}
	info, err := loadRepositoryConnectorInfo(w.db, repositoryID)
	if err != nil {
		return fmt.Errorf("failed to load connector for repository %d: %w", repositoryID, err)
	}
	host, err := w.webhookStore.GetWebhookPublicEndpoint(ctx)
	if err != nil {
		log.Printf("[WARN] pr policy: public URL not configured, posting without review link: %v", err)
	}
	body := buildPolicySummary(res) + " · " + policyReviewLink(host, args.ReviewID) + "\n\n" + buildPolicyDetails(res)
	if host != "" {
		// The description's review history table is a convenience; don't fail the publish over it.
		if err := refreshPRSection(ctx, w.db, info, host, pullRequestID, number); err != nil {
			log.Printf("[WARN] pr policy: failed to update PR description for review %d: %v", args.ReviewID, err)
		}
	}

	switch info.Provider {
	case "github", "github-com", "github-enterprise":
		event := "REQUEST_CHANGES"
		if res.Approved {
			event = "APPROVE"
		}
		if err := providergithub.SubmitPullRequestReview(ctx, info.ProviderURL, info.token(), info.FullName, number, event, body); err != nil {
			// GitHub refuses approve/request-changes when the bot authored the PR; still show the result.
			log.Printf("[WARN] pr policy: %s failed for review %d, posting as comment: %v", event, args.ReviewID, err)
			return providergithub.PostIssueComment(ctx, info.ProviderURL, info.token(), info.FullName, number, body)
		}
		return nil
	case "gitlab", "gitlab-com", "gitlab-self-hosted":
		client := providergitlab.NewHTTPClient(info.ProviderURL, info.token())
		if err := client.CreateMRGeneralComment(info.ProviderRepoID, number, body); err != nil {
			return err
		}
		// GitLab Free has no "request changes", so a failed policy is comment-only.
		if res.Approved {
			if err := client.ApproveMR(info.ProviderRepoID, number); err != nil {
				log.Printf("[WARN] pr policy: approve failed for review %d: %v", args.ReviewID, err)
			}
		}
		return nil
	case "gitea":
		event := "REQUEST_CHANGES"
		if res.Approved {
			event = "APPROVED"
		}
		if err := providergitea.SubmitPullRequestReview(ctx, info.ProviderURL, info.token(), info.FullName, number, event, body); err != nil {
			// Gitea refuses reviews on the bot's own PRs; still show the result.
			log.Printf("[WARN] pr policy: %s failed for review %d, posting as comment: %v", event, args.ReviewID, err)
			return providergitea.PostIssueComment(ctx, info.ProviderURL, info.token(), info.FullName, number, body)
		}
		return nil
	case "bitbucket":
		if err := providerbitbucket.PostPRComment(ctx, info.Email, info.token(), info.FullName, number, body); err != nil {
			return err
		}
		if err := providerbitbucket.SetPRVerdict(ctx, info.Email, info.token(), info.FullName, number, res.Approved); err != nil {
			log.Printf("[WARN] pr policy: bitbucket verdict failed for review %d: %v", args.ReviewID, err)
		}
		return nil
	case "azuredevops":
		client, err := azureClient(info)
		if err != nil {
			return err
		}
		if err := client.PostPRThread(ctx, info.FullName, number, body); err != nil {
			return err
		}
		if err := client.SetPRVote(ctx, info.FullName, number, res.Approved); err != nil {
			log.Printf("[WARN] pr policy: azure devops vote failed for review %d: %v", args.ReviewID, err)
		}
		return nil
	default:
		return nil
	}
}

// PRPolicyPublishJobArgs re-publishes the policy comment after the landing delay.
type PRPolicyPublishJobArgs struct {
	OrgID    int64 `json:"org_id"`
	ReviewID int64 `json:"review_id"`
}

func (PRPolicyPublishJobArgs) Kind() string { return "pr_policy_publish" }

type PRPolicyPublishWorker struct {
	river.WorkerDefaults[PRPolicyPublishJobArgs]
	db           *sql.DB
	webhookStore *storagejobqueue.WebhookStore
}

func (w *PRPolicyPublishWorker) Work(ctx context.Context, job *river.Job[PRPolicyPublishJobArgs]) error {
	args := job.Args

	// Guard against double-posting.
	res0, err := w.db.ExecContext(ctx,
		`UPDATE reviews SET metadata = metadata || jsonb_build_object('policy_published_at', now())
		 WHERE id = $1 AND org_id = $2 AND NOT (metadata ? 'policy_published_at')`,
		args.ReviewID, args.OrgID)
	if err != nil {
		return err
	}
	if n, _ := res0.RowsAffected(); n == 0 {
		return nil // already published
	}
	release := func() {
		_, _ = w.db.ExecContext(ctx, `UPDATE reviews SET metadata = metadata - 'policy_published_at' WHERE id = $1`, args.ReviewID)
	}

	var pullRequestID sql.NullInt64
	var resultJSON []byte
	if err := w.db.QueryRowContext(ctx,
		`SELECT pull_request_id, COALESCE(metadata->'policy_result', '{}'::jsonb) FROM reviews WHERE id = $1 AND org_id = $2`,
		args.ReviewID, args.OrgID).Scan(&pullRequestID, &resultJSON); err != nil {
		release()
		return err
	}
	if !pullRequestID.Valid {
		release()
		return nil
	}
	var res policyengine.FunnelResult
	if err := json.Unmarshal(resultJSON, &res); err != nil {
		release()
		return fmt.Errorf("failed to decode policy result for review %d: %w", args.ReviewID, err)
	}

	checker := &PRPolicyCheckWorker{db: w.db, webhookStore: w.webhookStore}
	if err := checker.publish(ctx, PRPolicyCheckJobArgs{OrgID: args.OrgID, ReviewID: args.ReviewID}, pullRequestID.Int64, &res); err != nil {
		release()
		return err
	}
	return nil
}

// buildPolicySummary is the one-line verdict used as the approve / request-changes review text.
func buildPolicySummary(res *policyengine.FunnelResult) string {
	if res.Approved {
		return "✅ **LiveReview policy: Approved**"
	}
	for _, st := range res.Stages {
		if st.Verdict == policyengine.VerdictFail {
			return fmt.Sprintf("❌ **LiveReview policy: Not approved** (blocked at %s)", titleCaseStage(st.Stage))
		}
	}
	return "❌ **LiveReview policy: Not approved**"
}

// policyReviewLink links the review on LiveReview, or names it when the host is unknown.
func policyReviewLink(host string, reviewID int64) string {
	if host == "" {
		return fmt.Sprintf("Review #%d", reviewID)
	}
	return fmt.Sprintf("[Review #%d](%s/#/reviews/%d)", reviewID, strings.TrimSuffix(host, "/"), reviewID)
}

// buildPolicyDetails renders the funnel stage by stage for the policy comment, so it shows *why* the policy decided.
func buildPolicyDetails(res *policyengine.FunnelResult) string {
	var b strings.Builder
	b.WriteString("| Stage | Result | Detail |\n|---|---|---|\n")
	for _, st := range res.Stages {
		reason := ""
		if r, ok := st.Detail["reason"].(string); ok {
			reason = strings.ReplaceAll(r, "|", "\\|")
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", titleCaseStage(st.Stage), stageIcon(st.Verdict), reason)
	}
	b.WriteString("\n")
	return b.String()
}

func stageIcon(verdict string) string {
	switch verdict {
	case policyengine.VerdictFail, policyengine.FinalBlocked:
		return "❌"
	case policyengine.VerdictSkip:
		return "⏭️"
	default:
		return "✅"
	}
}

func titleCaseStage(s string) string {
	switch s {
	case "eligibility":
		return "Eligibility"
	case "static":
		return "Static rules"
	case "risk":
		return "Risk score"
	case "llm":
		return "LLM classification"
	case "final":
		return "Final decision"
	default:
		return s
	}
}

// QueuePRPolicyCheckIfFlagged enqueues the policy check only for reviews started from the PR link.
func (jq *JobQueue) QueuePRPolicyCheckIfFlagged(ctx context.Context, orgID, reviewID int64) error {
	var flagged bool
	if err := jq.db.QueryRowContext(ctx,
		`SELECT COALESCE((metadata->>'pr_link_policy')::boolean, false) FROM reviews WHERE id = $1 AND org_id = $2`,
		reviewID, orgID,
	).Scan(&flagged); err != nil || !flagged {
		return err
	}
	_, err := jq.client.Insert(ctx, PRPolicyCheckJobArgs{OrgID: orgID, ReviewID: reviewID},
		&river.InsertOpts{Queue: "repo_sync", MaxAttempts: 5})
	if err != nil {
		return fmt.Errorf("failed to queue PR policy check job: %w", err)
	}
	return nil
}
