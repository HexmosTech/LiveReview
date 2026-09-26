package seed_demo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/livereview/storage/license"
	storageseeddemo "github.com/livereview/storage/seed_demo"
)

// cloneOne clones one new synthetic review (plus review_commits, recent_activity, and
// review_events) from src via storageseeddemo.Store.CloneReview, then accounts its LOC
// usage. If accounting fails, the review is deleted again (storage/license.AccountSuccess's
// idempotency key is scoped to the new review's id, so it can only run after the review
// row exists - the two can't share one transaction) so a review is never left behind
// without its usage accounted.
func cloneOne(ctx context.Context, store *storageseeddemo.Store, db *sql.DB, orgID int64, src storageseeddemo.SourceReview, day time.Time) error {
	commitSHA, err := randomCommitSHA()
	if err != nil {
		return fmt.Errorf("generate commit sha: %w", err)
	}
	createdAt, err := randomTimeInDay(day, time.Now())
	if err != nil {
		return fmt.Errorf("pick created_at: %w", err)
	}
	startedAt := createdAt.Add(time.Duration(1+mustRandomCount(0, 2)) * time.Second)
	completedAt := startedAt.Add(time.Duration(30+mustRandomCount(0, 60)) * time.Second)

	metadata, err := clonedMetadata(src.Metadata, src.ID, day)
	if err != nil {
		return fmt.Errorf("build cloned metadata: %w", err)
	}

	activityJSON, err := json.Marshal(map[string]interface{}{
		"branch":       src.Branch,
		"provider":     src.Provider.String,
		"repository":   src.Repository,
		"user_email":   src.UserEmail.String,
		"commit_hash":  "",
		"original_url": src.PrMrURL,
		"trigger_type": src.TriggerType,
	})
	if err != nil {
		return fmt.Errorf("marshal activity data: %w", err)
	}

	stageEvents, err := buildStageEvents(startedAt, completedAt, commentCount(src.Metadata))
	if err != nil {
		return fmt.Errorf("build stage events: %w", err)
	}

	reviewID, err := store.CloneReview(ctx, storageseeddemo.NewReview{
		OrgID:             orgID,
		Repository:        src.Repository,
		Branch:            src.Branch,
		PrMrURL:           src.PrMrURL,
		ConnectorID:       src.ConnectorID,
		TriggerType:       src.TriggerType,
		UserEmail:         src.UserEmail,
		Provider:          src.Provider,
		Metadata:          metadata,
		MRTitle:           src.MRTitle,
		AuthorName:        src.AuthorName,
		AuthorUsername:    src.AuthorUsername,
		FriendlyName:      src.FriendlyName,
		PullRequestID:     src.PullRequestID,
		CreatedAt:         createdAt,
		StartedAt:         startedAt,
		CompletedAt:       completedAt,
		CommitSHA:         commitSHA,
		RepositoryID:      src.RepositoryID,
		ActivityEventData: activityJSON,
		StageEvents:       stageEvents,
	})
	if err != nil {
		return fmt.Errorf("clone review: %w", err)
	}

	if src.BillableLOC.Valid && src.BillableLOC.Int64 > 0 {
		if err := accountUsage(ctx, store, db, orgID, reviewID, src); err != nil {
			if delErr := store.DeleteReview(ctx, reviewID); delErr != nil {
				log.Printf("[seed_demo] org_id=%d review_id=%d: accounting failed (%v) AND compensating delete failed (%v) - review left in an inconsistent state, needs manual cleanup", orgID, reviewID, err, delErr)
			}
			return fmt.Errorf("account usage for review %d: %w", reviewID, err)
		}
	}

	return nil
}

func accountUsage(ctx context.Context, store *storageseeddemo.Store, db *sql.DB, orgID, reviewID int64, src storageseeddemo.SourceReview) error {
	billableLOC, err := jitterInt64(src.BillableLOC.Int64, 0.2)
	if err != nil {
		return err
	}

	var inputTokens, outputTokens *int64
	if src.InputTokens.Valid {
		v, err := jitterInt64(src.InputTokens.Int64, 0.2)
		if err != nil {
			return err
		}
		inputTokens = &v
	}
	if src.OutputTokens.Valid {
		v, err := jitterInt64(src.OutputTokens.Int64, 0.2)
		if err != nil {
			return err
		}
		outputTokens = &v
	}
	var costUSD *float64
	if src.LLMCostUSD.Valid {
		v, err := jitterFloat64(src.LLMCostUSD.Float64, 0.2)
		if err != nil {
			return err
		}
		costUSD = &v
	}

	var actorUserID *int64
	if src.ActorUserID.Valid {
		v := src.ActorUserID.Int64
		actorUserID = &v
	}

	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0)

	planCode, monthlyLimit, err := store.CurrentPlan(ctx, orgID)
	if err != nil {
		return fmt.Errorf("load current plan: %w", err)
	}

	accountingStore := license.NewLOCAccountingStore(db)
	return accountingStore.AccountSuccess(ctx, license.AccountSuccessRecord{
		OrgID:              orgID,
		ReviewID:           &reviewID,
		ActorUserID:        actorUserID,
		ActorEmail:         src.UserEmail.String,
		OperationType:      "manual_review",
		TriggerSource:      "manual",
		OperationID:        fmt.Sprintf("seed-demo:%d", reviewID),
		IdempotencyKey:     fmt.Sprintf("seed-demo:%d", reviewID),
		BillableLOC:        billableLOC,
		BillingPeriodStart: periodStart,
		BillingPeriodEnd:   periodEnd,
		PlanCode:           planCode,
		MonthlyLOCLimit:    monthlyLimit,
		Provider:           src.LOCProvider.String,
		Model:              src.LOCModel.String,
		PricingVersion:     src.LOCPricing.String,
		InputTokens:        inputTokens,
		OutputTokens:       outputTokens,
		CostUSD:            costUSD,
	})
}

// stageEvent is one "Stage started"/"Stage completed" log line, matching the exact
// message format internal/logging.ReviewLogger.EmitStageStarted/EmitStageCompleted
// writes for a real review, so ui/src/components/reviews/ReviewProgressView.tsx's
// keyword matching renders all five stages as completed instead of stuck pending.
type stageEvent struct {
	level   string
	message string
}

// buildStageEvents fabricates the same 10 stage-progress log events (5 stages x
// started/completed) a real review run would have emitted, spread evenly across
// [startedAt, completedAt]. Without these, review_events stays empty for a synthetic
// review and its Progress tab shows 0%/all-pending even though the review itself is
// complete - the same gap real reviews in this org have today (review_events is empty
// for every review here, seeded or not), which this at least fixes going forward for
// seeded ones.
func buildStageEvents(startedAt, completedAt time.Time, comments int) ([]storageseeddemo.StageEvent, error) {
	fileCount, err := randomCount(1, 6)
	if err != nil {
		return nil, err
	}

	templates := []stageEvent{
		{"info", "Stage started: Preparation"},
		{"success", "Stage completed successfully: Preparation - Providers initialized and configured"},
		{"info", "Stage started: Analysis"},
		{"success", fmt.Sprintf("Stage completed successfully: Analysis - Retrieved %d changed files from merge request", fileCount)},
		{"info", "Stage started: Review"},
		{"success", fmt.Sprintf("Stage completed successfully: Review - Generated %d comments and summary", comments)},
		{"info", "Stage started: Artifact Generation"},
		{"success", fmt.Sprintf("Stage completed successfully: Artifact Generation - Posted %d comments to merge request", comments)},
		{"info", "Stage started: Finalization"},
		{"success", "Stage completed successfully: Finalization - Review process completed successfully"},
	}

	span := completedAt.Sub(startedAt)
	step := span / time.Duration(len(templates))
	if step <= 0 {
		step = time.Second
	}

	events := make([]storageseeddemo.StageEvent, len(templates))
	for i, t := range templates {
		events[i] = storageseeddemo.StageEvent{
			Timestamp: startedAt.Add(time.Duration(i) * step),
			Level:     t.level,
			Message:   t.message,
		}
	}
	return events, nil
}

// commentCount reads len(metadata.review_result.comments), defaulting to a plausible
// small number if the source's review_result is shaped unexpectedly.
func commentCount(metadata json.RawMessage) int {
	var wrapper struct {
		ReviewResult struct {
			Comments []json.RawMessage `json:"comments"`
		} `json:"review_result"`
	}
	if err := json.Unmarshal(metadata, &wrapper); err != nil {
		return 3
	}
	if n := len(wrapper.ReviewResult.Comments); n > 0 {
		return n
	}
	return 3
}

// clonedMetadata copies the source review's metadata verbatim (including
// preloaded_changes/review_result, so the diff/findings viewer and dashboard
// aggregation work unchanged) and tags it as synthetic.
func clonedMetadata(sourceMetadata json.RawMessage, sourceReviewID int64, day time.Time) (json.RawMessage, error) {
	m := map[string]interface{}{}
	if len(sourceMetadata) > 0 {
		if err := json.Unmarshal(sourceMetadata, &m); err != nil {
			return nil, fmt.Errorf("unmarshal source metadata: %w", err)
		}
	}
	m["seed_demo"] = true
	m["seed_demo_source_review_id"] = sourceReviewID
	m["seed_demo_run_date"] = day.Format("2006-01-02")
	return json.Marshal(m)
}

func mustRandomCount(min, max int) int {
	n, err := randomCount(min, max)
	if err != nil {
		return min
	}
	return n
}
