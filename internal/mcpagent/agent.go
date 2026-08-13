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

const (
	DefaultMaxAgentSteps = 20
	maxToolResultLen     = 200000
	toolResultPreviewLen = 500
)

// llmProvider is the slice of *Provider the agent actually uses. Keeping it an
// interface lets the analytics orchestration be driven by a scripted fake in
// tests, so the SQL path can be verified end to end without a live model.
type llmProvider interface {
	Complete(ctx context.Context, history []HistoryEntry, tools []llms.Tool) (string, TokenUsage, error)
	Describe() string
	FormatTools(tools []MCPToolDef) []llms.Tool
}

// Agent runs the ReAct tool-calling loop.
type Agent struct {
	provider      llmProvider
	mcpSession    *MCPSession
	providerTools []llms.Tool
	systemPrompt  string
	maxSteps      int
	// analytics executes generated SQL. Nil leaves the agent tool-only, which
	// is the pre-existing behaviour; see WithAnalytics.
	analytics AnalyticsEngine

	// The following are precomputed once by WithAnalytics and only
	// meaningful when analyticsEnabled() is true - RunTurnWithArtifacts
	// falls back to systemPrompt/providerTools above otherwise, byte-
	// identical to a plain tool-only agent. See livi_analytics_plan.md's
	// "Call #0" section: the point of splitting these out is that each
	// branch's call gets ONLY the prompt content it needs - the action
	// branch never sees the SQL schema, and the count_query/chat branches
	// never see the general tool list.
	classifyPrompt string       // call #0's system prompt: shape instructions + tool NAMES only
	actionPrompt   string       // call #1: identical in shape to the plain tool-only prompt
	actionTools    []llms.Tool  // call #1's tools (raw-row tools withheld, same as before the split)
	chatPrompt     string       // persona/org header only, no tools, no schema
	countQueryHead string       // header + analyticsSchemaIntro, precomputed
	countQueryTail string       // examples + plan instructions, precomputed
	// countQueryHead/Tail bracket dbctxTableText's output, which can only be
	// resolved per turn (schema_index.go's index may not have been ready
	// when WithAnalytics ran) - see (*Agent).countQueryPrompt.
	finalizeHead string // header + analyticsSchemaIntro, precomputed
	finalizeTail string // examples + finalize/chart instructions, precomputed
	// finalizeHead/Tail bracket dbctxTableText's output the same way
	// countQueryHead/Tail do - call #3 writes its own data_sql from scratch
	// (it is not handed the count call's SQL to extend) and needs the same
	// column-name grounding, or it silently guesses wrong column names for
	// anything the count query didn't already select. See (*Agent).finalizePrompt.
}

func NewAgent(provider *Provider, mcpSession *MCPSession, maxSteps int) *Agent {
	if maxSteps <= 0 {
		maxSteps = DefaultMaxAgentSteps
	}
	tools := provider.FormatTools(mcpSession.Tools)
	systemPrompt := buildSystemPrompt(mcpSession.Tools, mcpSession.OrgName, mcpSession.UserName)

	return &Agent{
		provider:      provider,
		mcpSession:    mcpSession,
		providerTools: tools,
		systemPrompt:  systemPrompt,
		maxSteps:      maxSteps,
	}
}

// RunTurn processes one user message through the ReAct loop and returns the
// final text response and updated history. sessionID/source identify the
// conversation for the debug log (see internal/logging.ChatTurnLogger) - the
// Agent instance itself is reused across many sessions by the bots, so the
// session identity has to be passed in per call rather than baked into Agent.
func (a *Agent) RunTurn(ctx context.Context, history []HistoryEntry, userText string, sessionID, source string) (string, []HistoryEntry, error) {
	text, updated, _, err := a.RunTurnWithArtifacts(ctx, history, userText, sessionID, source)
	return text, updated, err
}

// RunTurnWithArtifacts is RunTurn plus any files the turn produced (CSV
// exports). Callers that cannot deliver a file - the bots today - can keep
// using RunTurn and lose nothing but the attachment.
func (a *Agent) RunTurnWithArtifacts(ctx context.Context, history []HistoryEntry, userText string, sessionID, source string) (string, []HistoryEntry, []Artifact, error) {
	log.Debug().Int("history_entries", len(history)).Int("user_text_len", len(userText)).Msg("Agent RunTurn starting")

	clog := logging.NewChatTurnLogger(sessionID, source)
	clog.Context(a.mcpSession.OrgName, a.mcpSession.UserName, a.provider.Describe())
	clog.UserInput(userText)

	systemPrompt := a.systemPrompt
	tools := a.providerTools
	// callNumber is livi_analytics_plan.md's "Call #0" diagram number for
	// whichever call the main step loop below ends up making (1=action,
	// 2=count-query proposal). 0 means "not a numbered diagram call" - the
	// chat branch, or a plain tool-only agent with analytics disabled.
	callNumber := 0

	// Call #0: classify before paying for schema/tool-schema tokens. Only
	// runs when analytics is enabled - a plain tool-only agent keeps its
	// single fixed prompt, byte-identical to before this split existed.
	if a.analyticsEnabled() {
		shape, err := a.classify(ctx, history, userText, clog)
		if err != nil {
			// No retry here: a second classify call would double the very
			// cost this split exists to avoid. Degrade to the safest
			// branch - no schema, no tool side effects triggered blindly -
			// and let the user re-ask if that guess was wrong.
			log.Warn().Err(err).Msg("call #0 classify failed; degrading to chat-only response for this turn")
			shape = shapeChat
		}

		switch shape {
		case shapeAction:
			systemPrompt = a.actionPrompt
			tools = a.actionTools
			callNumber = 1
		case shapeCountQuery:
			systemPrompt = a.countQueryPrompt(clog, userText)
			tools = nil
			callNumber = 2
		default:
			systemPrompt = a.chatPrompt
			tools = nil
		}
		clog.BranchSelected(string(shape), len(systemPrompt), len(tools))
	}

	// Swapped every turn, not appended once: which prompt a turn needs is
	// decided fresh above, so a session that changes shape turn-to-turn
	// (a data question, then "trigger a review for that repo", then
	// another data question) must not keep whatever was baked in on turn
	// one. The rest of history (actual conversation content) is untouched.
	switch {
	case len(history) == 0:
		if systemPrompt != "" {
			history = append(history, HistoryEntry{"role": "system", "content": systemPrompt})
		}
	case history[0]["role"] == "system":
		if systemPrompt != "" {
			history[0] = HistoryEntry{"role": "system", "content": systemPrompt}
		}
	default:
		if systemPrompt != "" {
			history = append([]HistoryEntry{{"role": "system", "content": systemPrompt}}, history...)
		}
	}
	history = append(history, HistoryEntry{"role": "user", "content": userText})

	for step := 0; step < a.maxSteps; step++ {
		log.Debug().Int("step", step).Int("history_len", len(history)).Int("num_tools", len(tools)).Msg("Calling LLM")
		if clog.Enabled() {
			if b, err := json.Marshal(history); err == nil {
				clog.AIRequest(callNumber, step, string(b))
			}
		}
		aiStart := time.Now()
		response, usage, err := a.provider.Complete(ctx, history, tools)
		aiElapsed := time.Since(aiStart)
		if err != nil {
			log.Error().Err(err).Int("step", step).Msg("LLM completion failed")
			clog.AIError(callNumber, step, aiElapsed, err)
			return "", history, nil, fmt.Errorf("llm completion step %d: %w", step, err)
		}
		log.Debug().Int("step", step).Int("response_len", len(response)).Msg("LLM call succeeded")
		clog.AIResponse(callNumber, step, aiElapsed, usage.InputTokens, usage.OutputTokens, response)

		history = append(history, HistoryEntry{"role": "assistant", "text": response})

		toolCalls := parseToolCalls(response)
		if len(toolCalls) == 0 {
			// A response carrying no `tool` field falls through parseToolCalls
			// untouched, so this is where an analytics plan surfaces. Checked
			// before the empty/excuse nags so a valid plan is never mistaken for
			// a non-answer.
			if a.analyticsEnabled() {
				if plan, ok := parseAnalyticsPlan(response); ok {
					clog.SQLPlan(step, response)
					// history's last entry is the raw analytics_plan JSON just
					// appended above (line 181) - runAnalyticsPlan appends its
					// own final user-facing text in its place. Without
					// dropping it here, both entries would persist: the raw
					// plan JSON AND the real answer, as two separate
					// "assistant" turns. That raw JSON then leaks into every
					// later call's conversation history (classify included),
					// and the model starts imitating its own prior "reply
					// with a JSON blob" turn instead of following whatever
					// that later call actually asked for.
					return a.runAnalyticsPlan(ctx, plan, history[:len(history)-1], userText, clog)
				}
			}
			// The "you MUST call a tool" nudges below only make sense when
			// this turn's branch actually offered tools (the action
			// branch). The count_query/chat branches offer none - after
			// the call #0 split, telling the model to call a tool it was
			// never given would be nonsensical, not a helpful retry.
			if len(tools) == 0 {
				if strings.TrimSpace(response) == "" {
					log.Warn().Int("step", step).Msg("AI returned an empty response on a no-tools branch, forcing retry")
					history = append(history, HistoryEntry{
						"role":    "user",
						"content": "Your previous reply was empty. Please give a complete answer to the question.",
					})
					continue
				}
				// A whole-message JSON object that got this far matched
				// neither a tool call nor a valid analytics plan, so it is
				// not a shape anything downstream understands - showing it
				// verbatim just dumps raw JSON into the chat UI (observed:
				// a misclassified turn on the chat branch fabricated a
				// {"chart": ..., "learning": ...} object nothing parses).
				// One retry asking for prose is cheap and much better than
				// that.
				if looksLikeUnrecognizedJSON(response) {
					log.Warn().Int("step", step).Str("response_preview", truncateContent(response, 200)).
						Msg("AI produced an unrecognized JSON object on a no-tools branch, forcing retry")
					history = append(history, HistoryEntry{
						"role":    "user",
						"content": "Your previous reply was a JSON object, but this turn has no data or chart capability. Answer in plain prose instead - do not output JSON.",
					})
					continue
				}
				clog.FinalResponse(response)
				return response, history, nil, nil
			}
			if strings.TrimSpace(response) == "" {
				log.Warn().Int("step", step).Msg("AI returned an empty response, forcing retry")
				history = append(history, HistoryEntry{
					"role":    "user",
					"content": "Your previous reply was empty. You MUST call at least one tool before you can respond to the user, then give a complete answer. If the exact data isn't available, call the closest tool you have and create a chart from whatever data you receive.",
				})
				continue
			}
			if isExcuseResponse(response) {
				log.Warn().Str("response_preview", truncateContent(response, 200)).Msg("AI gave excuse response, forcing retry")
				history = append(history, HistoryEntry{
					"role":    "user",
					"content": "You did not call any tools. You MUST call at least one tool before you can respond to the user. If the exact data isn't available, call the closest tool you have and create a chart from whatever data you receive. If you cannot answer the question directly, suggest what the user CAN ask instead. Do not apologize or say you can't do something.",
				})
				continue
			}
			clog.FinalResponse(response)
			return response, history, nil, nil
		}

		for _, tc := range toolCalls {
			log.Info().Str("tool", tc.Name).Any("arguments", tc.Arguments).Msg("Calling MCP tool")
			if clog.Enabled() {
				if b, err := json.Marshal(tc.Arguments); err == nil {
					clog.ToolCall(step, tc.Name, string(b))
				}
			}
			toolStart := time.Now()
			content, err := CallTool(ctx, a.mcpSession, tc.Name, tc.Arguments)
			toolElapsed := time.Since(toolStart)
			if err != nil {
				clog.ToolError(step, tc.Name, toolElapsed, err)
				content = fmt.Sprintf("[Tool call failed: %s]", err)
			}
			displayLen := len(content)
			content = truncateContent(content, maxToolResultLen)
			truncated := displayLen > maxToolResultLen
			if truncated {
				content += "\n\n_[Result truncated to " + fmt.Sprintf("%d", maxToolResultLen) + " characters — original was " + fmt.Sprintf("%d", displayLen) + " chars. You can request data in smaller batches (lower perPage) or additional pages.]_"
			}
			if err == nil {
				clog.ToolResponse(step, tc.Name, toolElapsed, displayLen, truncated, content)
			}
			log.Debug().Str("tool", tc.Name).Int("result_len", displayLen).Msg("MCP tool result received")
			log.Debug().Str("tool", tc.Name).Str("result_preview", content[:min(len(content), toolResultPreviewLen)]).Msg("MCP tool result (truncated for LLM)")

			// An expired/invalid session token can't be fixed by trying a
			// different tool - every tool call will fail identically. Without
			// this, the model (per its "never give up, try another tool"
			// instructions) burns a full LLM call per tool it tries, cycling
			// through most of the tool list before giving up.
			if isAuthError(content) {
				const authErrorResponse = "Your session has expired. Please refresh the page and sign in again."
				log.Warn().Str("tool", tc.Name).Msg("Auth error from tool call, stopping agent loop instead of retrying")
				clog.FinalResponse(authErrorResponse + " (stopped after auth error from " + tc.Name + ")")
				return authErrorResponse, history, nil, nil
			}

			history = append(history, HistoryEntry{
				"role":    "user",
				"content": fmt.Sprintf("Result of `%s`:\n```\n%s\n```", tc.Name, content),
			})
		}

		log.Debug().Int("step", step).Int("history_len", len(history)).Int("tool_calls", len(toolCalls)).Msg("Agent step complete")
	}

	log.Warn().Int("max_steps", a.maxSteps).Msg("Agent hit step limit")
	clog.StepLimitHit(a.maxSteps)
	return "I hit my step limit trying to finish that — try breaking the request down.", history, nil, nil
}

// personaIntro is the one sentence every branch's prompt opens with -
// shared by buildSystemPrompt (tool-bearing branches: the plain tool-only
// agent and call #1's action branch) and buildPromptHeader (call #2's
// count_query branch and the chat branch, neither of which lists tools).
const personaIntro = "You are an AI assistant connected to a LiveReview API server. " +
	"You act as a friendly persona (Livi), not a faceless system. Speak in the first person and take ownership of the actions you perform: when you trigger a review, create a learning, or add a connector, YOU did it — never refer to 'the system' doing it.\n"

// buildPromptHeader is the persona + org/user context block with no tool
// list - the shared prefix for branches that never call a tool
// (count_query, chat). Kept separate from buildSystemPrompt rather than
// having the tool-bearing prompt strip its own tool section, so the
// no-tools prompt never risks carrying "You have access to the following
// tools:" language when there are none.
func buildPromptHeader(orgName, userName string) string {
	var b strings.Builder
	b.WriteString(personaIntro)
	if orgName != "" || userName != "" {
		b.WriteString("The user you are helping belongs to the following context. This is MANDATORY:\n")
		if userName != "" {
			b.WriteString(fmt.Sprintf("- User: %s\n", userName))
		}
		if orgName != "" {
			b.WriteString(fmt.Sprintf("- Organization: %s\n", orgName))
		}
		b.WriteString("\n")
		b.WriteString(orgPromptGuidance) // imported from prompts/org_prompt.md
		b.WriteString("\n")
	}
	return b.String()
}

// buildSystemPrompt is the tool-bearing prompt: the plain tool-only agent
// (NewAgent) and call #1's action branch both use it unchanged - the action
// branch is deliberately built to look exactly like the pre-analytics
// prompt, since its whole point is to never carry SQL/schema content.
func buildSystemPrompt(tools []MCPToolDef, orgName, userName string) string {
	if len(tools) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(personaIntro)
	b.WriteString("You have access to the following tools:\n\n")

	if orgName != "" || userName != "" {
		b.WriteString("The user you are helping belongs to the following context. This is MANDATORY:\n")
		if userName != "" {
			b.WriteString(fmt.Sprintf("- User: %s\n", userName))
		}
		if orgName != "" {
			b.WriteString(fmt.Sprintf("- Organization: %s\n", orgName))
		}
		b.WriteString("\n")
		b.WriteString(orgPromptGuidance) // imported from prompts/org_prompt.md
		b.WriteString("\n")
	}

	for _, t := range tools {
		b.WriteString(fmt.Sprintf("- `%s`", t.Name))
		if t.Description != "" {
			b.WriteString(fmt.Sprintf(": %s", t.Description))
		}
		b.WriteString("\n")
	}
	b.WriteString(agentInstructions) // imported from prompts/agent_instructions.md

	return b.String()
}

// orgIDFilterInstruction tells the model the literal org_id value every SQL
// query it writes must filter by. This has to be explicit now: the SQL
// guard (internal/livisql) no longer injects org scoping itself - it only
// checks that some `org_id = ...` comparison is present (see the guard's
// package doc) - so the model is the only place the correct value can come
// from at all.
func orgIDFilterInstruction(orgID int64) string {
	return fmt.Sprintf("\nEvery query you write MUST include `org_id = %d` in its WHERE clause "+
		"(join other tables' own org_id the same way). This is the organization's actual id - "+
		"use this exact number, not a placeholder or a guess. A query without it will be rejected.\n\n", orgID)
}

// buildCountQueryPromptHalves precomputes the static parts of call #2's
// system prompt. They bracket dbctxTableText's output, which is resolved
// fresh per turn by (*Agent).countQueryPrompt rather than here, because the
// dbctx index's readiness can change over the process's life in a way none
// of the other precomputed prompts depend on.
func buildCountQueryPromptHalves(orgName, userName string, orgID int64) (head, tail string) {
	var h strings.Builder
	h.WriteString(buildPromptHeader(orgName, userName))
	h.WriteString(orgIDFilterInstruction(orgID))
	h.WriteString(analyticsSchemaIntro) // imported from prompts/analytics_schema_intro.md
	head = h.String()

	var t strings.Builder
	t.WriteString("\n\n")
	t.WriteString(analyticsSchemaExamples) // imported from prompts/analytics_schema_examples.md
	t.WriteString("\n\n")
	t.WriteString(analyticsPlanInstructions) // imported from prompts/analytics_plan.md
	tail = t.String()

	return head, tail
}

// countQueryPrompt assembles call #2's full system prompt for this turn,
// splicing the live dbctx table text between the precomputed head and tail.
// Logs SchemaSourceDegraded when the live index wasn't available and the
// static fallback table list was used instead - see schema_render.go.
func (a *Agent) countQueryPrompt(clog *logging.ChatTurnLogger, userText string) string {
	clog.DBCtxRequest(2, string(a.analyticsRole()), userText)
	start := time.Now()
	tableText, err := dbctxTableText(userText)
	clog.DBCtxResponse(2, time.Since(start), tableText, err)
	if err != nil {
		clog.SchemaSourceDegraded(err.Error())
	}
	return a.countQueryHead + "\n\n" + tableText + a.countQueryTail
}

// buildFinalizePromptHalves precomputes the static parts of call #3's system
// prompt, mirroring buildCountQueryPromptHalves. Call #3 writes a brand new
// data_sql (not a reuse of the count call's SQL, and not part of the same
// conversation - completeOnce is a fresh two-message exchange), so it needs
// the same table/column reference the count call gets, not just the chart-
// format rules in analytics_finalize.md.
func buildFinalizePromptHalves(orgName, userName string, orgID int64) (head, tail string) {
	var h strings.Builder
	h.WriteString(buildPromptHeader(orgName, userName))
	h.WriteString(orgIDFilterInstruction(orgID))
	h.WriteString(analyticsSchemaIntro) // imported from prompts/analytics_schema_intro.md
	head = h.String()

	var t strings.Builder
	t.WriteString("\n\n")
	t.WriteString(analyticsSchemaExamples) // imported from prompts/analytics_schema_examples.md
	t.WriteString("\n\n")
	t.WriteString(analyticsFinalizeInstructions) // imported from prompts/analytics_finalize.md
	tail = t.String()

	return head, tail
}

// finalizePrompt assembles call #3's full system prompt for this turn, the
// same way countQueryPrompt does for call #2. queryText is the specific
// report question (entry.Question) rather than the original raw user
// message - by call #3 that's the more precise text to narrow the schema
// against, since one user turn can fan out into several reports each
// needing different tables.
func (a *Agent) finalizePrompt(clog *logging.ChatTurnLogger, queryText string) string {
	clog.DBCtxRequest(3, string(a.analyticsRole()), queryText)
	start := time.Now()
	tableText, err := dbctxTableText(queryText)
	clog.DBCtxResponse(3, time.Since(start), tableText, err)
	if err != nil {
		clog.SchemaSourceDegraded(err.Error())
	}
	return a.finalizeHead + "\n\n" + tableText + a.finalizeTail
}

// isAuthError reports whether a tool result signals an expired/invalid
// session token or missing auth - the exact messages LiveReview's own auth
// middleware returns (internal/api/auth/middleware.go). Retrying with a
// different tool can never recover from these; every tool call would fail
// identically until the user re-authenticates.
func isAuthError(content string) bool {
	authErrorSubstrings := []string{
		"Invalid or expired token",
		"Authorization header required",
		"Invalid authorization header format",
	}
	for _, s := range authErrorSubstrings {
		if strings.Contains(content, s) {
			return true
		}
	}
	return false
}

// looksLikeUnrecognizedJSON reports whether response, once any code fence is
// stripped, looks like an attempted JSON object - complete or not. A prose
// answer never starts with `{` as its first character, so this only fires
// on a response that was clearly attempting some structured shape - one
// this codebase's parsers already had a chance to recognize (parseToolCalls,
// parseAnalyticsPlan) and did not. Deliberately does NOT require the body to
// end with `}`: a response cut off mid-object (observed - Gemini's reasoning
// eating into its output budget, tokens_out in the tens for a call that
// should have been hundreds) is exactly the case this exists to catch, and
// requiring a closing brace let a truncated {"analytics_plan": [...] reach
// the user as raw JSON text instead of triggering this retry.
func looksLikeUnrecognizedJSON(response string) bool {
	body := strings.TrimSpace(vlrender.ExtractJSONBlock(response))
	return strings.HasPrefix(body, "{")
}

func isExcuseResponse(response string) bool {
	lower := strings.ToLower(response)
	excusePatterns := []string{
		"i cannot",
		"i can't",
		"i'm unable",
		"i am unable",
		"i'm not able",
		"i am not able",
		"i cannot directly",
		"cannot directly provide",
		"there is no tool",
		"there's no tool",
		"no tool available",
		"no tools available",
		"cannot provide",
		"cannot show",
		"don't have access",
		"do not have access",
		"don't have the ability",
		"do not have the ability",
		"not designed to",
		"i'm sorry",
		"i apologize",
		"i don't have a tool",
		"i do not have a tool",
		"i don't have the tool",
		"i do not have the tool",
	}
	for _, p := range excusePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// parseToolCalls extracts tool calls from a JSON code block in the response.
func parseToolCalls(text string) []ToolCall {
	// Find ```json ... ``` blocks
	var calls []ToolCall

	for {
		start := strings.Index(text, "```json")
		if start < 0 {
			break
		}
		start += len("```json")
		end := strings.Index(text[start:], "```")
		if end < 0 {
			break
		}
		block := strings.TrimSpace(text[start : start+end])

		// Try to parse as a single tool call
		var single struct {
			Tool      string         `json:"tool"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(block), &single); err == nil && single.Tool != "" {
			calls = append(calls, ToolCall{Name: single.Tool, Arguments: single.Arguments})
			text = text[start+end+3:]
			continue
		}

		// Try to parse as an array of tool calls
		var arr []struct {
			Tool      string         `json:"tool"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(block), &arr); err == nil && len(arr) > 0 {
			for _, item := range arr {
				if item.Tool != "" {
					calls = append(calls, ToolCall{Name: item.Tool, Arguments: item.Arguments})
				}
			}
			text = text[start+end+3:]
			continue
		}

		// Not a valid tool call block, move past it
		text = text[start+end+3:]
	}

	if len(calls) > 0 {
		return calls
	}

	// Fallback: the model sometimes emits the tool call as BARE JSON without a
	// ```json fence (e.g. `[{"tool": "GET_api_v1_reviews", "arguments": {...}}]`).
	// Only treat it as a tool call if it actually carries a `tool` field — chart
	// specs (title/spec/reports) never do, so they are left untouched.
	trimmed := strings.TrimSpace(text)

	var single struct {
		Tool      string         `json:"tool"`
		Arguments map[string]any `json:"arguments"`
	}
	if strings.HasPrefix(trimmed, "{") {
		if err := json.Unmarshal([]byte(trimmed), &single); err == nil && single.Tool != "" {
			return []ToolCall{{Name: single.Tool, Arguments: single.Arguments}}
		}
		return nil
	}

	if strings.HasPrefix(trimmed, "[") {
		var arr []struct {
			Tool      string         `json:"tool"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(trimmed), &arr); err == nil && len(arr) > 0 {
			for _, item := range arr {
				if item.Tool != "" {
					calls = append(calls, ToolCall{Name: item.Tool, Arguments: item.Arguments})
				}
			}
			if len(calls) > 0 {
				return calls
			}
		}
	}

	return nil
}

func truncateContent(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
