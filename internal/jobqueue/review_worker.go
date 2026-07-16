package jobqueue

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/aidefault"
	"github.com/livereview/internal/diffutil"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/lrcconfig"
	"github.com/livereview/internal/review"
	reviewprocessor "github.com/livereview/internal/review_processor"
	"github.com/livereview/pkg/models"
	storageaiconnectors "github.com/livereview/storage/aiconnectors"
	storagetools "github.com/livereview/storage/tools"
	"github.com/riverqueue/river"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

// WebhookReviewJobArgs represents the arguments for an asynchronous webhook review job.
type WebhookReviewJobArgs struct {
	OrgID        int64  `json:"org_id"`
	ConnectorID  int64  `json:"connector_id"`
	EventJSON    string `json:"event_json"`
	ScenarioType string `json:"scenario_type"`
}

// Kind returns the job kind for River routing.
func (WebhookReviewJobArgs) Kind() string {
	return "webhook_review"
}

// WebhookReviewWorker handles async webhook reviews.
type WebhookReviewWorker struct {
	river.WorkerDefaults[WebhookReviewJobArgs]
	jq *JobQueue
}

// Timeout overrides the default River 60s timeout to allow longer AI reviews.
func (w *WebhookReviewWorker) Timeout(job *river.Job[WebhookReviewJobArgs]) time.Duration {
	return 10 * time.Minute
}

// Work implements the River Worker interface.
func (w *WebhookReviewWorker) Work(ctx context.Context, job *river.Job[WebhookReviewJobArgs]) error {
	args := job.Args
	if w.jq == nil || w.jq.db == nil {
		log.Printf("[ERROR] Database connection not available on JobQueue")
		return fmt.Errorf("database connection not available")
	}
	return reviewprocessor.ProcessWebhookReview(ctx, w.jq.db, args.OrgID, args.ConnectorID, args.EventJSON, args.ScenarioType)
}

// ManualReviewJobArgs represents the arguments for an asynchronous manual dashboard review job.
type ManualReviewJobArgs struct {
	OrgID       int64  `json:"org_id"`
	PlanCode    string `json:"plan_code"`
	ActorUserID *int64 `json:"actor_user_id,omitempty"`
	ActorEmail  string `json:"actor_email"`
	ReviewID    int64  `json:"review_id"`
	RequestJSON string `json:"request_json"`
}

// Kind returns the job kind for River routing.
func (ManualReviewJobArgs) Kind() string {
	return "manual_review"
}

// ManualReviewWorker handles async manual reviews.
type ManualReviewWorker struct {
	river.WorkerDefaults[ManualReviewJobArgs]
	jq *JobQueue
}

// Timeout overrides the default River 60s timeout to allow longer AI reviews.
func (w *ManualReviewWorker) Timeout(job *river.Job[ManualReviewJobArgs]) time.Duration {
	return 10 * time.Minute
}

// Work implements the River Worker interface.
func (w *ManualReviewWorker) Work(ctx context.Context, job *river.Job[ManualReviewJobArgs]) error {
	args := job.Args
	if w.jq == nil || w.jq.db == nil {
		log.Printf("[ERROR] Database connection not available on JobQueue")
		return fmt.Errorf("database connection not available")
	}

	err := reviewprocessor.ProcessManualReview(ctx, w.jq.db, args.OrgID, args.PlanCode, args.ActorUserID, args.ActorEmail, args.ReviewID, args.RequestJSON,
		func(ctx context.Context, model string, batch license.QuotaBatchInput, extraMeta map[string]interface{}) error {
			operationID := fmt.Sprintf("manual-review:%d", args.ReviewID)
			idempotencyKey := operationID
			return w.jq.QueueUpdateOrgUsageJob(ctx, UpdateOrgUsageJobArgs{
				OrgID:          args.OrgID,
				ReviewID:       &args.ReviewID,
				ActorUserID:    args.ActorUserID,
				ActorEmail:     args.ActorEmail,
				OperationType:  "manual_review",
				TriggerSource:  "manual",
				OperationID:    operationID,
				IdempotencyKey: idempotencyKey,
				Provider:       batch.Provider,
				Model:          model,
				Batch:          batch,
				ExtraMeta:      extraMeta,
			})
		},
	)
	if err != nil {
		return err
	}

	// After AI review completes, fan-out to tool jobs if any tools are enabled.
	w.maybeQueueToolJobs(ctx, args.OrgID, args.ReviewID)
	return nil
}

// maybeQueueToolJobs checks whether any tools are enabled for the org and, if so,
// checks credits and queues a ToolReviewOrchestratorJob for the completed review.
func (w *ManualReviewWorker) maybeQueueToolJobs(ctx context.Context, orgID, reviewID int64) {
	toolsStore := storagetools.NewToolsStore(w.jq.db)
	enabledTools, err := toolsStore.GetEnabledToolsForOrg(ctx, orgID)
	if err != nil {
		log.Printf("[WARN] ManualReviewWorker: failed to fetch enabled tools for org %d: %v", orgID, err)
		return
	}
	if len(enabledTools) == 0 {
		return
	}

	var totalMultiplier float64
	for _, t := range enabledTools {
		totalMultiplier += t.Multiplier
	}

	creditStore := storagetools.NewCreditStore(w.jq.db)
	if err := creditStore.CheckCreditPreflight(ctx, orgID, totalMultiplier); err != nil {
		log.Printf("[WARN] ManualReviewWorker: insufficient tool credits for org %d: %v", orgID, err)
		return
	}

	// Read pr_mr_url, connector_id, provider from the review row.
	var prURL, provider string
	var connectorID sql.NullInt64
	qErr := w.jq.db.QueryRowContext(ctx,
		`SELECT COALESCE(pr_mr_url, ''), COALESCE(connector_id, 0), COALESCE(provider, '')
		   FROM public.reviews WHERE id = $1 AND org_id = $2`,
		reviewID, orgID,
	).Scan(&prURL, &connectorID, &provider)
	if qErr != nil {
		log.Printf("[WARN] ManualReviewWorker: failed to read review row for tool job (review=%d): %v", reviewID, qErr)
		return
	}

	var connID int64
	if connectorID.Valid {
		connID = connectorID.Int64
	}

	if err := w.jq.QueueToolReviewOrchestratorJob(ctx, reviewID, orgID, prURL, connID, provider, totalMultiplier); err != nil {
		log.Printf("[WARN] ManualReviewWorker: failed to queue tool orchestrator for review %d: %v", reviewID, err)
	} else {
		log.Printf("[INFO] ManualReviewWorker: queued tool orchestrator for review %d", reviewID)
	}
}

// DiffReviewJobArgs represents the arguments for an asynchronous diff review job.
// The raw base64 ZIP payload is passed directly in the job args and stored in
// PostgreSQL via River's TOAST storage, avoiding bloating the reviews table.
type DiffReviewJobArgs struct {
	ReviewID      int64  `json:"review_id"`
	OrgID         int64  `json:"org_id"`
	PlanCode      string `json:"plan_code"`
	ActorUserID   int64  `json:"actor_user_id"`
	ActorEmail    string `json:"actor_email"`
	RepoName      string `json:"repo_name"`
	DiffZipBase64 string `json:"diff_zip_base64"`
	TriggerSource string `json:"trigger_source"`
	ToolsOnly     bool   `json:"tools_only"`
}

// Kind returns the job kind for River routing.
func (DiffReviewJobArgs) Kind() string {
	return "diff_review"
}

// DiffReviewWorker handles async diff review jobs picked from the "review" queue.
type DiffReviewWorker struct {
	river.WorkerDefaults[DiffReviewJobArgs]
	db   *sql.DB
	pool *pgxpool.Pool
	jq   *JobQueue
}

// Timeout overrides the default River 60s timeout to allow longer AI reviews.
func (w *DiffReviewWorker) Timeout(job *river.Job[DiffReviewJobArgs]) time.Duration {
	return 10 * time.Minute
}

// Work implements the River Worker interface to execute the full review pipeline.
func (w *DiffReviewWorker) Work(ctx context.Context, job *river.Job[DiffReviewJobArgs]) error {
	args := job.Args

	// 1. Initialize logger with event sink for UI polling stream
	logger, err := logging.StartReviewLoggingWithIDs(fmt.Sprintf("%d", args.ReviewID), args.ReviewID, args.OrgID)
	if err != nil {
		log.Printf("[ERROR] Failed to start logging for review %d: %v", args.ReviewID, err)
	}

	eventSink := reviewprocessor.NewDatabaseEventSink(w.db)
	if logger != nil {
		logger.SetEventSink(eventSink)
		defer logger.Close()
		logger.LogSection("CLI DIFF REVIEW STARTED")
		logger.Log("Review ID: %d", args.ReviewID)
		logger.Log("Organization ID: %d", args.OrgID)
		logger.Log("Processing diff from CLI...")
	}

	// 2. Decode and parse base64 ZIP payload
	if logger != nil {
		logger.Log("Decompressing and parsing diff files...")
	}
	localDiffs, lrcBundle, err := diffutil.ParseDiffZipBase64(args.DiffZipBase64)
	if err != nil {
		w.handleFailure(ctx, args, logger, eventSink, fmt.Sprintf("failed to parse diff: %v", err), "failed_to_parse_zip")
		return nil // Return nil so River marks job succeeded; business-level failure already handled.
	}

	// 2b. Apply .lrc/ignore (if present) before computing billable LOC, so
	// ignored files affect neither the AI input nor billing.
	var excludedFiles []string
	ignorePatterns, ignoreIssues := lrcconfig.LoadIgnorePatterns(lrcBundle)
	if len(ignoreIssues) > 0 && logger != nil {
		logger.Log("[WARN] .lrc/ignore: %v", ignoreIssues)
	}
	if len(ignorePatterns) > 0 {
		filtered, excluded := lrcconfig.FilterDiffs(localDiffs, ignorePatterns)
		localDiffs = filtered
		excludedFiles = excluded
		if len(excluded) > 0 && logger != nil {
			logger.Log("Excluded %d files by .lrc/ignore: %v", len(excluded), excluded)
		}
	}

	// 2c. Build the Repository Rules bundle for prompt injection, truncating
	// (with a warning) rather than failing the review if oversized.
	repoRules, rulesCharCount, rulesIssues := lrcconfig.BuildRulesBundle(lrcBundle)
	if rulesCharCount > lrcconfig.CharLimit {
		if logger != nil {
			logger.Log("[WARN] .lrc rules bundle (%d chars) exceeds limit (%d), truncating: %v", rulesCharCount, lrcconfig.CharLimit, rulesIssues)
		}
		repoRules = lrcconfig.TruncateAtLineBoundary(repoRules, lrcconfig.CharLimit)
	}

	// 3. Calculate Lines of Code
	billableLOC := diffutil.CalculateEffectiveDiffLOCFromLocalDiffs(localDiffs)

	// 5. Convert diffs and persist preloaded_changes for UI polling
	modelDiffs := diffutil.ConvertLocalDiffs(localDiffs)
	rm := reviewprocessor.NewReviewManager(w.db)
	if err := rm.MergeReviewMetadata(args.ReviewID, map[string]interface{}{
		"preloaded_changes":      modelDiffs,
		"operation_billable_loc": billableLOC,
		"excluded_files":         excludedFiles,
	}); err != nil {
		log.Printf("[WARN] failed to store preloaded_changes for review %d: %v", args.ReviewID, err)
	}

	// If .lrc/ignore excluded every changed file, there's nothing for the AI
	// to review — complete immediately rather than running an empty review.
	if len(localDiffs) == 0 && len(excludedFiles) > 0 {
		summary := fmt.Sprintf("All %d changed file(s) excluded by .lrc/ignore: %s",
			len(excludedFiles), diffutil.FormatExcludedFiles(excludedFiles))
		if err := rm.MergeReviewMetadata(args.ReviewID, map[string]interface{}{
			"review_result": map[string]interface{}{
				"summary":  summary,
				"comments": nil,
			},
		}); err != nil {
			log.Printf("[WARN] failed to store review_result for review %d: %v", args.ReviewID, err)
		}
		if err := rm.UpdateReviewStatus(args.ReviewID, "completed"); err != nil {
			log.Printf("[WARN] failed to mark review %d completed: %v", args.ReviewID, err)
		}
		if logger != nil {
			logger.Log("All files ignored. Completed review immediately.")
		}
		return nil
	}

	// 4. Quota Preflight Check
	planCode := license.PlanType(args.PlanCode)
	if planCode == "" {
		planCode = license.PlanFree30K
	}

	quotaModule := license.NewQuotaModule(w.db)
	preflightResult, err := quotaModule.PreflightCheck(ctx, license.QuotaPreflightInput{
		OrgID:       args.OrgID,
		RequiredLOC: billableLOC,
		PlanCode:    planCode,
	})
	if err != nil {
		w.handleFailure(ctx, args, logger, eventSink, fmt.Sprintf("failed quota preflight: %v", err), "failed_quota_preflight")
		return nil
	}

	if preflightResult.Blocked {
		errorCode := "quota_exceeded"
		errorMessage := fmt.Sprintf("Operation requires %d LOC, but you only have %d remaining this month. Upgrade your plan to continue.",
			billableLOC, preflightResult.LOCRemainingMonth)
		if preflightResult.BlockReason == "trial_readonly" {
			errorCode = "trial_readonly"
			errorMessage = "Trial period ended; review operations are read-only until plan update"
		}
		w.handleFailure(ctx, args, logger, eventSink, errorMessage, errorCode)
		return nil
	}

	status := "failed"
	summary := ""
	var comments []*models.ReviewComment
	failureReason := ""

	if args.ToolsOnly {
		status = "completed"
		summary = "### Static Analysis Tools Review Only\n\nAI review skipped due to --tools flag."
		if logger != nil {
			logger.LogSection("PROCESSING STATIC ANALYSIS REVIEW")
			logger.Log("AI review skipped. Triggering static analysis tools...")
		}
	} else {
		// 7. Load AI Configuration
		selection, err := w.getReviewAISelectionFromDatabase(ctx, args.OrgID, planCode)
		if err != nil {
			w.handleFailure(ctx, args, logger, eventSink, fmt.Sprintf("failed to load AI config: %v", err), "failed_to_load_ai_config")
			return nil
		}

		reviewRequest := review.ReviewRequest{
			URL:              fmt.Sprintf("cli-diff:%s", args.RepoName),
			ReviewID:         fmt.Sprintf("%d", args.ReviewID),
			Provider:         review.ProviderConfig{Type: "cli", URL: "", Token: "", Config: map[string]interface{}{}},
			AI:               selection.Leader,
			HelperAI:         selection.Helper,
			HelperEnabled:    selection.HelperEnabled,
			HelperMode:       selection.HelperMode,
			PreloadedChanges: modelDiffs,
			RepoRules:        repoRules,
		}

		if logger != nil {
			logger.LogSection("PROCESSING REVIEW")
			logger.Log("Analyzing changes and generating comments...")
		}

		// 8. Execute AI Review Engine
		var aiFactory review.AIProviderFactory = review.NewStandardAIProviderFactory()
		if mockFactory, ok := getMockAIFactory(); ok {
			aiFactory = mockFactory
		}

		result := review.NewService(
			review.NewStandardProviderFactory(),
			aiFactory,
			review.DefaultReviewConfig(),
		).ProcessReview(ctx, reviewRequest)

		if result != nil {
			if result.Success {
				status = "completed"
				if err := rm.MergeReviewMetadata(args.ReviewID, buildQueuedReviewAIMetadata(&reviewRequest, result)); err != nil {
					log.Printf("[WARN] failed to persist AI stage metadata for review %d: %v", args.ReviewID, err)
				}
				resolvedReviewID := args.ReviewID
				operationID := fmt.Sprintf("diff-review:%d", args.ReviewID)
				idempotencyKey := operationID
				var actorUserIDPtr *int64
				if args.ActorUserID > 0 {
					resolvedActorUserID := args.ActorUserID
					actorUserIDPtr = &resolvedActorUserID
				}

				// Queue the billing update, batch recording, and AI stage metadata asynchronously.
				extraMeta := buildQueuedReviewAIMetadata(&reviewRequest, result)

				err = w.jq.QueueUpdateOrgUsageJob(ctx, UpdateOrgUsageJobArgs{
					OrgID:          args.OrgID,
					ReviewID:       &resolvedReviewID,
					ActorUserID:    actorUserIDPtr,
					ActorEmail:     strings.TrimSpace(args.ActorEmail),
					OperationType:  "diff_review",
					TriggerSource:  args.TriggerSource,
					OperationID:    operationID,
					IdempotencyKey: idempotencyKey,
					Provider:       result.Provider,
					Model:          result.Model,
					Batch: license.QuotaBatchInput{
						PlanCode:                 planCode,
						Provider:                 result.Provider,
						RawLOCBatch:              billableLOC,
						ProviderTotalInputTokens: result.InputTokens,
						OutputTokensBatch:        result.OutputTokens,
					},
					ExtraMeta: extraMeta,
				})
				if err != nil {
					log.Printf("[WARN] failed to queue billing finalization for review %d: %v", args.ReviewID, err)
				}

				if logger != nil {
					logger.LogSection("REVIEW COMPLETED")
					logger.Log("Review ID: %d", args.ReviewID)
					logger.Log("Successfully generated %d comments", len(result.Comments))
				}
			} else {
				if result.Error != nil {
					failureReason = result.Error.Error()
				}
				if failureReason == "" {
					failureReason = "review processing encountered errors"
				}
				if logger != nil {
					logger.LogSection("REVIEW FAILED")
					logger.Log("Review processing encountered errors: %s", failureReason)
				}
			}
			summary = result.Summary
			comments = result.Comments
		} else {
			failureReason = "review processing returned no result"
			if logger != nil {
				logger.LogSection("REVIEW FAILED")
				logger.Log("Review processing returned no result")
			}
		}
	}

	// Trigger Static Analysis Tools if enabled and review hasn't failed
	var toolComments []*models.ReviewComment
	if failureReason == "" {
		awsCfg, awsErr := awsconfig.LoadDefaultConfig(ctx)
		if awsErr != nil {
			if logger != nil {
				logger.Log("[ERROR] Failed to load AWS config: %v. Skipping tools review.", awsErr)
			}
			if args.ToolsOnly {
				status = "failed"
				failureReason = fmt.Sprintf("failed to load AWS config: %v", awsErr)
			}
		} else {
			rawDiff := review.FormatDiffs(modelDiffs)
			comments, err := ExecuteToolsForReview(ctx, w.db, awsCfg, args.OrgID, args.ReviewID, rawDiff, args.DiffZipBase64, logger)
			if err != nil {
				if logger != nil {
					logger.Log("[WARN] Static analysis tools review failed: %v", err)
				}
				if args.ToolsOnly {
					status = "failed"
					failureReason = fmt.Sprintf("static analysis tools review failed: %v", err)
				}
			} else {
				toolComments = comments
			}
		}
	}
	comments = append(comments, toolComments...)

	// 10. Persist final results and update status
	type diffReviewResultPayload struct {
		Summary  string                  `json:"summary"`
		Comments []*models.ReviewComment `json:"comments"`
	}
	payload := diffReviewResultPayload{Summary: summary, Comments: comments}
	meta := map[string]interface{}{"review_result": payload}
	if failureReason != "" {
		meta["failure_reason"] = failureReason
	}
	if err := rm.MergeReviewMetadata(args.ReviewID, meta); err != nil {
		log.Printf("[WARN] failed to persist review_result for %d: %v", args.ReviewID, err)
	}

	if err := rm.UpdateReviewStatus(args.ReviewID, status); err != nil {
		log.Printf("[WARN] failed to update review status for %d: %v", args.ReviewID, err)
	}

	// Persist AI summary title for later display
	if summary != "" {
		title := extractFirstHeading(summary)
		if title != "" {
			if err := rm.MergeReviewMetadata(args.ReviewID, map[string]interface{}{"ai_summary_title": title}); err != nil {
				log.Printf("[WARN] failed to persist ai_summary_title for %d: %v", args.ReviewID, err)
			}
		}
	}


	// Emit final completion/failure event
	_ = eventSink.EmitCompletionEvent(ctx, args.ReviewID, args.OrgID, summary, len(comments), failureReason)

	return nil
}

// handleFailure marks the review as failed and emits failure events.
func (w *DiffReviewWorker) handleFailure(ctx context.Context, args DiffReviewJobArgs, logger *logging.ReviewLogger, eventSink logging.EventSink, failureReason string, errorCode string) {
	if logger != nil {
		logger.LogSection("REVIEW FAILED")
		logger.Log("Review processing encountered errors: %s", failureReason)
	}

	rm := reviewprocessor.NewReviewManager(w.db)
	_ = rm.UpdateReviewStatus(args.ReviewID, "failed")
	_ = rm.MergeReviewMetadata(args.ReviewID, map[string]interface{}{
		"failure_reason": failureReason,
		"error_code":     errorCode,
	})

	_ = eventSink.EmitCompletionEvent(ctx, args.ReviewID, args.OrgID, "", 0, failureReason)
}

// --- AI Config helpers (replicated from api/reviews_api.go) ---

type diffReviewAISelection struct {
	Leader        review.AIConfig
	Helper        *review.AIConfig
	HelperEnabled bool
	HelperMode    string
}

func (w *DiffReviewWorker) getReviewAISelectionFromDatabase(ctx context.Context, orgID int64, planCode license.PlanType) (*diffReviewAISelection, error) {
	storage := aiconnectors.NewStorage(w.db)
	leaderConnectors, err := storage.GetConnectorsByRole(ctx, orgID, storageaiconnectors.AIConnectorRoleLeader)
	if err != nil {
		return nil, fmt.Errorf("failed to get Leader AI connectors: %w", err)
	}

	leaderConfig, err := w.selectLeaderAIConfig(ctx, leaderConnectors, planCode)
	if err != nil {
		return nil, err
	}

	settingsStore := storageaiconnectors.NewReviewAISettingsStore(w.db)
	settings, err := settingsStore.GetByOrgID(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get review AI settings: %w", err)
	}

	selection := &diffReviewAISelection{
		Leader:        leaderConfig,
		HelperEnabled: settings.HelperEnabled,
		HelperMode:    settings.HelperMode,
	}

	if !settings.HelperEnabled {
		return selection, nil
	}

	helperConnectors, err := storage.GetConnectorsByRole(ctx, orgID, storageaiconnectors.AIConnectorRoleHelper)
	if err != nil {
		return nil, fmt.Errorf("failed to get Helper AI connectors: %w", err)
	}
	if len(helperConnectors) == 0 {
		// Adaptive Review is on but no helper connector is configured yet.
		// Degrade to leader-only instead of failing the review.
		log.Printf("[WARN] org %d: helper_enabled=true but no Helper AI connector configured; falling back to leader-only", orgID)
		selection.HelperEnabled = false
		return selection, nil
	}
	helperConfig, err := w.selectHelperAIConfig(ctx, helperConnectors)
	if err != nil {
		return nil, err
	}
	selection.Helper = &helperConfig

	return selection, nil
}

func (w *DiffReviewWorker) selectLeaderAIConfig(ctx context.Context, connectors []*aiconnectors.ConnectorRecord, planCode license.PlanType) (review.AIConfig, error) {
	if planCode == "" {
		planCode = license.PlanFree30K
	}

	if planCode == license.PlanFree30K {
		var byokConnector *aiconnectors.ConnectorRecord
		for _, c := range connectors {
			if c.ProviderName != aidefault.ProviderName {
				byokConnector = c
				break
			}
		}
		if byokConnector == nil {
			return review.AIConfig{}, fmt.Errorf("the Free plan requires you to configure your own LLM API key (BYOK) for your organization.")
		}
		return w.buildBYOKAIConfig(ctx, byokConnector, "byok_required")
	}

	if planCode == license.PlanTeam32USD {
		if len(connectors) > 0 {
			connector := connectors[0]
			if connector.ProviderName == aidefault.ProviderName {
				return buildDefaultAIConfig(ctx, w.db, connector)
			}
			return w.buildBYOKAIConfig(ctx, connector, "byok_override")
		}
		return w.buildHostedAutoAIConfig(ctx)
	}

	if len(connectors) > 0 {
		return w.buildBYOKAIConfig(ctx, connectors[0], "byok_optional")
	}
	return w.buildHostedAutoAIConfig(ctx)
}

func (w *DiffReviewWorker) selectHelperAIConfig(ctx context.Context, connectors []*aiconnectors.ConnectorRecord) (review.AIConfig, error) {
	if len(connectors) == 0 {
		// Defensive: callers should already have routed around this via the
		// empty-helperConnectors check in getReviewAISelectionFromDatabase.
		return review.AIConfig{}, fmt.Errorf("helper model is enabled but no Helper AI connector is configured")
	}
	connector := connectors[0]
	if connector.ProviderName == aidefault.ProviderName {
		return buildDefaultAIConfig(ctx, w.db, connector)
	}
	return w.buildBYOKAIConfig(ctx, connector, "helper_connector")
}

func buildDefaultAIConfig(ctx context.Context, db *sql.DB, record *aiconnectors.ConnectorRecord) (review.AIConfig, error) {
	tier := record.GetSelectedModel()
	if tier == "" {
		tier = "default"
	}
	options, err := aidefault.ResolveConnectorOptions(ctx, db, tier)
	if err != nil {
		return review.AIConfig{}, fmt.Errorf("failed to resolve managed AI options for tier %s: %w", tier, err)
	}

	configMap := map[string]interface{}{
		"provider_name":       record.ProviderName,
		"ai_provider_type":    string(options.Provider),
		"connector_name":      record.ConnectorName,
		"display_order":       record.DisplayOrder,
		"ai_execution_mode":   "managed_default",
		"ai_execution_source": "internal",
	}

	return review.AIConfig{
		Type:        "langchain",
		APIKey:      options.APIKey,
		Model:       options.ModelConfig.Model,
		Temperature: 0.4,
		Config:      configMap,
	}, nil
}

func (w *DiffReviewWorker) buildBYOKAIConfig(ctx context.Context, connector *aiconnectors.ConnectorRecord, executionMode string) (review.AIConfig, error) {
	if connector == nil {
		return review.AIConfig{}, fmt.Errorf("connector is required for BYOK mode")
	}

	var model string
	if connector.SelectedModel.Valid && connector.SelectedModel.String != "" {
		model = connector.SelectedModel.String
	} else {
		storage := aiconnectors.NewStorage(w.db)
		model = storage.GetDefaultModel(ctx, connector.Provider)
		if model == "" {
			return review.AIConfig{}, fmt.Errorf("no active default model configured in database for provider %s", connector.ProviderName)
		}
	}

	configMap := map[string]interface{}{
		"provider_name":       connector.ProviderName,
		"connector_name":      connector.ConnectorName,
		"display_order":       connector.DisplayOrder,
		"ai_execution_mode":   executionMode,
		"ai_execution_source": "connector",
	}

	if connector.GCPProjectID.Valid && connector.GCPProjectID.String != "" {
		configMap["gcp_project_id"] = connector.GCPProjectID.String
	}
	if connector.GCPLocation.Valid && connector.GCPLocation.String != "" {
		configMap["gcp_location"] = connector.GCPLocation.String
	}
	if connector.AWSAccessKeyID.Valid && connector.AWSAccessKeyID.String != "" {
		configMap["aws_access_key_id"] = connector.AWSAccessKeyID.String
	}
	if connector.AWSRegion.Valid && connector.AWSRegion.String != "" {
		configMap["aws_region"] = connector.AWSRegion.String
	}

	baseURL := ""
	if connector.BaseURL.Valid && connector.BaseURL.String != "" {
		baseURL = connector.BaseURL.String
	}
	baseURL = aiconnectors.ResolveBaseURLForProviderName(connector.ProviderName, baseURL)

	if baseURL != "" {
		configMap["base_url"] = baseURL
	}

	return review.AIConfig{
		Type:        "langchain",
		APIKey:      connector.ApiKey,
		Model:       model,
		Temperature: 0.4,
		Config:      configMap,
	}, nil
}

func (w *DiffReviewWorker) buildHostedAutoAIConfig(ctx context.Context) (review.AIConfig, error) {
	providerName := strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_AI_PROVIDER"))
	if providerName == "" {
		providerName = "gemini"
	}

	model := strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_AI_MODEL"))
	if model == "" {
		storage := aiconnectors.NewStorage(w.db)
		model = storage.GetDefaultModel(ctx, aiconnectors.Provider(providerName))
		if model == "" {
			return review.AIConfig{}, fmt.Errorf("no active default model configured in database for hosted provider %s", providerName)
		}
	}

	apiKey := ""
	switch providerName {
	case "gemini":
		apiKey = strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_GEMINI_API_KEY"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
		}
	case "openai":
		apiKey = strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_OPENAI_API_KEY"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		}
	case "deepseek":
		apiKey = strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_DEEPSEEK_API_KEY"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
		}
	case "openrouter":
		apiKey = strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_OPENROUTER_API_KEY"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
		}
	case "claude":
		apiKey = strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_CLAUDE_API_KEY"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
		}
	case "ollama":
		// Ollama does not require API key.
	default:
		return review.AIConfig{}, fmt.Errorf("unsupported hosted auto provider: %s", providerName)
	}

	if providerName != "ollama" && apiKey == "" {
		return review.AIConfig{}, fmt.Errorf("hosted auto provider '%s' is configured without API key; set LIVEREVIEW_HOSTED_*_API_KEY", providerName)
	}

	configMap := map[string]interface{}{
		"provider_name":       providerName,
		"connector_name":      "Hosted Auto",
		"display_order":       -1,
		"ai_execution_mode":   "hosted_auto",
		"ai_execution_source": "platform",
	}

	baseURL := aiconnectors.ResolveBaseURLForProviderName(providerName, strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_AI_BASE_URL")))
	if baseURL != "" {
		configMap["base_url"] = baseURL
	}

	return review.AIConfig{
		Type:        "langchain",
		APIKey:      apiKey,
		Model:       model,
		Temperature: 0.4,
		Config:      configMap,
	}, nil
}

func buildQueuedReviewAIMetadata(request *review.ReviewRequest, result *review.ReviewResult) map[string]interface{} {
	if request == nil || result == nil {
		return map[string]interface{}{}
	}

	meta := map[string]interface{}{
		"helper_enabled": request.HelperEnabled,
		"helper_mode":    strings.TrimSpace(request.HelperMode),
	}

	stages := make([]map[string]interface{}, 0, 2)
	if result.LeaderUsage != nil {
		stages = append(stages, queuedStageUsageToMetadata(result.LeaderUsage))
	}
	if result.HelperUsage != nil {
		stages = append(stages, queuedStageUsageToMetadata(result.HelperUsage))
	}
	if len(stages) > 0 {
		meta["stage_breakdown"] = stages
	}

	for k, v := range queuedAIExecutionMetadataForRole("leader", request.AI.Config) {
		meta[k] = v
	}
	if request.HelperAI != nil {
		for k, v := range queuedAIExecutionMetadataForRole("helper", request.HelperAI.Config) {
			meta[k] = v
		}
	}

	return meta
}

func queuedStageUsageToMetadata(usage *review.AIStageUsage) map[string]interface{} {
	meta := map[string]interface{}{
		"stage":           usage.Stage,
		"provider":        usage.Provider,
		"model":           usage.Model,
		"pricing_version": usage.PricingVersion,
	}
	if usage.InputTokens != nil {
		meta["input_tokens"] = *usage.InputTokens
	}
	if usage.OutputTokens != nil {
		meta["output_tokens"] = *usage.OutputTokens
	}
	if usage.CostUSD != nil {
		meta["cost_usd"] = *usage.CostUSD
	}
	return meta
}

func queuedAIExecutionMetadataForRole(role string, config map[string]interface{}) map[string]interface{} {
	meta := map[string]interface{}{}
	if len(config) == 0 {
		return meta
	}
	prefix := strings.TrimSpace(role)
	if prefix == "" {
		prefix = "ai"
	} else {
		prefix = prefix + "_ai"
	}
	if mode, ok := config["ai_execution_mode"].(string); ok && strings.TrimSpace(mode) != "" {
		meta[prefix+"_execution_mode"] = strings.TrimSpace(mode)
	}
	if source, ok := config["ai_execution_source"].(string); ok && strings.TrimSpace(source) != "" {
		meta[prefix+"_execution_source"] = strings.TrimSpace(source)
	}
	if provider, ok := config["provider_name"].(string); ok && strings.TrimSpace(provider) != "" {
		meta[prefix+"_provider_name"] = strings.TrimSpace(provider)
	}
	if connectorName, ok := config["connector_name"].(string); ok && strings.TrimSpace(connectorName) != "" {
		meta[prefix+"_connector_name"] = strings.TrimSpace(connectorName)
	}
	return meta
}

// --- Small utility helpers ---

func aiExecutionMetadataFromConfig(config map[string]interface{}) map[string]interface{} {
	meta := map[string]interface{}{}
	if len(config) == 0 {
		return meta
	}
	if mode, ok := config["ai_execution_mode"].(string); ok && strings.TrimSpace(mode) != "" {
		meta["ai_execution_mode"] = strings.TrimSpace(mode)
	}
	if source, ok := config["ai_execution_source"].(string); ok && strings.TrimSpace(source) != "" {
		meta["ai_execution_source"] = strings.TrimSpace(source)
	}
	if provider, ok := config["provider_name"].(string); ok && strings.TrimSpace(provider) != "" {
		meta["ai_provider_name"] = strings.TrimSpace(provider)
	}
	if connectorName, ok := config["connector_name"].(string); ok && strings.TrimSpace(connectorName) != "" {
		meta["ai_connector_name"] = strings.TrimSpace(connectorName)
	}
	return meta
}

func extractFirstHeading(markdown string) string {
	lines := strings.Split(markdown, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
	}
	return ""
}
