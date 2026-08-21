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
	Complete(ctx context.Context, history []HistoryEntry, tools []llms.Tool, extraOpts ...llms.CallOption) (string, TokenUsage, error)
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
	classifyPrompt string      // call #0's system prompt: shape instructions + tool NAMES only
	actionPrompt   string      // call #1: identical in shape to the plain tool-only prompt
	actionTools    []llms.Tool // call #1's tools (raw-row tools withheld, same as before the split)
	chatPrompt     string      // persona/org header only, no tools, no schema
	countQueryHead string      // header, precomputed from lawbook
	countQueryTail string      // plan laws, precomputed from lawbook
	// countQueryHead/Tail bracket dbctxTableText's output, which can only be
	// resolved per turn (schema_index.go's index may not have been ready
	// when WithAnalytics ran) - see (*Agent).countQueryPrompt.
	finalizeHead string // header, precomputed from lawbook
	finalizeTail string // finalize + chart-selection laws, precomputed from lawbook
	// finalizeHead/Tail bracket dbctxTableText's output the same way
	// countQueryHead/Tail do - call #3 writes its own data_sql from scratch
	// (it is not handed the count call's SQL to extend) and needs the same
	// column-name grounding, or it silently guesses wrong column names for
	// anything the count query didn't already select. See (*Agent).finalizePrompt.

	// repairPrompt and noDataPrompt are the full system prompts for the
	// degraded paths (rejected query retry and zero-row result). Populated
	// from the lawbook when available, falling back to the old embedded
	// prompts if the lawbook fails to load.
	repairPrompt string
	noDataPrompt string
	// describePrompt is the system prompt for the post-data description
	// call - see alaws_livi/describe.md and (*Agent).regenerateDescription.
	describePrompt string
	// interpretSystem is the fully rendered multi-interpret system prompt
	// (self-contained, no schema splice — schema goes in the user message).
	interpretSystem string
	// chartTypes is the embedded chart_types.json content, passed in the
	// user message alongside the dbctx schema context.
	chartTypes string
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
	text, updated, _, _, err := a.RunTurnWithArtifacts(ctx, history, userText, sessionID, source)
	return text, updated, err
}

// RunTurnWithArtifacts is RunTurn plus any files the turn produced (CSV
// exports). Callers that cannot deliver a file - the bots today - can keep
// using RunTurn and lose nothing but the attachment.
func (a *Agent) RunTurnWithArtifacts(ctx context.Context, history []HistoryEntry, userText string, sessionID, source string) (string, []HistoryEntry, []Artifact, *DebugArtifacts, error) {
	log.Debug().Int("history_entries", len(history)).Int("user_text_len", len(userText)).Msg("Agent RunTurn starting")

	clog := logging.NewChatTurnLogger(sessionID, source)
	clog.Context(a.mcpSession.OrgName, a.mcpSession.UserName, a.provider.Describe())
	clog.UserInput(userText)

	// The schema index is a precondition, not a nicety. Without it the
	// model is never shown the real tables and columns, so every call this
	// turn would make is either wasted or - worse - answered against
	// guessed column names. Refusing up front costs nothing; proceeding
	// costs a full turn of tokens to produce something untrustworthy.
	if a.analyticsEnabled() && !schemaIndexReady() {
		const msg = "Livi is in preparing mode, please come back after 60s."
		log.Warn().Msg("schema index not ready; refusing the turn before any LLM call")
		clog.FinalResponse(msg)
		return msg, history, nil, nil, nil
	}

	// Call #0: classify before paying for schema/tool-schema tokens. Only
	// runs when analytics is enabled - a plain tool-only agent keeps its
	// single fixed prompt, byte-identical to before this split existed.
	if a.analyticsEnabled() {
		shape, err := a.classify(ctx, history, userText, clog)
		if err != nil {
			log.Warn().Err(err).Msg("call #0 classify failed; degrading to chat-only response for this turn")
			shape = shapeChat
		}

		if shape == shapeCountQuery {
			// Multi-interpret pipeline (ported from prelivi Python script):
			// single LLM call returns SQL + chart specs for up to 5
			// interpretations. No plan→finalize two-step.
			clog.BranchSelected("count_query", 0, 0)
			text, artifacts, debugArt, err := a.runMultiInterpret(ctx, userText, clog, a.mcpSession.OrgID, a.mcpSession.OrgName)
			if err != nil {
				return text, history, artifacts, debugArt, err
			}
			history = append(history, HistoryEntry{"role": "assistant", "text": text})
			return text, history, artifacts, debugArt, nil
		}

		// Action and chat branches use the existing step loop.
		systemPrompt := a.systemPrompt
		tools := a.providerTools
		callNumber := 0
		jsonMode := false
		planRetried := false

		switch shape {
		case shapeAction:
			systemPrompt = a.actionPrompt
			tools = a.actionTools
			callNumber = 1
		default:
			systemPrompt = a.chatPrompt
			tools = nil
		}
		clog.BranchSelected(string(shape), len(systemPrompt), len(tools))

		return a.runStepLoop(ctx, history, userText, systemPrompt, tools, callNumber, jsonMode, planRetried, "", clog)
	}

	// Non-analytics path (plain tool-only agent).
	return a.runStepLoop(ctx, history, userText, a.systemPrompt, a.providerTools, 0, false, false, "", clog)
}

// runStepLoop is the ReAct tool-calling loop. Extracted from RunTurnWithArtifacts
// so the count_query branch can skip it entirely (using runMultiInterpret instead)
// while action and chat branches continue through the step loop as before.
func (a *Agent) runStepLoop(
	ctx context.Context,
	history []HistoryEntry,
	userText string,
	systemPrompt string,
	tools []llms.Tool,
	callNumber int,
	jsonMode bool,
	planRetried bool,
	schemaTableText string,
	clog *logging.ChatTurnLogger,
) (string, []HistoryEntry, []Artifact, *DebugArtifacts, error) {
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
		var completeOpts []llms.CallOption
		if jsonMode {
			completeOpts = append(completeOpts, llms.WithJSONMode())
		}
		response, usage, err := a.provider.Complete(ctx, history, tools, completeOpts...)
		aiElapsed := time.Since(aiStart)
		if err != nil {
			log.Error().Err(err).Int("step", step).Msg("LLM completion failed")
			clog.AIError(callNumber, step, aiElapsed, err)
			return "", history, nil, nil, fmt.Errorf("llm completion step %d: %w", step, err)
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
					text, hist, arts, err := a.runAnalyticsPlan(ctx, plan, history[:len(history)-1], userText, schemaTableText, clog)
					return text, hist, arts, nil, err
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
				return response, history, nil, nil, nil
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
			return response, history, nil, nil, nil
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

			if isAuthError(content) {
				const authErrorResponse = "Your session has expired. Please refresh the page and sign in again."
				log.Warn().Str("tool", tc.Name).Msg("Auth error from tool call, stopping agent loop instead of retrying")
				clog.FinalResponse(authErrorResponse + " (stopped after auth error from " + tc.Name + ")")
				return authErrorResponse, history, nil, nil, nil
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
	return "I hit my step limit trying to finish that — try breaking the request down.", history, nil, nil, nil
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
	b.WriteString(actionInstructions()) // rendered from alaws_livi/action*.md

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

// countQueryPrompt assembles call #2's full system prompt for this turn,
// splicing the live dbctx table text between the precomputed head and tail.
// Logs SchemaSourceDegraded when the live index wasn't available and the
// static fallback table list was used instead - see schema_render.go.
//
// Also returns the raw tableText itself: this is the one and only dbctx
// fetch/narrowing call for the whole turn. Every later step that needs
// schema in its prompt (today: finalizePrompt, once per report) reuses this
// same rendered text instead of re-querying dbctx - see finalizePrompt's
// doc comment for why a second, differently-narrowed fetch isn't needed.
func (a *Agent) countQueryPrompt(clog *logging.ChatTurnLogger, userText string) (systemPrompt, tableText string, err error) {
	clog.DBCtxRequest(2, string(a.analyticsRole()), userText)
	start := time.Now()
	tableText, err = dbctxTableText(userText)
	clog.DBCtxResponse(2, time.Since(start), tableText, err)
	if err != nil {
		// No fallback schema. A prompt without the real table listing
		// invites SQL written against invented columns, which fails late
		// and confusingly - after the tokens are spent - instead of here.
		clog.SchemaSourceDegraded(err.Error())
		return "", "", err
	}
	return a.countQueryHead + "\n\n" + tableText + a.countQueryTail, tableText, nil
}

// finalizePrompt assembles call #3's full system prompt for this turn.
// tableText is the schema text dbctx rendered once for call #2
// (countQueryPrompt), reused here rather than re-fetched: call #2 narrows
// against the raw user message, which in practice renders the large
// majority of the catalog already (observed: 36 of ~37 tables for a
// realistic query), so a second, more narrowly-targeted fetch per report
// bought negligible precision for the cost of an extra ~10s dbctx round
// trip and a near-duplicate schema block in every report's prompt. Call #3
// is still its own fresh, isolated completeOnce exchange (no shared
// conversation history with call #2 or with other reports' finalize
// calls - see completeOnce's doc comment on why that isolation matters);
// only the already-rendered table text is shared, not the conversation.
// If a report's SQL ever needs a table this fetch didn't surface, the
// existing SQL-guard repair loop (runFinalizePhase) catches and fixes it
// the same way it catches any other rejected query.
func (a *Agent) finalizePrompt(tableText string) string {
	return a.finalizeHead + "\n\n" + tableText + a.finalizeTail
}

// interpretPrompt returns the self-contained multi-interpret system prompt
// (rendered from the PromptBook template with org_id/org_name variables).
// The system prompt includes all rules inline — no schema splice needed.
// Schema context and chart types are added to the user message instead
// (matching the Python script's approach in scripts/prelivi/interpretation.py).
func (a *Agent) interpretPrompt() string {
	return a.interpretSystem
}

// interpretUserMessage builds the user message for the multi-interpret
// pipeline, matching the Python script format:
//
//	User query: {query}
//	Org context: org_id = {ORG_ID} ({ORG_NAME})
//
//	--- dbctx schema context ---
//	{dbctx_context}
//
//	--- available chart types ---
//	{chart_types}
func (a *Agent) interpretUserMessage(clog *logging.ChatTurnLogger, userText string, orgID int64, orgName string) (string, error) {
	clog.DBCtxRequest(2, string(a.analyticsRole()), userText)
	start := time.Now()
	tableText, err := dbctxTableText(userText)
	clog.DBCtxResponse(2, time.Since(start), tableText, err)
	if err != nil {
		clog.SchemaSourceDegraded(err.Error())
		return "", err
	}
	msg := fmt.Sprintf(
		"User query: %s\nOrg context: org_id = %d (%s)\n\n--- dbctx schema context ---\n%s\n\n--- available chart types ---\n%s",
		userText, orgID, orgName, tableText, a.chartTypes,
	)
	return msg, nil
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
