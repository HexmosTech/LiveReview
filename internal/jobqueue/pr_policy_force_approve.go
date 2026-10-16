package jobqueue

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	providergithub "github.com/livereview/internal/providers/github"
	providergitlab "github.com/livereview/internal/providers/gitlab"
	storagejobqueue "github.com/livereview/storage/jobqueue"
	"github.com/riverqueue/river"
)

// PRPolicyForceApproveJobArgs posts a force-approval to the PR/MR after a human
// overrode a blocked policy verdict (the review_policy_overrides row is written
// by the API handler before this job is enqueued).
type PRPolicyForceApproveJobArgs struct {
	OrgID       int64  `json:"org_id"`
	ReviewID    int64  `json:"review_id"`
	ActorUserID int64  `json:"actor_user_id"`
	Reason      string `json:"reason"`
}

func (PRPolicyForceApproveJobArgs) Kind() string { return "pr_policy_force_approve" }

type PRPolicyForceApproveWorker struct {
	river.WorkerDefaults[PRPolicyForceApproveJobArgs]
	db           *sql.DB
	webhookStore *storagejobqueue.WebhookStore
}

func (w *PRPolicyForceApproveWorker) Work(ctx context.Context, job *river.Job[PRPolicyForceApproveJobArgs]) error {
	args := job.Args

	// Idempotency claim so River retries don't approve twice.
	res, err := w.db.ExecContext(ctx,
		`UPDATE reviews SET metadata = metadata || jsonb_build_object('policy_force_published_at', now())
		 WHERE id = $1 AND org_id = $2 AND NOT (metadata ? 'policy_force_published_at')`,
		args.ReviewID, args.OrgID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	release := func() {
		_, _ = w.db.ExecContext(ctx, `UPDATE reviews SET metadata = metadata - 'policy_force_published_at' WHERE id = $1`, args.ReviewID)
	}

	var pullRequestID sql.NullInt64
	if err := w.db.QueryRowContext(ctx,
		`SELECT pull_request_id FROM reviews WHERE id = $1 AND org_id = $2`,
		args.ReviewID, args.OrgID).Scan(&pullRequestID); err != nil {
		release()
		return err
	}
	if !pullRequestID.Valid {
		release()
		return nil
	}

	var repositoryID int64
	var number int
	if err := w.db.QueryRowContext(ctx,
		`SELECT repository_id, number FROM pull_requests WHERE id = $1 AND org_id = $2`,
		pullRequestID.Int64, args.OrgID).Scan(&repositoryID, &number); err != nil {
		release()
		return err
	}
	info, err := loadRepositoryConnectorInfo(w.db, repositoryID)
	if err != nil {
		release()
		return err
	}

	actorName := fmt.Sprintf("user %d", args.ActorUserID)
	if args.ActorUserID > 0 {
		var email string
		if err := w.db.QueryRowContext(ctx, `SELECT email FROM users WHERE id = $1`, args.ActorUserID).Scan(&email); err == nil && email != "" {
			actorName = email
		}
	}

	body := fmt.Sprintf("## ⚠️ LiveReview policy: Force-approved\n\n"+
		"Force-approved by **%s** despite a policy block.\n\n"+
		"**Reason:** %s\n\n"+
		"<sub>This override is recorded in LiveReview's audit log.</sub>\n", actorName, args.Reason)

	// Show the override in the PR's decisions table; never fail the approval over it.
	if w.webhookStore != nil {
		if host, err := w.webhookStore.GetWebhookPublicEndpoint(ctx); err == nil && host != "" {
			if err := refreshPRSection(ctx, w.db, info, host, pullRequestID.Int64, number); err != nil {
				log.Printf("[WARN] pr policy force-approve: failed to update PR description for review %d: %v", args.ReviewID, err)
			}
		}
	}

	switch info.Provider {
	case "github", "github-com", "github-enterprise":
		if err := providergithub.SubmitPullRequestReview(ctx, info.ProviderURL, info.token(), info.FullName, number, "APPROVE", body); err != nil {
			log.Printf("[WARN] pr policy force-approve: APPROVE failed for review %d, posting as comment: %v", args.ReviewID, err)
			return providergithub.PostIssueComment(ctx, info.ProviderURL, info.token(), info.FullName, number, body)
		}
		return nil
	case "gitlab", "gitlab-com", "gitlab-self-hosted":
		client := providergitlab.NewHTTPClient(info.ProviderURL, info.token())
		if err := client.CreateMRGeneralComment(info.ProviderRepoID, number, body); err != nil {
			return err
		}
		if err := client.ApproveMR(info.ProviderRepoID, number); err != nil {
			log.Printf("[WARN] pr policy force-approve: approve failed for review %d: %v", args.ReviewID, err)
		}
		return nil
	default:
		return nil
	}
}

// QueuePRPolicyForceApprove enqueues the force-approve publish.
func (jq *JobQueue) QueuePRPolicyForceApprove(ctx context.Context, args PRPolicyForceApproveJobArgs) error {
	_, err := jq.client.Insert(ctx, args, &river.InsertOpts{Queue: "repo_sync", MaxAttempts: 5})
	if err != nil {
		return fmt.Errorf("failed to queue force-approve: %w", err)
	}
	return nil
}
