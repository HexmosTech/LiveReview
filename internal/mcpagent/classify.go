package mcpagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/vlrender"
)

// classifyShape is call #0's three-way routing decision. See
// livi_analytics_plan.md's "Call #0" section - each value routes to a
// dedicated call with its own lean prompt, never a merged one.
type classifyShape string

const (
	shapeAction     classifyShape = "action"
	shapeCountQuery classifyShape = "count_query"
	shapeChat       classifyShape = "chat"
)

// classifyBoundHistory caps how many trailing history entries call #0 sees.
// Deliberately NOT the full, growing session history - if it were, call #0's
// own cost would creep up over a long conversation and quietly defeat the
// reason it exists. An approximation of "the last few exchanges" rather than
// an exact turn count, since history also carries tool-result entries that
// don't map 1:1 to conversational turns.
const classifyBoundHistory = 6

// classify runs call #0. It never touches the growing session `history`
// directly - only a bounded, independently-built copy - and never sees the
// SQL schema or full tool definitions, which is the entire point of
// splitting it out from the calls that used to decide and act in one shot.
func (a *Agent) classify(ctx context.Context, history []HistoryEntry, userText string, clog *logging.ChatTurnLogger) (classifyShape, error) {
	classifyHistory := make([]HistoryEntry, 0, classifyBoundHistory+2)
	classifyHistory = append(classifyHistory, HistoryEntry{"role": "system", "content": a.classifyPrompt})
	classifyHistory = append(classifyHistory, boundedRecentHistory(history, classifyBoundHistory)...)
	classifyHistory = append(classifyHistory, HistoryEntry{"role": "user", "content": userText})

	payload := ""
	if clog.Enabled() {
		if b, err := json.Marshal(classifyHistory); err == nil {
			payload = string(b)
		}
	}
	clog.LLMCallRequest(0, "classify", "", 1, payload)

	start := time.Now()
	response, usage, err := a.provider.Complete(ctx, classifyHistory, nil)
	elapsed := time.Since(start)
	if err != nil {
		clog.LLMCallError(0, "classify", "", 1, elapsed, err)
		return "", fmt.Errorf("classify call: %w", err)
	}
	clog.LLMCallResponse(0, "classify", "", 1, elapsed, usage.InputTokens, usage.OutputTokens, response)

	shape, ok := parseClassifyShape(response)
	if !ok {
		return "", fmt.Errorf("classify call returned an unparseable shape: %q", truncateContent(response, 200))
	}
	return shape, nil
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

type classifyEnvelope struct {
	Shape string `json:"shape"`
}

// parseClassifyShape decodes call #0's response, tolerating a fenced or
// bare JSON object the same way the rest of this protocol does (see
// parseAnalyticsPlan/parseFinalizePlan in analytics_types.go).
func parseClassifyShape(text string) (classifyShape, bool) {
	body := strings.TrimSpace(vlrender.ExtractJSONBlock(text))
	if body == "" {
		return "", false
	}
	var env classifyEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		return "", false
	}
	switch classifyShape(strings.ToLower(strings.TrimSpace(env.Shape))) {
	case shapeAction:
		return shapeAction, true
	case shapeCountQuery:
		return shapeCountQuery, true
	case shapeChat:
		return shapeChat, true
	default:
		return "", false
	}
}
