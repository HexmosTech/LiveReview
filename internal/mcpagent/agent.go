package mcpagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/livereview/internal/logging"
	"github.com/rs/zerolog/log"
	"github.com/tmc/langchaingo/llms"
)

const (
	DefaultMaxAgentSteps = 20
	maxToolResultLen     = 200000
	toolResultPreviewLen = 500
)

// Agent runs the ReAct tool-calling loop.
type Agent struct {
	provider      *Provider
	mcpSession    *MCPSession
	providerTools []llms.Tool
	systemPrompt  string
	maxSteps      int
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
	log.Debug().Int("history_entries", len(history)).Int("user_text_len", len(userText)).Msg("Agent RunTurn starting")

	clog := logging.NewChatTurnLogger(sessionID, source)
	clog.Context(a.mcpSession.OrgName, a.mcpSession.UserName, a.provider.Describe())
	clog.UserInput(userText)

	if len(history) == 0 && a.systemPrompt != "" {
		history = append(history, HistoryEntry{"role": "system", "content": a.systemPrompt})
	}
	history = append(history, HistoryEntry{"role": "user", "content": userText})

	for step := 0; step < a.maxSteps; step++ {
		log.Debug().Int("step", step).Int("history_len", len(history)).Int("num_tools", len(a.providerTools)).Msg("Calling LLM")
		if clog.Enabled() {
			if b, err := json.Marshal(history); err == nil {
				clog.AIRequest(step, string(b))
			}
		}
		aiStart := time.Now()
		response, err := a.provider.Complete(ctx, history, a.providerTools)
		aiElapsed := time.Since(aiStart)
		if err != nil {
			log.Error().Err(err).Int("step", step).Msg("LLM completion failed")
			clog.AIError(step, aiElapsed, err)
			return "", history, fmt.Errorf("llm completion step %d: %w", step, err)
		}
		log.Debug().Int("step", step).Int("response_len", len(response)).Msg("LLM call succeeded")
		clog.AIResponse(step, aiElapsed, response)

		history = append(history, HistoryEntry{"role": "assistant", "text": response})

		toolCalls := parseToolCalls(response)
		if len(toolCalls) == 0 {
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
			return response, history, nil
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
				return authErrorResponse, history, nil
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
	return "I hit my step limit trying to finish that — try breaking the request down.", history, nil
}

func buildSystemPrompt(tools []MCPToolDef, orgName, userName string) string {
	if len(tools) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("You are an AI assistant connected to a LiveReview API server. ")
	b.WriteString("You act as a friendly persona (Livi), not a faceless system. Speak in the first person and take ownership of the actions you perform: when you trigger a review, create a learning, or add a connector, YOU did it — never refer to 'the system' doing it.\n")
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
