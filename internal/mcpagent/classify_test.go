package mcpagent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	storageanalytics "github.com/livereview/storage/analytics"
	"github.com/tmc/langchaingo/llms"
)

func TestParseClassifyShape(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want classifyShape
		ok   bool
	}{
		{"bare", `{"shape":"action"}`, shapeAction, true},
		{"fenced", "```json\n{\"shape\": \"count_query\"}\n```", shapeCountQuery, true},
		{"chat", `{"shape":"chat"}`, shapeChat, true},
		{"case insensitive", `{"shape":"ACTION"}`, shapeAction, true},
		{"whitespace", `  {"shape":"chat"}  `, shapeChat, true},
		{"unknown shape", `{"shape":"delete_everything"}`, "", false},
		{"not json", `sure, I can help with that`, "", false},
		{"empty", ``, "", false},
		{"missing field", `{}`, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseClassifyShape(c.in)
			if ok != c.ok || got != c.want {
				t.Errorf("parseClassifyShape(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestBoundedRecentHistory(t *testing.T) {
	history := []HistoryEntry{
		{"role": "system", "content": "sys"}, // must be dropped regardless of position
		{"role": "user", "content": "1"},
		{"role": "assistant", "text": "2"},
		{"role": "user", "content": "3"},
		{"role": "assistant", "text": "4"},
		{"role": "user", "content": "5"},
	}
	got := boundedRecentHistory(history, 3)
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(got), got)
	}
	for _, h := range got {
		if h["role"] == "system" {
			t.Fatalf("system message leaked into bounded history: %v", got)
		}
	}
	if got[2]["content"] != "5" {
		t.Fatalf("expected the most recent entry last, got %v", got)
	}

	// Fewer entries than n: returns everything (minus system), no panic.
	short := boundedRecentHistory(history[:2], 10)
	if len(short) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(short), short)
	}
}

// recordingProvider is like scriptedProvider in analytics_pipeline_test.go
// but keeps every call's full history and tool list, not just the last
// message - the call #0/dispatch split has to be verified against WHICH
// system prompt (history[0]) and WHICH tools a given call actually received,
// not just its trailing user message.
type recordingProvider struct {
	replies      []string
	calls        int
	histories    [][]HistoryEntry
	toolsPerCall [][]llms.Tool
}

func (p *recordingProvider) Complete(_ context.Context, history []HistoryEntry, tools []llms.Tool) (string, TokenUsage, error) {
	p.histories = append(p.histories, append([]HistoryEntry(nil), history...))
	p.toolsPerCall = append(p.toolsPerCall, tools)
	if p.calls >= len(p.replies) {
		return "", TokenUsage{}, fmt.Errorf("recordingProvider exhausted after %d calls", p.calls)
	}
	reply := p.replies[p.calls]
	p.calls++
	return reply, TokenUsage{}, nil
}

func (p *recordingProvider) Describe() string { return "recording/test" }

func (p *recordingProvider) FormatTools(tools []MCPToolDef) []llms.Tool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]llms.Tool, len(tools))
	for i, t := range tools {
		out[i] = llms.Tool{Type: "function", Function: &llms.FunctionDefinition{Name: t.Name, Description: t.Description}}
	}
	return out
}

type nopAnalyticsEngine struct{}

func (nopAnalyticsEngine) Count(context.Context, int64, string) (int64, error) { return 0, nil }
func (nopAnalyticsEngine) Query(context.Context, int64, string, int) (*storageanalytics.ResultSet, error) {
	return nil, nil
}

// dispatchTestAgent builds an Agent with analytics enabled but no real
// database/tool-server behind it - every case below is designed to be
// resolved by the guard's own validation (rejecting a table that isn't in
// the catalog) or to never call a tool, so nothing here needs live
// infrastructure.
func dispatchTestAgent(prov *recordingProvider) *Agent {
	session := &MCPSession{
		OrgID:    1,
		OrgName:  "TestOrg",
		UserRole: "member",
		Tools: []MCPToolDef{
			{Name: "POST_api_v1_connectors_trigger_review", Description: "Trigger a review"},
			{Name: "GET_api_v1_reviews", Description: "List reviews"},
		},
	}
	agent := &Agent{provider: prov, mcpSession: session, maxSteps: 5}
	agent.WithAnalytics(nopAnalyticsEngine{})
	return agent
}

// TestDispatchSwapsSystemPromptPerTurn is the regression test for the gap
// this whole call #0 split exists to close (see livi_analytics_plan.md's
// "Implementation: per-turn system prompt selection"): a session that
// changes shape turn-to-turn must get the matching branch prompt EVERY
// turn, not just on the first one. Three turns, three different shapes, one
// shared Agent/history - if history[0] were only ever set once (the bug
// this replaced), turns 2 and 3 would still see turn 1's prompt.
func TestDispatchSwapsSystemPromptPerTurn(t *testing.T) {
	prov := &recordingProvider{replies: []string{
		// Turn 1: chat
		`{"shape":"chat"}`,
		"Hi! I'm Livi, how can I help you?",
		// Turn 2: action (no tool call in the reply - just confirms the
		// prompt/tools selection, not the tool-calling loop itself, which
		// is unchanged and already covered elsewhere)
		`{"shape":"action"}`,
		"Which repository would you like me to review?",
		// Turn 3: count_query - a table outside the catalog so the guard
		// rejects it deterministically, without touching a database.
		`{"shape":"count_query"}`,
		`{"analytics_plan":[{"id":"r1","question":"bad","count_sql":"SELECT count(*) FROM secret_table"}]}`,
		"SELECT count(*) AS n FROM public.reviews", // repair attempt, also rejected
	}}
	agent := dispatchTestAgent(prov)

	var history []HistoryEntry

	// --- Turn 1: chat ---
	text, updated, _, err := agent.RunTurnWithArtifacts(context.Background(), history, "hello", "s1", "test")
	if err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if text != "Hi! I'm Livi, how can I help you?" {
		t.Fatalf("turn 1: unexpected response %q", text)
	}
	history = updated
	sys1, _ := history[0]["content"].(string)
	if !strings.Contains(sys1, "friendly persona") {
		t.Fatalf("turn 1: expected the persona header in history[0], got: %q", sys1)
	}
	if strings.Contains(sys1, "Answering data questions with SQL") {
		t.Fatalf("turn 1 (chat): system prompt leaked the SQL schema section: %q", sys1)
	}
	if strings.Contains(sys1, "You have access to the following tools") {
		t.Fatalf("turn 1 (chat): system prompt leaked the tool list: %q", sys1)
	}
	if len(prov.toolsPerCall[0]) != 0 {
		t.Fatalf("turn 1 (classify call): expected no tool schemas, got %d", len(prov.toolsPerCall[0]))
	}
	if len(prov.toolsPerCall[1]) != 0 {
		t.Fatalf("turn 1 (chat call): expected no tools offered, got %d", len(prov.toolsPerCall[1]))
	}

	// --- Turn 2: action ---
	text, updated, _, err = agent.RunTurnWithArtifacts(context.Background(), history, "trigger a review", "s1", "test")
	if err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if text != "Which repository would you like me to review?" {
		t.Fatalf("turn 2: unexpected response %q", text)
	}
	history = updated
	sys2, _ := history[0]["content"].(string)
	if strings.Contains(sys2, "Answering data questions with SQL") {
		t.Fatalf("turn 2 (action): system prompt leaked the SQL schema section: %q", sys2)
	}
	if !strings.Contains(sys2, "POST_api_v1_connectors_trigger_review") {
		t.Fatalf("turn 2 (action): expected the tool list in history[0], got: %q", sys2)
	}
	if sys2 == sys1 {
		t.Fatalf("turn 2: system prompt was not swapped - identical to turn 1's chat prompt")
	}
	if len(prov.toolsPerCall[3]) == 0 {
		t.Fatalf("turn 2 (action call): expected tool schemas to be offered")
	}

	// --- Turn 3: count_query ---
	text, updated, _, err = agent.RunTurnWithArtifacts(context.Background(), history, "reviews per month?", "s1", "test")
	if err != nil {
		t.Fatalf("turn 3: %v", err)
	}
	if !strings.Contains(text, "could not") {
		t.Fatalf("turn 3: expected a graceful degradation message for the rejected query, got: %q", text)
	}
	history = updated
	sys3, _ := history[0]["content"].(string)
	if !strings.Contains(sys3, "Answering data questions with SQL") {
		t.Fatalf("turn 3 (count_query): expected the SQL schema section in history[0], got: %q", sys3)
	}
	if strings.Contains(sys3, "You have access to the following tools") {
		t.Fatalf("turn 3 (count_query): system prompt leaked the general tool list: %q", sys3)
	}
	if len(prov.toolsPerCall[5]) != 0 {
		t.Fatalf("turn 3 (count_query call): expected no tools offered, got %d", len(prov.toolsPerCall[5]))
	}
	if sys3 == sys2 || sys3 == sys1 {
		t.Fatalf("turn 3: system prompt was not swapped to the count_query branch")
	}

	// history[0] must still be exactly ONE system message throughout - the
	// swap replaces it in place, it never accumulates extra system entries.
	systemCount := 0
	for _, h := range history {
		if h["role"] == "system" {
			systemCount++
		}
	}
	if systemCount != 1 {
		t.Fatalf("expected exactly 1 system message in history after 3 turns, found %d", systemCount)
	}
}

// TestDispatchClassifyFailureDegradesToChat verifies the documented
// fallback (livi_analytics_plan.md's "Call #0" risk note): a classify call
// that returns something unparseable must not crash the turn or fall
// through to a stale/wrong prompt - it degrades to the chat branch (no
// schema, no tools) for that turn.
func TestDispatchClassifyFailureDegradesToChat(t *testing.T) {
	prov := &recordingProvider{replies: []string{
		"I'm not sure what you mean, could you clarify?", // classify call: not the {"shape": ...} JSON it was told to return
		"Sure, I'm happy to help with that.",              // the degraded chat-branch call
	}}
	agent := dispatchTestAgent(prov)

	text, history, _, err := agent.RunTurnWithArtifacts(context.Background(), nil, "hello", "s1", "test")
	if err != nil {
		t.Fatalf("expected the turn to degrade gracefully, not error: %v", err)
	}
	if text != "Sure, I'm happy to help with that." {
		t.Fatalf("unexpected response %q", text)
	}
	if len(prov.toolsPerCall) != 2 {
		t.Fatalf("expected exactly 2 provider calls (classify + degraded chat), got %d", len(prov.toolsPerCall))
	}
	if len(prov.toolsPerCall[1]) != 0 {
		t.Fatalf("degraded call should offer no tools (chat branch), got %d", len(prov.toolsPerCall[1]))
	}
	sys, _ := history[0]["content"].(string)
	if strings.Contains(sys, "Answering data questions with SQL") || strings.Contains(sys, "You have access to the following tools") {
		t.Fatalf("degraded turn should use the bare chat prompt, got: %q", sys)
	}
}
