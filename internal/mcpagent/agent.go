package mcpagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
// final text response and updated history.
func (a *Agent) RunTurn(ctx context.Context, history []HistoryEntry, userText string) (string, []HistoryEntry, error) {
	log.Debug().Int("history_entries", len(history)).Int("user_text_len", len(userText)).Msg("Agent RunTurn starting")

	if len(history) == 0 && a.systemPrompt != "" {
		history = append(history, HistoryEntry{"role": "system", "content": a.systemPrompt})
	}
	history = append(history, HistoryEntry{"role": "user", "content": userText})

	for step := 0; step < a.maxSteps; step++ {
		log.Debug().Int("step", step).Int("history_len", len(history)).Int("num_tools", len(a.providerTools)).Msg("Calling LLM")
		response, err := a.provider.Complete(ctx, history, a.providerTools)
		if err != nil {
			log.Error().Err(err).Int("step", step).Msg("LLM completion failed")
			return "", history, fmt.Errorf("llm completion step %d: %w", step, err)
		}
		log.Debug().Int("step", step).Int("response_len", len(response)).Msg("LLM call succeeded")

		history = append(history, HistoryEntry{"role": "assistant", "text": response})

		toolCalls := parseToolCalls(response)
		if len(toolCalls) == 0 {
			if isExcuseResponse(response) {
				log.Warn().Str("response_preview", truncateContent(response, 200)).Msg("AI gave excuse response, forcing retry")
				history = append(history, HistoryEntry{
					"role":    "user",
					"content": "You did not call any tools. You MUST call at least one tool before you can respond to the user. If the exact data isn't available, call the closest tool you have and create a chart from whatever data you receive. If you cannot answer the question directly, suggest what the user CAN ask instead. Do not apologize or say you can't do something.",
				})
				continue
			}
			return response, history, nil
		}

		for _, tc := range toolCalls {
			log.Info().Str("tool", tc.Name).Any("arguments", tc.Arguments).Msg("Calling MCP tool")
			content, err := CallTool(ctx, a.mcpSession, tc.Name, tc.Arguments)
			if err != nil {
				content = fmt.Sprintf("[Tool call failed: %s]", err)
			}
			displayLen := len(content)
			content = truncateContent(content, maxToolResultLen)
			if displayLen > maxToolResultLen {
				content += "\n\n_[Result truncated to " + fmt.Sprintf("%d", maxToolResultLen) + " characters — original was " + fmt.Sprintf("%d", displayLen) + " chars. You can request data in smaller batches (lower perPage) or additional pages.]_"
			}
			log.Debug().Str("tool", tc.Name).Int("result_len", displayLen).Msg("MCP tool result received")
			log.Debug().Str("tool", tc.Name).Str("result_preview", content[:min(len(content), toolResultPreviewLen)]).Msg("MCP tool result (truncated for LLM)")
			history = append(history, HistoryEntry{
				"role":    "user",
				"content": fmt.Sprintf("Result of `%s`:\n```\n%s\n```", tc.Name, content),
			})
		}

		log.Debug().Int("step", step).Int("history_len", len(history)).Int("tool_calls", len(toolCalls)).Msg("Agent step complete")
	}

	log.Warn().Int("max_steps", a.maxSteps).Msg("Agent hit step limit")
	return "I hit my step limit trying to finish that — try breaking the request down.", history, nil
}

func buildSystemPrompt(tools []MCPToolDef, orgName, userName string) string {
	if len(tools) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("You are an AI assistant connected to a LiveReview API server. ")
	b.WriteString("You have access to the following tools:\n\n")

	if orgName != "" || userName != "" {
		b.WriteString("The user you are helping belongs to the following context. Use these names (never numeric IDs) when scoping answers and chart descriptions:\n")
		if userName != "" {
			b.WriteString(fmt.Sprintf("- User: %s\n", userName))
		}
		if orgName != "" {
			b.WriteString(fmt.Sprintf("- Organization: %s\n", orgName))
		}
		b.WriteString("\n")
	}

	for _, t := range tools {
		b.WriteString(fmt.Sprintf("- `%s`", t.Name))
		if t.Description != "" {
			b.WriteString(fmt.Sprintf(": %s", t.Description))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n## Critical Instructions — Read Carefully\n")
	b.WriteString("1. You MUST call at least one tool before giving a final answer. Never respond without calling a tool first.\n")
	b.WriteString("2. Never output phrases like 'I cannot', 'I can't', 'I'm unable', 'I cannot directly', 'there is no tool', 'no tool available', 'cannot provide', 'don't have access', 'not designed to'.\n")
	b.WriteString("3. If you cannot find the exact data requested, call the closest available tool and chart whatever data you receive. Then suggest: 'I don't have a direct tool for X, but here's what I can show you:' followed by your chart.\n")
	b.WriteString("4. If no tool is remotely relevant, suggest alternative questions the user CAN ask based on the available tools. For example: 'I can help you explore review data, top reviewers, trends over time, LOC statistics, and more. Try asking about reviews by user, monthly trends, or top contributors.'\n")
	b.WriteString("5. The user would rather see a chart of loosely related data than read an apology. Always produce output.\n\n")

	b.WriteString("## LiveReview Domain Context\n")
	b.WriteString("LiveReview is a code review platform. The key concepts you should understand:\n\n")
	b.WriteString("- **Review**: a code review performed in the system. A review is created by a user and has an author.\n")
	b.WriteString("- **Review fields** (returned by `GET_api_v1_reviews`):\n")
	b.WriteString("  - `id`: review ID\n")
	b.WriteString("  - `authorName`: full name of the user who created/performed the review\n")
	b.WriteString("  - `authorUsername`: username of the reviewer\n")
	b.WriteString("  - `friendlyName`: short name/title of the review\n")
	b.WriteString("  - `aiSummaryTitle`: AI-generated summary title\n")
	b.WriteString("  - `status`: review status\n")
	b.WriteString("  - `createdAt`, `completedAt`: timestamps\n")
	b.WriteString("  - `metadata`: extra info including `ai_connector_name`, `ai_provider_name`, etc.\n\n")
	b.WriteString("- **User / Reviewer**: in this system, a 'user who did code reviews' is the same as the `authorName` or `authorUsername` of review objects.\n")
	b.WriteString("- **Aggregation**: you CAN count, group, sort, and rank review data yourself. For example, to find top reviewers, call `GET_api_v1_reviews`, then count reviews grouped by `authorUsername`, sort by count descending, and return the top N.\n\n")
	b.WriteString("- **Lines of Code (LOC)**:\n")
	b.WriteString("  - If a user asks **'who got the most code reviewed'**, **'most code reviewed'**, or anything about LOC per user/member, they mean ranked by **total LOC reviewed** (billable LOC).\n")
	b.WriteString("  - **Primary tool for LOC per user**: `GET_api_v1_billing_usage_members`. Use this FIRST for user/member LOC rankings.\n")
	b.WriteString("  - **Fallback tool for per-review LOC**: `GET_api_v1_reviews_id_accounting` returns `totalBillableLoc` for a single review.\n")
	b.WriteString("  - **Org summary**: `GET_api_v1_billing_usage_summary` gives org-wide LOC totals.\n")
	b.WriteString("  - If `GET_api_v1_billing_usage_members` returns a permission error, fall back to counting reviews per user via `GET_api_v1_reviews`.\n\n")
	b.WriteString("- **Pagination**: list endpoints like `GET_api_v1_reviews` return paginated results (`page`, `per_page`, `hasNext`, `hasPrevious`).\n")
	b.WriteString("  - For accurate aggregation, request `per_page=200`. If `hasNext: true`, fetch remaining pages.\n")
	b.WriteString("  - NEVER report 'data is partial due to pagination' — fetch remaining pages.\n")
	b.WriteString("  - Use EXACT parameter names from inputSchema. Reviews uses `per_page` (snake_case), not `perPage`.\n\n")

	b.WriteString("Common patterns (use exact parameter names from tool inputSchema):\n")
	b.WriteString("- 'Top reviewers' → `GET_api_v1_reviews` with `per_page=200` → fetch pages → group by `authorUsername` → count → sort descending\n")
	b.WriteString("- 'Reviews per week/month' → `GET_api_v1_reviews` with `per_page=200` → fetch pages → group by week/month → count → chart\n")
	b.WriteString("- 'Review trends' → `GET_api_v1_reviews` with `per_page=200` → fetch pages → sort by `createdAt` → group by time period\n")
	b.WriteString("- 'Top users by LOC' → `GET_api_v1_billing_usage_members` → sort by `total_billable_loc` descending\n")
	b.WriteString("- 'Recent reviews' → `GET_api_v1_reviews` with `per_page=20`\n\n")

	b.WriteString("## How to Call Tools\n")
	b.WriteString("Respond with a JSON code block:\n")
	b.WriteString("```json\n{\"tool\": \"tool_name\", \"arguments\": {...}}\n```\n")
	b.WriteString("For multiple tools:\n")
	b.WriteString("```json\n[{\"tool\": \"tool_a\", \"arguments\": {...}}, {\"tool\": \"tool_b\", \"arguments\": {...}}]\n```\n\n")

	b.WriteString("## Final Response Format\n")
	b.WriteString("When you have all the information needed, respond with one of two formats:\n\n")

	b.WriteString("### Option A: Vega-Lite Chart (MANDATORY for data questions)\n")
	b.WriteString("For ANY question involving numbers, counts, rankings, comparisons, trends, or aggregated data, ")
	b.WriteString("you MUST output a Vega-Lite specification. This is not optional.\n")
	b.WriteString("Do not wait for the user to ask for a chart — if the answer can be visualized, visualize it.\n\n")

	b.WriteString("Single chart format (output WITHOUT json codeblock markers):\n")
	b.WriteString("{\n  \"title\": \"...\",\n  \"subtitle\": \"...\",\n")
	b.WriteString("  \"description\": \"*specific numbers* and insights here\",\n")
	b.WriteString("  \"query\": \"humanized form of the exact query used (state the scope level and filters, e.g. 'review completions across your whole organization over the past six months')\",\n")
	b.WriteString("  \"spec\": {\n    \"$schema\": \"https://vega.github.io/schema/vega-lite/v5.json\",\n")
	b.WriteString("    \"width\": 600, \"height\": 300,\n")
	b.WriteString("    \"data\": { \"values\": [...] },\n")
	b.WriteString("    \"mark\": \"bar\",\n")
	b.WriteString("    \"encoding\": { \"x\": {\"field\": \"...\", \"type\": \"...\"}, \"y\": {\"field\": \"...\", \"type\": \"quantitative\"} }\n")
	b.WriteString("  }\n}\n\n")

	b.WriteString("Multiple charts format:\n")
	b.WriteString("{\n  \"reports\": [\n    {\n      \"title\": \"...\",\n      \"description\": \"...\",\n      \"query\": \"humanized form of the exact query used (state the scope level and filters)\",\n")
	b.WriteString("      \"spec\": { \"$schema\": \"...\", \"width\": 600, \"height\": 300, \"data\": { \"values\": [...] }, \"mark\": \"bar\", \"encoding\": {...} }\n")
	b.WriteString("    }\n  ]\n}\n\n")

	b.WriteString("Vega-Lite rules:\n")
	b.WriteString("- ALWAYS embed data in `data.values` — no external URLs\n")
	b.WriteString("- `width` 600, `height` 300-400\n")
	b.WriteString("- Use `tooltip` for interactivity\n")
	b.WriteString("- Do NOT wrap chart JSON in ```json code block — output raw JSON\n")
	b.WriteString("- Include specific numbers in `description`: totals, averages, top values, comparisons.\n")
	b.WriteString("- Write `description` as SHORT LINES, NEVER as a paragraph. Separate every line with a literal `\\n` (newline) inside the string. Each line is one short sentence or one bullet fragment.\n")
	b.WriteString("- Use ACTIVE voice ONLY. Put the actor (organization, user, or repo) first in every sentence. Never use passive forms like 'were completed', 'was reviewed', 'is shown', 'can be seen'.\n")
	b.WriteString("- HUMANIZE dates: write the month name (e.g. 'February 12, 2026'), never raw '2026-02-12'. Format large numbers readably.\n")
	b.WriteString("- Name the scope: use the organization, user, or repository NAME (never the numeric ID) plus the time range, and say whether the data is org-level, member-level, or repo-level.\n")
	b.WriteString("- Use STE-100 Simplified Technical English: plain, controlled words, one idea per line.\n")
	b.WriteString("- FOLLOW THIS EXAMPLE exactly — short lines joined by `\\n`, and active voice:\n")
	b.WriteString("  \"description\": \"The organization completed 23 reviews in June 2026.\\nThe busiest day was May 27 with 4 reviews.\\nVolume rose 30% from May to June.\"\n")
	b.WriteString("- Always include a `query` field in each chart object: a humanized restatement of the exact query/filters used, naming the scope level and the names (org/user/repo, never IDs) and the time range.\n")
	b.WriteString("- For date/time fields set `\"type\": \"temporal\"` and only use `%`-style time formats (e.g. `\"axis\": {\"format\": \"%Y-%m-%d\"}`) on temporal axes. Never put `%` time formats on ordinal, nominal, or quantitative axes — they break rendering.\n\n")

	b.WriteString("### Option B: Plain Text\n")
	b.WriteString("For simple Q&A with no data to visualize. Use markdown.\n\n")

	b.WriteString("## Summary of Rules\n")
	b.WriteString("- For data questions, you MUST use Option A (Vega-Lite chart).\n")
	b.WriteString("- Always call a tool before responding. Never refuse without calling a tool.\n")
	b.WriteString("- Never say 'I cannot', 'there is no tool', or apologize for lack of tools.\n")
	b.WriteString("- If exact data isn't available, call closest tool and chart what you get, then suggest better queries.\n")
	b.WriteString("- Use exact parameter names from inputSchema (`per_page` not `perPage`).\n")
	b.WriteString("- Always fetch all pages — never report partial data.\n")
	b.WriteString("- Include concrete numbers in descriptions, not just chart titles.\n")

	return b.String()
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

	return calls
}

func truncateContent(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
