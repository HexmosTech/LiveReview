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
	"github.com/livereview/internal/review"
	reviewprocessor "github.com/livereview/internal/review_processor"
	"github.com/livereview/pkg/models"
	"github.com/riverqueue/river"
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
	return reviewprocessor.ProcessManualReview(ctx, w.jq.db, args.OrgID, args.PlanCode, args.ActorUserID, args.ActorEmail, args.ReviewID, args.RequestJSON)
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
	localDiffs, err := diffutil.ParseDiffZipBase64(args.DiffZipBase64)
	if err != nil {
		w.handleFailure(ctx, args, logger, eventSink, fmt.Sprintf("failed to parse diff: %v", err), "failed_to_parse_zip")
		return nil // Return nil so River marks job succeeded; business-level failure already handled.
	}

	// 3. Calculate Lines of Code
	billableLOC := diffutil.CalculateEffectiveDiffLOCFromLocalDiffs(localDiffs)

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

	// 5. Convert diffs and persist preloaded_changes for UI polling
	modelDiffs := diffutil.ConvertLocalDiffs(localDiffs)
	rm := reviewprocessor.NewReviewManager(w.db)
	if err := rm.MergeReviewMetadata(args.ReviewID, map[string]interface{}{
		"preloaded_changes":      modelDiffs,
		"operation_billable_loc": billableLOC,
	}); err != nil {
		log.Printf("[WARN] failed to store preloaded_changes for review %d: %v", args.ReviewID, err)
	}

	// 6. Transition status to in_progress
	_ = rm.UpdateReviewStatus(args.ReviewID, "in_progress")

	// 7. Load AI Configuration
	aiConfig, err := w.getAIConfigFromDatabase(ctx, args.OrgID, planCode)
	if err != nil {
		w.handleFailure(ctx, args, logger, eventSink, fmt.Sprintf("failed to load AI config: %v", err), "failed_to_load_ai_config")
		return nil
	}

	reviewRequest := review.ReviewRequest{
		URL:              fmt.Sprintf("cli-diff:%s", args.RepoName),
		ReviewID:         fmt.Sprintf("%d", args.ReviewID),
		Provider:         review.ProviderConfig{Type: "cli", URL: "", Token: "", Config: map[string]interface{}{}},
		AI:               aiConfig,
		PreloadedChanges: modelDiffs,
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

	status := "failed"
	summary := ""
	var comments []*models.ReviewComment
	failureReason := ""

	if result != nil {
		if result.Success {
			status = "completed"
			accountedAt := time.Now().UTC().Format(time.RFC3339)
			resolvedReviewID := args.ReviewID
			operationID := fmt.Sprintf("diff-review:%d", args.ReviewID)
			idempotencyKey := operationID
			var actorUserIDPtr *int64
			if args.ActorUserID > 0 {
				resolvedActorUserID := args.ActorUserID
				actorUserIDPtr = &resolvedActorUserID
			}

			// 9. Quota Accounting (Billing)
			_, err = quotaModule.RecordBatch(ctx, license.QuotaRecordBatchInput{
				OrgID:          args.OrgID,
				ReviewID:       &resolvedReviewID,
				OperationType:  "diff_review",
				TriggerSource:  args.TriggerSource,
				OperationID:    operationID,
				IdempotencyKey: idempotencyKey,
				BatchIndex:     1,
				Batch: license.QuotaBatchInput{
					PlanCode:                 planCode,
					Provider:                 result.Provider,
					RawLOCBatch:              billableLOC,
					ProviderTotalInputTokens: result.InputTokens,
					OutputTokensBatch:        result.OutputTokens,
				},
			})
			if err != nil {
				log.Printf("[WARN] failed batch accounting for review %d: %v", args.ReviewID, err)
			} else {
				finalized, err := quotaModule.FinalizeOperation(ctx, license.QuotaFinalizeInput{
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
					BatchFallback:  nil,
				})
				if err != nil {
					log.Printf("[WARN] failed final accounting for review %d: %v", args.ReviewID, err)
				} else {
					meta := map[string]interface{}{
						"accounted_at":           accountedAt,
						"operation_id":           operationID,
						"idempotency_key":        idempotencyKey,
						"operation_raw_loc":      finalized.RawLOCTotal,
						"operation_billable_loc": finalized.EffectiveLOCTotal,
						"operation_extra_loc":    finalized.ExtraEffectiveLOCTotal,
						"context_tokens":         finalized.ContextTokensTotal,
						"allowed_context_tokens": finalized.AllowedContextTokensTotal,
						"extra_context_tokens":   finalized.ExtraContextTokensTotal,
						"input_cost_usd":         finalized.InputCostUSDTotal,
						"output_cost_usd":        finalized.OutputCostUSDTotal,
						"total_cost_usd":         finalized.TotalCostUSDTotal,
						"pricing_version":        finalized.PricingVersion,
					}
					for k, v := range aiExecutionMetadataFromConfig(reviewRequest.AI.Config) {
						meta[k] = v
					}
					if err := rm.MergeReviewMetadata(args.ReviewID, meta); err != nil {
						log.Printf("[WARN] failed to store accounted_at for review %d: %v", args.ReviewID, err)
					}
				}
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

func (w *DiffReviewWorker) getAIConfigFromDatabase(ctx context.Context, orgID int64, planCode license.PlanType) (review.AIConfig, error) {
	storage := aiconnectors.NewStorage(w.db)

	connectors, err := storage.GetAllConnectors(ctx, orgID)
	if err != nil {
		return review.AIConfig{}, fmt.Errorf("failed to get AI connectors: %w", err)
	}

	if planCode == "" {
		planCode = license.PlanFree30K
	}

	// Free tier enforces BYOK strictly
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

	// Paid team defaults to hosted auto model when no BYOK connector is configured.
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

	// Fallback for other plans: prefer BYOK if present, else hosted-auto.
	if len(connectors) > 0 {
		return w.buildBYOKAIConfig(ctx, connectors[0], "byok_optional")
	}
	return w.buildHostedAutoAIConfig(ctx)
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


