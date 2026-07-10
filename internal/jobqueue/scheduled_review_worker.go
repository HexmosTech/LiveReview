package jobqueue

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/livereview/internal/aiselection"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/naming"
	githubprovider "github.com/livereview/internal/providers/github"
	"github.com/livereview/internal/review"
	reviewprocessor "github.com/livereview/internal/review_processor"
	"github.com/livereview/pkg/models"
	storagelicense "github.com/livereview/storage/license"
	scheduledreviewstore "github.com/livereview/storage/scheduledreview"
	"github.com/riverqueue/river"
)

// ScheduledReviewJobArgs represents the arguments for an asynchronous scheduled-review run.
type ScheduledReviewJobArgs struct {
	ConfigID int64 `json:"config_id"`
}

// Kind returns the job kind for River routing.
func (ScheduledReviewJobArgs) Kind() string {
	return "scheduled_review"
}

// ScheduledReviewWorker diffs a repo's default branch since the last checkpoint and runs
// it through the standard AI review engine. Unlike other review flows, it deliberately
// does NOT persist the fetched diff/code anywhere — only the AI's comments and a
// base/head SHA pointer are stored, so the diff can be re-fetched live from GitHub when a
// user views the review (see GetDiffReviewStatus in internal/api/diff_review.go).
type ScheduledReviewWorker struct {
	river.WorkerDefaults[ScheduledReviewJobArgs]
	db *sql.DB
	jq *JobQueue
}

// Timeout overrides the default River 60s timeout to allow longer AI reviews.
func (w *ScheduledReviewWorker) Timeout(job *river.Job[ScheduledReviewJobArgs]) time.Duration {
	return 10 * time.Minute
}

// Work implements the River Worker interface.
func (w *ScheduledReviewWorker) Work(ctx context.Context, job *river.Job[ScheduledReviewJobArgs]) error {
	args := job.Args
	if w.db == nil {
		return fmt.Errorf("database connection not available")
	}
	store := scheduledreviewstore.NewStore(w.db)

	cfg, err := store.GetByID(ctx, args.ConfigID)
	if err != nil {
		log.Printf("[ERROR] ScheduledReviewWorker: config %d not found: %v", args.ConfigID, err)
		return nil // config may have been deleted since the job was queued; don't retry.
	}
	if !cfg.Enabled {
		return nil
	}

	nextRunAt := time.Now().Add(time.Duration(cfg.IntervalHours) * time.Hour)

	var provider, providerURL, patToken string
	err = w.db.QueryRowContext(ctx, `SELECT provider, provider_url, pat_token FROM integration_tokens WHERE id = $1`, cfg.IntegrationTokenID).
		Scan(&provider, &providerURL, &patToken)
	if err != nil {
		log.Printf("[ERROR] ScheduledReviewWorker: failed to load integration token %d for config %d: %v", cfg.IntegrationTokenID, cfg.ID, err)
		return fmt.Errorf("failed to load integration token: %w", err)
	}
	if !strings.HasPrefix(provider, "github") {
		log.Printf("[WARN] ScheduledReviewWorker: config %d has unsupported provider %q; skipping", cfg.ID, provider)
		return store.UpdateCheckpoint(ctx, cfg.ID, cfg.DefaultBranch.String, cfg.LastSyncedSHA.String, time.Now(), nextRunAt)
	}

	parts := strings.SplitN(cfg.ProjectFullName, "/", 2)
	if len(parts) != 2 {
		log.Printf("[ERROR] ScheduledReviewWorker: invalid project_full_name %q for config %d; disabling", cfg.ProjectFullName, cfg.ID)
		return nil
	}
	owner, repo := parts[0], parts[1]

	ghProvider := githubprovider.NewGitHubProvider(patToken)

	branch := cfg.DefaultBranch.String
	if branch == "" {
		branch, err = ghProvider.GetDefaultBranch(ctx, owner, repo)
		if err != nil {
			return fmt.Errorf("failed to resolve default branch for %s: %w", cfg.ProjectFullName, err)
		}
	}

	headSHA, err := ghProvider.GetBranchHeadSHA(ctx, owner, repo, branch)
	if err != nil {
		return fmt.Errorf("failed to resolve branch head for %s: %w", cfg.ProjectFullName, err)
	}

	baseSHA := cfg.LastSyncedSHA.String
	if baseSHA == "" {
		lookback := time.Now().Add(-time.Duration(cfg.IntervalHours) * time.Hour)
		baseSHA, err = ghProvider.GetCommitBefore(ctx, owner, repo, branch, lookback)
		if err != nil {
			return fmt.Errorf("failed to resolve lookback commit for %s: %w", cfg.ProjectFullName, err)
		}
	}

	// Nothing changed (or no history before the lookback window yet) — just checkpoint.
	if baseSHA == "" || baseSHA == headSHA {
		return store.UpdateCheckpoint(ctx, cfg.ID, branch, headSHA, time.Now(), nextRunAt)
	}

	diffs, err := ghProvider.GetCompareChanges(ctx, owner, repo, baseSHA, headSHA)
	if err != nil {
		return fmt.Errorf("failed to fetch compare diff for %s (%s...%s): %w", cfg.ProjectFullName, baseSHA, headSHA, err)
	}
	if len(diffs) == 0 {
		return store.UpdateCheckpoint(ctx, cfg.ID, branch, headSHA, time.Now(), nextRunAt)
	}

	planCode, err := resolveOrgPlanCode(ctx, w.db, cfg.OrgID)
	if err != nil {
		return fmt.Errorf("failed to resolve plan for org %d: %w", cfg.OrgID, err)
	}

	// Coarse safety gate: if the org is already blocked (quota exhausted / trial read-only),
	// don't let the scheduler keep spending AI budget silently in the background.
	quotaModule := license.NewQuotaModule(w.db)
	preflight, err := quotaModule.PreflightCheck(ctx, license.QuotaPreflightInput{OrgID: cfg.OrgID, RequiredLOC: 0, PlanCode: planCode})
	if err == nil && preflight.Blocked {
		log.Printf("[WARN] ScheduledReviewWorker: org %d is over quota; skipping scheduled review for %s", cfg.OrgID, cfg.ProjectFullName)
		return store.UpdateCheckpoint(ctx, cfg.ID, branch, headSHA, time.Now(), nextRunAt)
	}

	selection, err := aiselection.GetReviewAISelection(ctx, w.db, cfg.OrgID, planCode)
	if err != nil {
		return fmt.Errorf("failed to load AI config for org %d: %w", cfg.OrgID, err)
	}

	friendlyName := naming.GenerateFriendlyName()
	connectorID := cfg.IntegrationTokenID
	rm := reviewprocessor.NewReviewManager(w.db)
	initialMeta := map[string]interface{}{
		"source":         "scheduled",
		"connector_id":   cfg.IntegrationTokenID,
		"repo_full_name": cfg.ProjectFullName,
		"branch":         branch,
		"base_sha":       baseSHA,
		"head_sha":       headSHA,
	}
	reviewRecord, err := rm.CreateReviewWithOrg(cfg.ProjectFullName, branch, headSHA, "", "scheduled", "", provider, &connectorID, initialMeta, cfg.OrgID, friendlyName, "", "")
	if err != nil {
		return fmt.Errorf("failed to create review record: %w", err)
	}
	_ = rm.UpdateReviewStatus(reviewRecord.ID, "in_progress")

	reviewRequest := review.ReviewRequest{
		URL:              fmt.Sprintf("scheduled:%s@%s", cfg.ProjectFullName, headSHA),
		ReviewID:         fmt.Sprintf("%d", reviewRecord.ID),
		Provider:         review.ProviderConfig{Type: "cli", Config: map[string]interface{}{}}, // "cli" skips posting comments back — there's no PR to comment on.
		AI:               selection.Leader,
		HelperAI:         selection.Helper,
		HelperEnabled:    selection.HelperEnabled,
		HelperMode:       selection.HelperMode,
		PreloadedChanges: diffs,
	}

	result := review.NewService(
		review.NewStandardProviderFactory(),
		review.NewStandardAIProviderFactory(),
		review.DefaultReviewConfig(),
	).ProcessReview(ctx, reviewRequest)

	status := "failed"
	summary := ""
	failureReason := ""
	var comments []*models.ReviewComment
	if result != nil {
		summary = result.Summary
		comments = result.Comments
		if result.Success {
			status = "completed"
		} else if result.Error != nil {
			failureReason = result.Error.Error()
		} else {
			failureReason = "review processing encountered errors"
		}
	} else {
		failureReason = "review processing returned no result"
	}

	// Deliberately no "preloaded_changes" key here — the diff itself is never persisted.
	resultMeta := map[string]interface{}{
		"review_result": map[string]interface{}{
			"summary":  summary,
			"comments": comments,
		},
	}
	if failureReason != "" {
		resultMeta["failure_reason"] = failureReason
	}
	if err := rm.MergeReviewMetadata(reviewRecord.ID, resultMeta); err != nil {
		log.Printf("[WARN] ScheduledReviewWorker: failed to persist review_result for review %d: %v", reviewRecord.ID, err)
	}
	if err := rm.UpdateReviewStatus(reviewRecord.ID, status); err != nil {
		log.Printf("[WARN] ScheduledReviewWorker: failed to update status for review %d: %v", reviewRecord.ID, err)
	}

	if status == "completed" && result.BillableLOC > 0 && w.jq != nil {
		operationID := fmt.Sprintf("scheduled-review:%d", reviewRecord.ID)
		resolvedReviewID := reviewRecord.ID
		if err := w.jq.QueueUpdateOrgUsageJob(ctx, UpdateOrgUsageJobArgs{
			OrgID:          cfg.OrgID,
			ReviewID:       &resolvedReviewID,
			OperationType:  "scheduled_review",
			TriggerSource:  "scheduled",
			OperationID:    operationID,
			IdempotencyKey: operationID,
			Provider:       result.Provider,
			Model:          result.Model,
			Batch: license.QuotaBatchInput{
				PlanCode:                 planCode,
				Provider:                 result.Provider,
				RawLOCBatch:              result.BillableLOC,
				ProviderTotalInputTokens: result.InputTokens,
				OutputTokensBatch:        result.OutputTokens,
			},
		}); err != nil {
			log.Printf("[WARN] ScheduledReviewWorker: failed to queue billing finalization for review %d: %v", reviewRecord.ID, err)
		}
	}

	if err := store.UpdateCheckpoint(ctx, cfg.ID, branch, headSHA, time.Now(), nextRunAt); err != nil {
		log.Printf("[WARN] ScheduledReviewWorker: failed to update checkpoint for config %d: %v", cfg.ID, err)
	}

	return nil
}

// resolveOrgPlanCode looks up the org's current billing plan, defaulting new orgs to Free.
func resolveOrgPlanCode(ctx context.Context, db *sql.DB, orgID int64) (license.PlanType, error) {
	store := storagelicense.NewPlanChangeStore(db)
	if err := store.EnsureOrgBillingState(ctx, orgID, license.PlanFree30K.String()); err != nil {
		return "", fmt.Errorf("failed to ensure billing state for org=%d: %w", orgID, err)
	}
	state, err := store.GetOrgBillingState(ctx, orgID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve current plan for org=%d: %w", orgID, err)
	}
	resolved := license.PlanType(strings.TrimSpace(state.CurrentPlanCode))
	if !resolved.IsValid() {
		return "", fmt.Errorf("invalid current plan code for org=%d", orgID)
	}
	return resolved, nil
}
