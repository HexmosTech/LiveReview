package api

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/livereview/internal/api/auth"
	coreprocessor "github.com/livereview/internal/core_processor"
	"github.com/livereview/internal/license"
	storagelicense "github.com/livereview/storage/license"
)

// Phase 8: Webhook Orchestrator for coordinating provider and processing layers
// This orchestrator ties together the provider layer (Phases 1-6) with the unified processing core (Phase 7)

// WebhookOrchestratorV2 coordinates webhook processing across all providers and processing components
type WebhookOrchestratorV2 struct {
	server               *Server
	providerRegistry     *WebhookProviderRegistry
	unifiedProcessor     UnifiedProcessorV2
	contextBuilder       ContextBuilderV2
	learningProcessor    LearningProcessorV2
	processingTimeoutSec int
}

type timelineFetcher interface {
	FetchMRTimeline(coreprocessor.UnifiedMergeRequestV2) (*coreprocessor.UnifiedTimelineV2, error)
}

// NewWebhookOrchestratorV2 creates a new webhook orchestrator instance
func NewWebhookOrchestratorV2(server *Server) *WebhookOrchestratorV2 {
	log.Printf("[DEBUG] Creating webhook orchestrator V2...")

	if server == nil {
		log.Printf("[ERROR] Server is nil in NewWebhookOrchestratorV2")
		return nil
	}

	providerRegistry := NewWebhookProviderRegistry(server)
	if providerRegistry == nil {
		log.Printf("[ERROR] Failed to create provider registry")
		return nil
	}

	unifiedProcessor := NewUnifiedProcessorV2(server)
	if unifiedProcessor == nil {
		log.Printf("[ERROR] Failed to create unified processor")
		return nil
	}

	contextBuilder := coreprocessor.NewUnifiedContextBuilderV2()
	if contextBuilder == nil {
		log.Printf("[ERROR] Failed to create context builder")
		return nil
	}

	learningProcessor := NewLearningProcessorV2(server)
	if learningProcessor == nil {
		log.Printf("[ERROR] Failed to create learning processor")
		return nil
	}

	orchestrator := &WebhookOrchestratorV2{
		server:               server,
		providerRegistry:     providerRegistry,
		unifiedProcessor:     unifiedProcessor,
		contextBuilder:       contextBuilder,
		learningProcessor:    learningProcessor,
		processingTimeoutSec: 30, // 30 second timeout for AI processing
	}

	log.Printf("[INFO] Webhook orchestrator V2 initialized with %d providers",
		len(orchestrator.providerRegistry.providers))

	return orchestrator
}

// ProcessWebhookEvent is the main entry point for webhook processing (replaces individual handlers)
func (wo *WebhookOrchestratorV2) ProcessWebhookEvent(c echo.Context) error {
	startTime := time.Now()

	// Read headers for provider detection
	headers := make(map[string]string)
	for key, values := range c.Request().Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	// Read and buffer the body using Echo's body reader
	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		log.Printf("[ERROR] Failed to read webhook body: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Failed to read request body",
		})
	}

	log.Printf("[INFO] Processing webhook: %d bytes, headers: %v",
		len(bodyBytes), getRelevantHeaders(headers))

	// Phase 1: Provider Detection and Event Conversion
	if wo.providerRegistry == nil {
		log.Printf("[ERROR] Provider registry is nil")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Webhook orchestrator not properly initialized",
		})
	}

	providerName, provider := wo.providerRegistry.DetectProvider(headers, bodyBytes)
	if provider == nil {
		return wo.handleUnknownWebhook(c, headers)
	}

	log.Printf("[INFO] Detected provider: %s", providerName)

	// Phase 2: Convert to Unified Event Structure
	event, err := wo.convertToUnifiedEvent(provider, headers, bodyBytes)
	if err != nil {
		log.Printf("[ERROR] Failed to convert webhook to unified event: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error":    "Failed to process webhook",
			"provider": providerName,
		})
	}

	if event == nil {
		log.Printf("[DEBUG] Event filtered out during conversion")
		return c.JSON(http.StatusOK, map[string]string{
			"status":   "ignored",
			"provider": providerName,
			"reason":   "filtered_during_conversion",
		})
	}

	log.Printf("[INFO] Unified event created: type=%s, provider=%s", event.EventType, event.Provider)

	// Phase 3: Response Warrant Check
	botInfo, err := wo.getBotUserInfo(event)
	if err != nil {
		log.Printf("[WARN] Failed to get bot user info: %v", err)
		// Continue without bot info - some checks may not work but processing can continue
	}

	if wo.unifiedProcessor == nil {
		log.Printf("[ERROR] Unified processor is nil")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Unified processor not properly initialized",
		})
	}

	warrantsResponse, scenario := wo.unifiedProcessor.CheckResponseWarrant(*event, botInfo)
	if scenario.Type == "hard_failure" {
		log.Printf("[ERROR] Response warrant hard failure: %s", scenario.Reason)
		response := map[string]interface{}{
			"status":   "error",
			"provider": providerName,
			"reason":   scenario.Reason,
		}
		if len(scenario.Metadata) > 0 {
			response["details"] = scenario.Metadata
		}
		return c.JSON(http.StatusUnprocessableEntity, response)
	}
	if !warrantsResponse {
		log.Printf("[DEBUG] Event does not warrant response: %s", scenario.Type)
		return c.JSON(http.StatusOK, map[string]string{
			"status":   "ignored",
			"provider": providerName,
			"reason":   "no_response_warrant",
			"scenario": scenario.Type,
		})
	}

	log.Printf("[INFO] Response warranted: scenario=%s", scenario.Type)

	// Extract org ID and connector ID from context (set by BuildOrgContextFromConnector middleware)
	connectorID, ok := auth.GetConnectorIDFromContext(c)
	if !ok {
		log.Printf("[ERROR] Connector ID not found in context - webhook route configuration error")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "internal_error",
		})
	}
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		log.Printf("[ERROR] Org ID not found in context - middleware configuration error")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "internal_error",
		})
	}

	// Webhook signature validation (if provider supports it)
	type signatureValidator interface {
		ValidateWebhookSignature(connectorID int64, headers map[string]string, body []byte) bool
	}
	if validator, ok := provider.(signatureValidator); ok {
		if !validator.ValidateWebhookSignature(connectorID, headers, bodyBytes) {
			log.Printf("[ERROR] Webhook signature validation failed for connector_id=%d, provider=%s", connectorID, providerName)
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error":    "invalid_signature",
				"provider": providerName,
			})
		}
		log.Printf("[DEBUG] Webhook signature validated for connector_id=%d", connectorID)
	}

	log.Printf("[DEBUG] Processing webhook for connector_id=%d, org_id=%d", connectorID, orgID)
	if event.Repository.Metadata == nil {
		event.Repository.Metadata = map[string]interface{}{}
	}
	event.Repository.Metadata["connector_id"] = connectorID
	if event.MergeRequest != nil {
		if event.MergeRequest.Metadata == nil {
			event.MergeRequest.Metadata = map[string]interface{}{}
		}
		event.MergeRequest.Metadata["connector_id"] = connectorID
		if strings.TrimSpace(event.Repository.ID) != "" {
			event.MergeRequest.Metadata["repository_id"] = strings.TrimSpace(event.Repository.ID)
		}
	}

	// Phase 4: Asynchronous Processing (return response quickly)
	go wo.processEventAsync(context.Background(), event, provider, scenario, startTime, orgID)

	// Return success immediately - processing continues asynchronously
	return c.JSON(http.StatusOK, map[string]string{
		"status":        "accepted",
		"provider":      providerName,
		"event_type":    event.EventType,
		"event_ts":      event.Timestamp,
		"scenario":      scenario.Type,
		"processing":    "async",
		"response_time": fmt.Sprintf("%.2fms", float64(time.Since(startTime).Nanoseconds())/1e6),
	})
}

// processEventAsync handles the complete event processing pipeline asynchronously
func (wo *WebhookOrchestratorV2) processEventAsync(ctx context.Context, event *UnifiedWebhookEventV2, provider WebhookProviderV2, scenario ResponseScenarioV2, startTime time.Time, orgID int64) {
	processingCtx, cancel := context.WithTimeout(ctx, time.Duration(wo.processingTimeoutSec)*time.Second)
	defer cancel()

	log.Printf("[INFO] Starting async processing for event %s/%s", event.EventType, event.Provider)

	// Phase 5: Fetch Additional Context Data
	if err := provider.FetchMergeRequestData(event); err != nil {
		log.Printf("[WARN] Failed to fetch MR data, continuing with available data: %v", err)
	}

	if blocked, reason, locUsed, locLimit := wo.enforceWebhookPreflight(processingCtx, event, scenario.Type, orgID); blocked {
		log.Printf("[INFO] Webhook operation blocked by preflight checks for event %s/%s: %s", event.EventType, event.Provider, reason)
		wo.postQuotaExhaustedResponse(provider, event, locUsed, locLimit)
		return
	}

	// Phase 6: Build Timeline and Context
	var timeline *UnifiedTimelineV2
	var err error
	if event.MergeRequest != nil {
		if fetcher, ok := provider.(timelineFetcher); ok {
			timeline, err = fetcher.FetchMRTimeline(*event.MergeRequest)
			if err != nil {
				log.Printf("[ERROR] Provider timeline fetch failed: %v", err)
				wo.postErrorResponse(provider, event, "Failed to fetch merge request timeline")
				return
			}
			if timeline == nil {
				log.Printf("[ERROR] Provider timeline fetch returned nil timeline")
				wo.postErrorResponse(provider, event, "Merge request timeline unavailable")
				return
			}
		} else {
			timeline, err = wo.contextBuilder.BuildTimeline(*event.MergeRequest, event.Provider)
			if err != nil {
				log.Printf("[ERROR] Failed to build timeline: %v", err)
				wo.postErrorResponse(provider, event, "Failed to build context timeline")
				return
			}
		}
	} else {
		log.Printf("[WARN] No MR data available, creating empty timeline")
		timeline = &UnifiedTimelineV2{}
	}

	// Phase 7: Process Response Based on Scenario
	switch scenario.Type {
	case "comment_reply":
		wo.handleCommentReplyFlow(processingCtx, event, provider, timeline, orgID)
	case "full_review":
		wo.handleFullReviewFlow(processingCtx, event, provider, timeline, orgID)
	case "emoji_only":
		wo.handleEmojiOnlyFlow(processingCtx, event, provider)
	// Map the actual scenario types from unified processor to comment reply flow
	case "bot_reply", "reply_to_bot":
		log.Printf("[INFO] Bot reply scenario - handling as comment reply")
		wo.handleCommentReplyFlow(processingCtx, event, provider, timeline, orgID)
	case "direct_mention":
		log.Printf("[INFO] Direct mention scenario - handling as comment reply")
		wo.handleCommentReplyFlow(processingCtx, event, provider, timeline, orgID)
	case "discussion_reply":
		log.Printf("[INFO] Discussion reply scenario - handling as comment reply")
		wo.handleCommentReplyFlow(processingCtx, event, provider, timeline, orgID)
	case "content_trigger":
		log.Printf("[INFO] Content trigger scenario - handling as comment reply")
		wo.handleCommentReplyFlow(processingCtx, event, provider, timeline, orgID)
	default:
		log.Printf("[WARN] Unknown response scenario: %s", scenario.Type)
		wo.postErrorResponse(provider, event, "Unknown response scenario")
	}

	processingTime := time.Since(startTime)
	log.Printf("[INFO] Async processing completed for event %s/%s in %.2fs",
		event.EventType, event.Provider, processingTime.Seconds())
}

// handleCommentReplyFlow handles comment reply processing
func (wo *WebhookOrchestratorV2) handleCommentReplyFlow(ctx context.Context, event *UnifiedWebhookEventV2, provider WebhookProviderV2, timeline *UnifiedTimelineV2, orgID int64) {
	log.Printf("[INFO] Processing comment reply flow for event %s/%s", event.EventType, event.Provider)

	// Generate AI response
	response, learning, usage, err := wo.unifiedProcessor.ProcessCommentReply(ctx, *event, timeline, orgID)
	if err != nil {
		log.Printf("[ERROR] Failed to process comment reply: %v", err)
		wo.postErrorResponse(provider, event, "Failed to generate AI response")
		return
	}

	var learningAck string

	// Apply learning if extracted
	if learning != nil {
		// Ensure the learning is recorded under the org that owns this webhook context.
		// The unified processor may set an OrgID, but prefer the org from the incoming request
		// (X-Org-Context) which was propagated into this async pipeline.
		learning.OrgID = orgID
		log.Printf("[DEBUG] Applying learning with OrgID=%d (overriding if needed)", learning.OrgID)
		if err := wo.learningProcessor.ApplyLearning(learning); err != nil {
			log.Printf("[WARN] Failed to apply learning: %v", err)
		} else {
			learningAck = wo.learningProcessor.FormatLearningAcknowledgment(learning)
		}
	}

	if learningAck != "" {
		response = strings.TrimSpace(response) + "\n\n" + learningAck
	}

	if usage != nil && usage.Chargeable && usage.BillableLOC > 0 {
		blocked, reason, locUsed, locLimit := wo.enforceWebhookPreflightWithRequiredLOC(ctx, orgID, usage.BillableLOC, "webhook_comment_response")
		if blocked {
			log.Printf("[INFO] Webhook comment reply blocked by definitive preflight for event %s/%s: %s", event.EventType, event.Provider, reason)
			wo.postQuotaExhaustedResponse(provider, event, locUsed, locLimit)
			return
		}
	}

	// Post the response
	log.Printf("[DIAG] Calling provider.PostCommentReply with response_len=%d, event=%s/%s, comment_id=%s",
		len(response), event.EventType, event.Provider, event.Comment.ID)
	if err := provider.PostCommentReply(event, response); err != nil {
		log.Printf("[ERROR] Failed to post comment reply: %v", err)
		return
	}

	if usage != nil && usage.Chargeable && usage.BillableLOC > 0 {
		wo.accountWebhookSuccess(ctx, orgID, event, usage, "webhook_comment_response")
	}

	log.Printf("[INFO] Comment reply posted successfully for event %s/%s", event.EventType, event.Provider)
}

// handleFullReviewFlow handles full review processing
func (wo *WebhookOrchestratorV2) handleFullReviewFlow(ctx context.Context, event *UnifiedWebhookEventV2, provider WebhookProviderV2, timeline *UnifiedTimelineV2, orgID int64) {
	log.Printf("[INFO] Processing full review flow for event %s/%s", event.EventType, event.Provider)

	// Generate full review
	reviewComments, learning, usage, err := wo.unifiedProcessor.ProcessFullReview(ctx, *event, timeline)
	if err != nil {
		log.Printf("[ERROR] Failed to process full review: %v", err)
		wo.postErrorResponse(provider, event, "Failed to generate code review")
		return
	}

	var learningAck string

	// Apply learning if extracted
	if learning != nil {
		if err := wo.learningProcessor.ApplyLearning(learning); err != nil {
			log.Printf("[WARN] Failed to apply learning: %v", err)
		} else {
			learningAck = wo.learningProcessor.FormatLearningAcknowledgment(learning)
		}
	}

	// Convert review comments to overall comment
	overallComment := wo.formatReviewComments(reviewComments)
	if learningAck != "" {
		overallComment = strings.TrimSpace(overallComment) + "\n\n" + learningAck
	}

	// Post the full review
	if err := provider.PostFullReview(event, overallComment); err != nil {
		log.Printf("[ERROR] Failed to post full review: %v", err)
		return
	}

	if usage != nil && usage.Chargeable && usage.BillableLOC > 0 {
		wo.accountWebhookSuccess(ctx, orgID, event, usage, "webhook_full_review")
	}

	log.Printf("[INFO] Full review posted successfully for event %s/%s with %d comments",
		event.EventType, event.Provider, len(reviewComments))
}

// handleEmojiOnlyFlow handles emoji-only responses
func (wo *WebhookOrchestratorV2) handleEmojiOnlyFlow(ctx context.Context, event *UnifiedWebhookEventV2, provider WebhookProviderV2) {
	log.Printf("[INFO] Processing emoji-only flow for event %s/%s", event.EventType, event.Provider)

	// Choose appropriate emoji based on comment content
	emoji := wo.selectAppropriateEmoji(event.Comment.Body)

	// Post emoji reaction
	if err := provider.PostEmojiReaction(event, emoji); err != nil {
		log.Printf("[ERROR] Failed to post emoji reaction: %v", err)
		return
	}

	log.Printf("[INFO] Emoji reaction (%s) posted successfully for event %s/%s", emoji, event.EventType, event.Provider)
}

// Helper methods

// convertToUnifiedEvent attempts to convert webhook to unified event using different event types
func (wo *WebhookOrchestratorV2) convertToUnifiedEvent(provider WebhookProviderV2, headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	// Try comment event conversion first
	if event, err := provider.ConvertCommentEvent(headers, body); err == nil {
		return event, nil
	}

	// Try reviewer event conversion
	if event, err := provider.ConvertReviewerEvent(headers, body); err == nil {
		return event, nil
	}

	// If neither worked, return error
	return nil, fmt.Errorf("unable to convert webhook to any unified event type")
}

// getBotUserInfo retrieves bot user information for the event's provider
func (wo *WebhookOrchestratorV2) getBotUserInfo(event *UnifiedWebhookEventV2) (*UnifiedBotUserInfoV2, error) {
	if event == nil {
		return nil, fmt.Errorf("nil webhook event")
	}
	if wo.providerRegistry == nil {
		return nil, fmt.Errorf("provider registry not initialized")
	}

	type botInfoProvider interface {
		GetBotUserInfo(repository UnifiedRepositoryV2) (*UnifiedBotUserInfoV2, error)
	}

	switch event.Provider {
	case "gitlab":
		provider, ok := wo.providerRegistry.providers["gitlab"]
		if !ok {
			return nil, fmt.Errorf("gitlab provider not registered")
		}
		botProvider, ok := provider.(botInfoProvider)
		if !ok {
			return nil, fmt.Errorf("gitlab provider not configured")
		}
		return botProvider.GetBotUserInfo(event.Repository)

	case "github":
		provider, ok := wo.providerRegistry.providers["github"]
		if !ok {
			return nil, fmt.Errorf("github provider not registered")
		}
		botProvider, ok := provider.(botInfoProvider)
		if !ok {
			return nil, fmt.Errorf("github provider does not implement bot lookup")
		}
		return botProvider.GetBotUserInfo(event.Repository)

	case "bitbucket":
		provider, ok := wo.providerRegistry.providers["bitbucket"]
		if !ok {
			return nil, fmt.Errorf("bitbucket provider not registered")
		}
		botProvider, ok := provider.(botInfoProvider)
		if !ok {
			return nil, fmt.Errorf("bitbucket provider does not implement bot lookup")
		}
		return botProvider.GetBotUserInfo(event.Repository)

	case "gitea":
		provider, ok := wo.providerRegistry.providers["gitea"]
		if !ok {
			return nil, fmt.Errorf("gitea provider not registered")
		}
		botProvider, ok := provider.(botInfoProvider)
		if !ok {
			return nil, fmt.Errorf("gitea provider does not implement bot lookup")
		}
		return botProvider.GetBotUserInfo(event.Repository)

	default:
		return nil, fmt.Errorf("unknown provider: %s", event.Provider)
	}
}

// extractGitLabInstanceURL extracts base GitLab instance URL
func (wo *WebhookOrchestratorV2) extractGitLabInstanceURL(projectWebURL string) string {
	// This logic is extracted from the existing extractGitLabInstanceURL function
	// Implementation would be the same as in webhook_handler.go
	if projectWebURL == "" {
		return "https://gitlab.com"
	}

	// Find the project path separator
	if idx := strings.Index(projectWebURL, "/-/"); idx != -1 {
		return projectWebURL[:idx]
	}

	// For standard GitLab URLs like https://gitlab.com/group/project
	parts := strings.Split(projectWebURL, "/")
	if len(parts) >= 3 {
		return fmt.Sprintf("%s//%s", parts[0], parts[2])
	}

	return "https://gitlab.com"
}

// selectAppropriateEmoji selects emoji based on comment content
func (wo *WebhookOrchestratorV2) selectAppropriateEmoji(commentBody string) string {
	commentLower := strings.ToLower(commentBody)

	// Thanks/appreciation
	if strings.Contains(commentLower, "thank") || strings.Contains(commentLower, "appreciate") {
		return "heart"
	}

	// Questions
	if strings.Contains(commentLower, "?") || strings.Contains(commentLower, "how") ||
		strings.Contains(commentLower, "why") || strings.Contains(commentLower, "what") {
		return "point_up"
	}

	// Positive feedback
	if strings.Contains(commentLower, "good") || strings.Contains(commentLower, "great") ||
		strings.Contains(commentLower, "nice") || strings.Contains(commentLower, "excellent") {
		return "thumbsup"
	}

	// Issues/problems
	if strings.Contains(commentLower, "issue") || strings.Contains(commentLower, "problem") ||
		strings.Contains(commentLower, "bug") || strings.Contains(commentLower, "error") {
		return "eyes"
	}

	// Default
	return "thumbsup"
}

func (wo *WebhookOrchestratorV2) enforceWebhookPreflight(ctx context.Context, event *UnifiedWebhookEventV2, scenarioType string, orgID int64) (bool, string, int64, int64) {
	if wo == nil || wo.server == nil || wo.server.db == nil || event == nil || orgID <= 0 {
		return false, "", 0, 0
	}

	requiredLOC, ok := estimateWebhookRequiredLOC(event)
	if !ok || requiredLOC <= 0 {
		return false, "", 0, 0
	}

	operationType, ok := webhookOperationTypeFromScenario(scenarioType)
	if !ok {
		return false, "", 0, 0
	}

	return wo.enforceWebhookPreflightWithRequiredLOC(ctx, orgID, requiredLOC, operationType)
}

func (wo *WebhookOrchestratorV2) enforceWebhookPreflightWithRequiredLOC(ctx context.Context, orgID int64, requiredLOC int64, operationType string) (bool, string, int64, int64) {
	if wo == nil || wo.server == nil || wo.server.db == nil || orgID <= 0 || requiredLOC <= 0 || strings.TrimSpace(operationType) == "" {
		return false, "", 0, 0
	}

	quotaModule := license.NewQuotaModule(wo.server.db)
	planCode, err := wo.resolveOrgPlanCode(ctx, orgID)
	if err != nil {
		log.Printf("[ERROR] LOC preflight aborted for org=%d operation=%s: %v", orgID, operationType, err)
		return true, "plan_resolution_error", 0, 0
	}
	result, err := quotaModule.PreflightCheck(ctx, license.QuotaPreflightInput{
		OrgID:       orgID,
		RequiredLOC: requiredLOC,
		PlanCode:    planCode,
	})
	if err != nil {
		log.Printf("[WARN] LOC preflight check failed for org=%d operation=%s required_loc=%d: %v", orgID, operationType, requiredLOC, err)
		return false, "", 0, 0
	}
	if !result.Blocked {
		return false, "", 0, 0
	}

	return true, result.BlockReason, result.LOCUsedMonth, result.LOCLimitMonth
}

func (wo *WebhookOrchestratorV2) accountWebhookSuccess(ctx context.Context, orgID int64, event *UnifiedWebhookEventV2, usage *OperationUsageV2, operationType string) {
	if wo == nil || wo.server == nil || wo.server.db == nil || event == nil || usage == nil || orgID <= 0 || !usage.Chargeable || usage.BillableLOC <= 0 {
		return
	}

	quotaModule := license.NewQuotaModule(wo.server.db)
	planCode, err := wo.resolveOrgPlanCode(ctx, orgID)
	if err != nil {
		log.Printf("[ERROR] skipping webhook accounting for org=%d operation=%s due plan resolution failure: %v", orgID, operationType, err)
		return
	}

	operationID := buildWebhookOperationKey(event, operationType)
	actorUserID, actorEmail := wo.resolveWebhookActor(ctx, orgID, event)
	_, err = quotaModule.RecordBatch(ctx, license.QuotaRecordBatchInput{
		OrgID:          orgID,
		ReviewID:       nil,
		OperationType:  operationType,
		TriggerSource:  "webhook",
		OperationID:    operationID,
		IdempotencyKey: operationID,
		BatchIndex:     1,
		Batch: license.QuotaBatchInput{
			PlanCode:                 planCode,
			Provider:                 strings.TrimSpace(usage.Provider),
			RawLOCBatch:              usage.BillableLOC,
			ProviderTotalInputTokens: usage.InputTokens,
			OutputTokensBatch:        usage.OutputTokens,
		},
	})
	if err != nil {
		log.Printf("[WARN] failed to record webhook quota batch for org=%d operation=%s: %v", orgID, operationType, err)
		return
	}

	_, err = quotaModule.FinalizeOperation(ctx, license.QuotaFinalizeInput{
		OrgID:          orgID,
		ReviewID:       nil,
		ActorUserID:    actorUserID,
		ActorEmail:     actorEmail,
		OperationType:  operationType,
		TriggerSource:  "webhook",
		OperationID:    operationID,
		IdempotencyKey: operationID,
		Provider:       strings.TrimSpace(usage.Provider),
		Model:          strings.TrimSpace(usage.Model),
		BatchFallback:  nil,
	})
	if err != nil {
		log.Printf("[WARN] failed to account webhook usage for org=%d operation=%s: %v", orgID, operationType, err)
	}
}

func (wo *WebhookOrchestratorV2) resolveOrgPlanCode(ctx context.Context, orgID int64) (license.PlanType, error) {
	if wo == nil || wo.server == nil || wo.server.db == nil || orgID <= 0 {
		return "", fmt.Errorf("plan resolution requires valid webhook orchestrator context")
	}

	store := storagelicense.NewPlanChangeStore(wo.server.db)
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

func (wo *WebhookOrchestratorV2) resolveWebhookActor(ctx context.Context, orgID int64, event *UnifiedWebhookEventV2) (*int64, string) {
	actorEmail := webhookActorEmail(event)
	if actorEmail == "" || wo == nil || wo.server == nil || wo.server.db == nil || orgID <= 0 {
		return nil, actorEmail
	}

	lookupStore := storagelicense.NewActorLookupStore(wo.server.db)
	userID, err := lookupStore.ResolveOrgMemberUserIDByEmail(ctx, orgID, actorEmail)
	if err != nil {
		log.Printf("[WARN] failed to resolve webhook actor for org=%d email=%s: %v", orgID, actorEmail, err)
		return nil, actorEmail
	}

	return userID, actorEmail
}

func webhookActorEmail(event *UnifiedWebhookEventV2) string {
	if event == nil {
		return ""
	}
	if email := strings.TrimSpace(event.Actor.Email); email != "" {
		return email
	}
	if event.Comment != nil {
		if email := strings.TrimSpace(event.Comment.Author.Email); email != "" {
			return email
		}
	}
	if event.MergeRequest != nil {
		if email := strings.TrimSpace(event.MergeRequest.Author.Email); email != "" {
			return email
		}
	}
	return ""
}

func extractWebhookReviewID(event *UnifiedWebhookEventV2) int64 {
	if event == nil {
		return 0
	}

	if event.Comment != nil {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(event.Comment.ID), 10, 64); err == nil && parsed > 0 {
			return parsed
		}
	}

	if event.MergeRequest != nil {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(event.MergeRequest.ID), 10, 64); err == nil && parsed > 0 {
			return parsed
		}
		if event.MergeRequest.Number > 0 {
			return int64(event.MergeRequest.Number)
		}
	}

	return 0
}

func buildWebhookOperationKey(event *UnifiedWebhookEventV2, operationType string) string {
	if event == nil {
		return ""
	}

	repoID := event.Repository.FullName
	if repoID == "" {
		repoID = event.Repository.Name
	}

	mergeRequestID := ""
	if event.MergeRequest != nil {
		if event.MergeRequest.ID != "" {
			mergeRequestID = event.MergeRequest.ID
		} else if event.MergeRequest.Number > 0 {
			mergeRequestID = strconv.Itoa(event.MergeRequest.Number)
		}
	}

	commentID := ""
	if event.Comment != nil {
		commentID = event.Comment.ID
	}

	if mergeRequestID != "" {
		return fmt.Sprintf("webhook:%s:%s:%s:%s:%s", event.Provider, operationType, repoID, mergeRequestID, commentID)
	}

	return fmt.Sprintf("webhook:%s:%s:%s:%s", event.Provider, operationType, repoID, event.Timestamp)
}

func webhookOperationTypeFromScenario(scenarioType string) (string, bool) {
	switch scenarioType {
	case "comment_reply", "bot_reply", "reply_to_bot", "direct_mention", "discussion_reply", "content_trigger":
		return "webhook_comment_response", true
	case "full_review":
		return "webhook_full_review", true
	default:
		return "", false
	}
}

func estimateWebhookRequiredLOC(event *UnifiedWebhookEventV2) (int64, bool) {
	if event == nil || event.MergeRequest == nil || event.MergeRequest.Metadata == nil {
		return 0, false
	}
	v, ok := event.MergeRequest.Metadata["operation_billable_loc"]
	if !ok {
		return 0, false
	}

	switch typed := v.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case int32:
		return int64(typed), true
	case float64:
		return int64(typed), true
	case float32:
		return int64(typed), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

// formatReviewComments formats review comments into a single overall comment
func (wo *WebhookOrchestratorV2) formatReviewComments(comments []UnifiedReviewCommentV2) string {
	if len(comments) == 0 {
		return "✅ Code review completed - no issues found."
	}

	result := fmt.Sprintf("🔍 **Code Review Summary** (%d comments)\n\n", len(comments))

	for i, comment := range comments {
		result += fmt.Sprintf("**%d. %s**", i+1, comment.FilePath)
		if comment.Severity != "" {
			result += fmt.Sprintf(" (%s)", comment.Severity)
		}
		result += "\n"
		if comment.LineNumber > 0 {
			result += fmt.Sprintf("   Line %d: ", comment.LineNumber)
		}
		result += comment.Content + "\n\n"
	}

	result += "---\n*Generated by LiveReview AI*"
	return result
}

// postErrorResponse posts a standardized error response
func (wo *WebhookOrchestratorV2) postErrorResponse(provider WebhookProviderV2, event *UnifiedWebhookEventV2, errorMsg string) {
	errorResponse := fmt.Sprintf("⚠️ %s\n\n*This issue has been logged and will be investigated.*", errorMsg)

	if err := provider.PostCommentReply(event, errorResponse); err != nil {
		log.Printf("[ERROR] Failed to post error response: %v", err)
	}
}

// postQuotaExhaustedResponse posts a user-friendly LOC quota exhaustion message
// back to the PR as a comment reply, so the user knows why the bot didn't respond.
func (wo *WebhookOrchestratorV2) postQuotaExhaustedResponse(provider WebhookProviderV2, event *UnifiedWebhookEventV2, locUsed int64, locLimit int64) {
	if provider == nil || event == nil {
		return
	}

	upgradeURL := "/settings-subscriptions-overview"
	if wo.server != nil {
		if prodURL, err := wo.server.GetProductionURLDirectly(); err == nil && strings.TrimSpace(prodURL) != "" {
			upgradeURL = strings.TrimRight(prodURL, "/") + upgradeURL
		}
	}

	// Build usage detail line if quota data is available
	usageLine := ""
	if locLimit > 0 {
		usageLine = fmt.Sprintf(
			"Your team has used all %s allocated lines of code for this month. ",
			formatNumber(locLimit),
		)
	} else {
		usageLine = "Your team has used all allocated lines of code for this month. "
	}

	quotaMessage := fmt.Sprintf(
		"⚠️ **Monthly LOC Limit Reached**\n\n"+
			"%s"+
			"Upgrade to a higher plan to continue reviewing code without any interruption to your workflow.\n\n"+
			"👉 [Upgrade Plan](%s)\n",
		usageLine, upgradeURL,
	)

	if err := provider.PostCommentReply(event, quotaMessage); err != nil {
		log.Printf("[ERROR] Failed to post quota exhausted response: %v", err)
	}
}

// formatNumber formats an int64 with comma separators (e.g. 100000 -> "100,000")
func formatNumber(n int64) string {
	if n < 0 {
		return "-" + formatNumber(-n)
	}
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

// handleUnknownWebhook handles webhooks that couldn't be routed to any provider
func (wo *WebhookOrchestratorV2) handleUnknownWebhook(c echo.Context, headers map[string]string) error {
	log.Printf("[WARN] Unknown webhook provider, headers: %v", getRelevantHeaders(headers))

	return c.JSON(http.StatusBadRequest, map[string]string{
		"error":   "Unknown webhook provider",
		"headers": fmt.Sprintf("%v", getRelevantHeaders(headers)),
	})
}

// GetProcessingStats returns processing statistics
func (wo *WebhookOrchestratorV2) GetProcessingStats() map[string]interface{} {
	return map[string]interface{}{
		"providers_registered":   len(wo.providerRegistry.providers),
		"provider_names":         wo.providerRegistry.getProviderNames(),
		"processing_timeout_sec": wo.processingTimeoutSec,
		"components": map[string]bool{
			"unified_processor":  wo.unifiedProcessor != nil,
			"context_builder":    wo.contextBuilder != nil,
			"learning_processor": wo.learningProcessor != nil,
			"provider_registry":  wo.providerRegistry != nil,
		},
	}
}

// UpdateProcessingTimeout updates the processing timeout
func (wo *WebhookOrchestratorV2) UpdateProcessingTimeout(timeoutSec int) {
	wo.processingTimeoutSec = timeoutSec
	log.Printf("[INFO] Processing timeout updated to %d seconds", timeoutSec)
}
