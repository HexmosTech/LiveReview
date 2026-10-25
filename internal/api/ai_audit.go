package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/auditlog"
	"github.com/livereview/internal/mcpagent"
)

// recordChatAIUsage writes one metadata-only audit event per chat turn. Raw
// prompts are never stored: the fingerprint covers the user's message, and the
// event records which provider/model handled it plus token counts and latency.
// Best-effort: a failed audit write must not fail the chat turn.
func (s *Server) recordChatAIUsage(
	ctx context.Context,
	c echo.Context,
	orgID int64,
	connector *aiconnectors.Connector,
	purpose string,
	conversationID *int64,
	userMessage string,
	started time.Time,
	usage mcpagent.TokenUsage,
	calls int,
	runErr error,
) {
	if s == nil || s.auditRecorder == nil {
		return
	}
	provider, model := "", ""
	if connector != nil {
		provider = string(connector.GetProvider())
		model = connector.GetModel()
	}
	digest := sha256.Sum256([]byte(userMessage))
	outcome := auditlog.OutcomeSuccess
	details := auditlog.Fields{
		"llm_calls":    calls,
		"prompt_scope": "user_message",
	}
	if runErr != nil {
		outcome = auditlog.OutcomeFailure
		details["error_category"] = string(aiconnectors.CategorizeLLMError(runErr))
	}
	ev := auditlog.AIEvent{
		OrgID:          orgID,
		Purpose:        purpose,
		Provider:       provider,
		Model:          model,
		InputTokens:    int64(usage.InputTokens),
		OutputTokens:   int64(usage.OutputTokens),
		LatencyMS:      time.Since(started).Milliseconds(),
		PromptHash:     hex.EncodeToString(digest[:]),
		PromptBytes:    len(userMessage),
		ConversationID: conversationID,
		Outcome:        outcome,
		Details:        details,
	}
	if pc := auth.GetPermissionContext(c); pc != nil && pc.User != nil {
		uid := pc.User.ID
		ev.ActorUserID = &uid
		ev.ActorEmail = pc.User.Email
		if pc.IsSuperAdmin {
			ev.ActorKind = auditlog.ActorKindSuperAdmin
			ev.ActorCapacity = auditlog.ActorCapacityAdmin
		}
	}
	if err := s.auditRecorder.AIInvocation(ctx, ev); err != nil {
		auditlog.LogRecordError(auditlog.ActionAIInvocation, err)
	}
}
