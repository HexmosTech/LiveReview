package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	providergithub "github.com/livereview/internal/providers/github"
	providergitlab "github.com/livereview/internal/providers/gitlab"
	storagejobqueue "github.com/livereview/storage/jobqueue"
	"github.com/riverqueue/river"
)

// PRPolicyCheckJobArgs runs the approval policy on a finished review that was started
// from the PR comment's "Start a review" link (metadata.pr_link_policy).
type PRPolicyCheckJobArgs struct {
	OrgID    int64 `json:"org_id"`
	ReviewID int64 `json:"review_id"`
}

func (PRPolicyCheckJobArgs) Kind() string { return "pr_policy_check" }

type PRPolicyCheckWorker struct {
	river.WorkerDefaults[PRPolicyCheckJobArgs]
	db           *sql.DB
	webhookStore *storagejobqueue.WebhookStore
}

// policyFinding is the subset of models.ReviewComment (stored without JSON tags) the policy reads.
type policyFinding struct {
	FilePath   string
	Line       int
	Content    string
	Severity   string
	IsInternal bool
}

type policyResult struct {
	Policy   string `json:"policy"`
	Approved bool   `json:"approved"`
	Critical int    `json:"critical"`
	Warning  int    `json:"warning"`
	Info     int    `json:"info"`
}

// ponytail: one static policy (no critical findings); swap for org-defined policies later.
const staticPolicyName = "No critical findings"

func (w *PRPolicyCheckWorker) Work(ctx context.Context, job *river.Job[PRPolicyCheckJobArgs]) error {
	args := job.Args

	var status string
	var pullRequestID sql.NullInt64
	var flagged bool
	var commentsJSON []byte
	err := w.db.QueryRowContext(ctx, `
		SELECT status, pull_request_id,
		       COALESCE((metadata->>'pr_link_policy')::boolean, false),
		       COALESCE(metadata->'review_result'->'comments', '[]'::jsonb)
		FROM reviews WHERE id = $1 AND org_id = $2`,
		args.ReviewID, args.OrgID,
	).Scan(&status, &pullRequestID, &flagged, &commentsJSON)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to load review %d: %w", args.ReviewID, err)
	}
	if status != "completed" || !pullRequestID.Valid || !flagged {
		return nil
	}

	var findings []policyFinding
	if err := json.Unmarshal(commentsJSON, &findings); err != nil {
		return fmt.Errorf("failed to parse findings for review %d: %w", args.ReviewID, err)
	}
	result := policyResult{Policy: staticPolicyName}
	var critical []policyFinding
	for _, f := range findings {
		if f.IsInternal {
			continue
		}
		switch strings.ToLower(f.Severity) {
		case "critical":
			result.Critical++
			critical = append(critical, f)
		case "warning":
			result.Warning++
		default:
			result.Info++
		}
	}
	result.Approved = result.Critical == 0

	// Store the result first; it doubles as a claim so River retries never approve twice.
	resultJSON, _ := json.Marshal(result)
	res, err := w.db.ExecContext(ctx,
		`UPDATE reviews SET metadata = metadata || jsonb_build_object('policy_result', $2::jsonb)
		 WHERE id = $1 AND NOT (metadata ? 'policy_result')`,
		args.ReviewID, string(resultJSON))
	if err != nil {
		return fmt.Errorf("failed to store policy result for review %d: %w", args.ReviewID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}

	if err := w.publish(ctx, args, pullRequestID.Int64, result, critical); err != nil {
		if _, resetErr := w.db.ExecContext(ctx, `UPDATE reviews SET metadata = metadata - 'policy_result' WHERE id = $1`, args.ReviewID); resetErr != nil {
			log.Printf("[WARN] pr policy: failed to release claim for review %d: %v", args.ReviewID, resetErr)
		}
		return err
	}
	return nil
}

// publish posts the policy comment and approves (or requests changes on) the PR/MR.
func (w *PRPolicyCheckWorker) publish(ctx context.Context, args PRPolicyCheckJobArgs, pullRequestID int64, result policyResult, critical []policyFinding) error {
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
	body := buildPolicyComment(host, args.ReviewID, result, critical)

	switch info.Provider {
	case "github", "github-com", "github-enterprise":
		event := "REQUEST_CHANGES"
		if result.Approved {
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
		if result.Approved {
			if err := client.ApproveMR(info.ProviderRepoID, number); err != nil {
				log.Printf("[WARN] pr policy: approve failed for review %d: %v", args.ReviewID, err)
			}
		}
		return nil
	default:
		return nil
	}
}

const maxListedCriticalFindings = 5

func buildPolicyComment(host string, reviewID int64, result policyResult, critical []policyFinding) string {
	var b strings.Builder
	if result.Approved {
		b.WriteString("## ✅ LiveReview policy: Approved\n\n")
	} else {
		b.WriteString("## ❌ LiveReview policy: Changes requested\n\n")
	}
	fmt.Fprintf(&b, "Review #%d · %d critical · %d warning · %d info\n\n", reviewID, result.Critical, result.Warning, result.Info)
	b.WriteString("| Policy | Result |\n|---|---|\n")
	if result.Approved {
		fmt.Fprintf(&b, "| %s | ✅ Passed |\n", result.Policy)
	} else {
		fmt.Fprintf(&b, "| %s | ❌ %d critical |\n\n", result.Policy, result.Critical)
		for i, f := range critical {
			if i == maxListedCriticalFindings {
				fmt.Fprintf(&b, "- …and %d more\n", len(critical)-maxListedCriticalFindings)
				break
			}
			fmt.Fprintf(&b, "- `%s:%d` — %s\n", f.FilePath, f.Line, firstLine(f.Content, 120))
		}
	}
	if host != "" {
		fmt.Fprintf(&b, "\n[View full review](%s/#/reviews/%d)\n", strings.TrimSuffix(host, "/"), reviewID)
	}
	return b.String()
}

// firstLine returns the first non-empty line of s, cut to limit runes.
func firstLine(s string, limit int) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if r := []rune(line); len(r) > limit {
			return string(r[:limit]) + "…"
		}
		return line
	}
	return ""
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
