package mcpagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/vlrender"
	"github.com/rs/zerolog/log"
	"github.com/tmc/langchaingo/llms"
)

// classifyShape is call #0's three-way routing decision. See
// livi_analytics_plan.md's "Call #0" section - each value routes to a
// dedicated call with its own lean prompt, never a merged one.
type classifyShape string

const (
	shapeAction          classifyShape = "action"
	shapeAnalytics       classifyShape = "analytics"        // formerly "count_query" - renamed when multi-interpret replaced plan+finalize
	shapeProductGuidance classifyShape = "product_guidance" // covers how-to questions AND conversational turns (replaces old "chat" shape)
	shapeUnclassified    classifyShape = "unclassified"     // out-of-domain / general world knowledge questions
)

// classifyBoundHistory caps how many trailing history entries call #0 sees.
// Deliberately NOT the full, growing session history - if it were, call #0's
// own cost would creep up over a long conversation and quietly defeat the
// reason it exists. An approximation of "the last few exchanges" rather than
// an exact turn count, since history also carries tool-result entries that
// don't map 1:1 to conversational turns.
const classifyBoundHistory = 6

type SuggestedQuestionCategory struct {
	Category  string   `json:"category"`
	Questions []string `json:"questions"`
}

type classifyResult struct {
	Shape              classifyShape
	Message            string
	SuggestedQuestions []SuggestedQuestionCategory
}

// classify runs call #0. It never touches the growing session `history`
// directly - only a bounded, independently-built copy - and never sees the
// SQL schema or full tool definitions, which is the entire point of
// splitting it out from the calls that used to decide and act in one shot.
func (a *Agent) classify(ctx context.Context, history []HistoryEntry, userText string, clog *logging.ChatTurnLogger) (classifyResult, error) {
	classifyHistory := make([]HistoryEntry, 0, classifyBoundHistory+2)
	classifyHistory = append(classifyHistory, HistoryEntry{"role": "system", "content": a.classifyPrompt})
	classifyHistory = append(classifyHistory, boundedRecentHistory(history, classifyBoundHistory)...)
	classifyHistory = append(classifyHistory, HistoryEntry{"role": "user", "content": userText})

	// Two attempts: the model occasionally imitates a tool-call-shaped reply
	// here (most often when recent history shows it just made a real tool
	// call in an earlier turn), which isn't a valid {"response": ...}
	// shape. One corrective retry recovers most of these instead of
	// silently degrading the whole turn to chat.
	for attempt := 1; attempt <= 2; attempt++ {
		payload := ""
		if clog.Enabled() {
			if b, err := json.Marshal(classifyHistory); err == nil {
				payload = string(b)
			}
		}
		clog.LLMCallRequest(0, "classify", "", attempt, payload)

		start := time.Now()
		response, usage, err := a.provider.Complete(ctx, classifyHistory, nil, llms.WithJSONMode())
		elapsed := time.Since(start)
		if err != nil {
			clog.LLMCallError(0, "classify", "", attempt, elapsed, err)
			return classifyResult{}, fmt.Errorf("classify call: %w", err)
		}
		clog.LLMCallResponse(0, "classify", "", attempt, elapsed, usage.InputTokens, usage.OutputTokens, response)

		if res, ok := parseClassifyShape(response); ok {
			return res, nil
		}

		if attempt == 1 {
			log.Warn().Str("response_preview", truncateContent(response, 200)).Msg("classify call returned unparseable shape, retrying with correction")
			classifyHistory = append(classifyHistory,
				HistoryEntry{"role": "assistant", "content": response},
				HistoryEntry{"role": "user", "content": `That is not a valid classification reply. Reply with ONLY a JSON object of the exact shape {"response": "action" | "analytics" | "product_guidance" | "unclassified", "message": "...", "suggested_questions": [...], "applied_laws": [...]} - never a tool call, never prose, nothing else.`},
			)
		}
	}

	return classifyResult{}, fmt.Errorf("classify call returned an unparseable shape after retry")
}

// boundedRecentHistory returns the last n entries of history, skipping any
// system message (call #0 carries its own). This is an approximation of
// "recent conversational context", not an exact exchange count - see
// classifyBoundHistory.
func boundedRecentHistory(history []HistoryEntry, n int) []HistoryEntry {
	recent := make([]HistoryEntry, 0, n)
	for _, h := range history {
		if role, _ := h["role"].(string); role == "system" {
			continue
		}
		recent = append(recent, h)
	}
	if len(recent) > n {
		recent = recent[len(recent)-n:]
	}
	return recent
}

// classifyEnvelope is call #0's response shape. AppliedLaws is not
// validated or acted on by this code - it exists purely so the raw
// response text logged by LLMCallResponse shows which numbered laws the
// model believed it followed, for debugging (see chat_debug.log).
type classifyEnvelope struct {
	Response           string                      `json:"response"`
	Message            string                      `json:"message"`
	SuggestedQuestions []SuggestedQuestionCategory `json:"suggested_questions"`
	AppliedLaws        []string                    `json:"applied_laws"`
}

// parseClassifyShape decodes call #0's response, tolerating a fenced or
// bare JSON object the same way the rest of this protocol does (see
// parseAnalyticsPlan/parseFinalizePlan in analytics_types.go).
func parseClassifyShape(text string) (classifyResult, bool) {
	body := strings.TrimSpace(vlrender.ExtractJSONBlock(text))
	if body == "" {
		return classifyResult{}, false
	}
	var env classifyEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		return classifyResult{}, false
	}
	shape := classifyShape(strings.ToLower(strings.TrimSpace(env.Response)))
	switch shape {
	case shapeAction, shapeAnalytics, shapeProductGuidance, shapeUnclassified:
		return classifyResult{
			Shape:              shape,
			Message:            env.Message,
			SuggestedQuestions: env.SuggestedQuestions,
		}, true
	default:
		return classifyResult{}, false
	}
}
