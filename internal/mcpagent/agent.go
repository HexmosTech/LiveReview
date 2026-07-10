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
	systemPrompt := buildSystemPrompt(mcpSession.Tools)

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

func buildSystemPrompt(tools []MCPToolDef) string {
	if len(tools) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("You are an AI assistant connected to a LiveReview API server. ")
	b.WriteString("You have access to the following tools:\n\n")

	for _, t := range tools {
		b.WriteString(fmt.Sprintf("- `%s`", t.Name))
		if t.Description != "" {
			b.WriteString(fmt.Sprintf(": %s", t.Description))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n## LiveReview Domain Context\n")
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
	b.WriteString("  - **Primary tool for LOC per user**: `GET_api_v1_billing_usage_members`. It returns members with `total_billable_loc` directly. Use this FIRST for user/member LOC rankings.\n")
	b.WriteString("  - **Fallback tool for per-review LOC**: `GET_api_v1_reviews_id_accounting` returns `totalBillableLoc` for a single review. Use it if you need to cross-reference reviews with their LOC.\n")
	b.WriteString("  - **Org summary**: `GET_api_v1_billing_usage_summary` gives org-wide LOC totals.\n")
	b.WriteString("  - If `GET_api_v1_billing_usage_members` returns a permission error, fall back to counting reviews per user via `GET_api_v1_reviews` and explain that LOC data requires billing access.\n\n")
	b.WriteString("- **Pagination**: list endpoints like `GET_api_v1_reviews` return paginated results (`page`, `per_page`, `hasNext`, `hasPrevious`).\n")
	b.WriteString("  - Default is often 20 items per page.\n")
	b.WriteString("  - For accurate aggregation or full data, request `per_page=200` to get a good batch in one call.\n")
	b.WriteString("  - If you see `hasNext: true`, request the next page with `page=2` (and `page=3`, etc.) until all data is collected.\n")
	b.WriteString("  - NEVER report 'data is partial due to pagination' — instead, actually fetch the remaining page(s). You have enough steps.\n")
	b.WriteString("  - IMPORTANT: Use EXACT parameter names from the tool's inputSchema. Reviews uses `per_page` (snake_case), not `perPage`.\n\n")

	b.WriteString("Common patterns (use exact parameter names from tool inputSchema — `per_page` not `perPage`):\n")
	b.WriteString("- 'Top reviewers by review count' → `GET_api_v1_reviews` with `per_page=200` → if more exist, fetch pages → group by `authorUsername` → count → sort descending\n")
	b.WriteString("- 'Reviews per user' → `GET_api_v1_reviews` with `per_page=200` → if more exist, fetch pages → group by `authorUsername` → count → sort descending\n")
	b.WriteString("- 'Reviews per week/month' → `GET_api_v1_reviews` with `per_page=200` → if more exist, fetch pages → group by week/month → count → chart\n")
	b.WriteString("- 'Review trends' / 'activity over time' → `GET_api_v1_reviews` with `per_page=200` → if more exist, fetch pages → sort by `createdAt` → group by time period\n")
	b.WriteString("- 'Who got the most code reviewed' / 'Top users by LOC' → `GET_api_v1_billing_usage_members` → sort by `total_billable_loc` descending\n")
	b.WriteString("- 'LOC per review' → `GET_api_v1_reviews` → for each review call `GET_api_v1_reviews_id_accounting` → read `totalBillableLoc`\n")
	b.WriteString("- 'Recent reviews' → `GET_api_v1_reviews` with `per_page=20` → sort by `createdAt` descending\n\n")

	b.WriteString("## Calling Tools\n")
	b.WriteString("When you need to call a tool, respond with a JSON code block like this:\n")
	b.WriteString("```json\n{\"tool\": \"tool_name\", \"arguments\": {...}}\n```\n")
	b.WriteString("To call multiple tools, use multiple JSON blocks or a JSON array:\n")
	b.WriteString("```json\n[{\"tool\": \"tool_a\", \"arguments\": {...}}, {\"tool\": \"tool_b\", \"arguments\": {...}}]\n```\n")
	b.WriteString("After you get the results, continue the conversation.\n\n")

	b.WriteString("## Structuring Your Final Answer\n")
	b.WriteString("When you have all the information needed, respond with one of two formats:\n\n")

	b.WriteString("### Option A: Vega-Lite Chart Report (Recommended for data/charts)\n")
	b.WriteString("For ANY question involving numbers, counts, rankings, comparisons, trends, or aggregated data, ")
	b.WriteString("ALWAYS output a Vega-Lite specification. It will be rendered as a PNG image and sent to Slack.\n")
	b.WriteString("Do not wait for the user to explicitly ask for a chart — if the answer can be visualized, visualize it.\n\n")

	b.WriteString("#### Single Chart\n")
	b.WriteString("Use this wrapped format for a single chart:\n\n")
	b.WriteString("```json\n{\n  \"title\": \"Monthly Review Volume\",\n  \"subtitle\": \"Reviews completed per month\",\n  \"description\": \"*27 reviews* in Mar, up from 19 in Feb and 12 in Jan. Overall trend: increasing.\",\n")
	b.WriteString("  \"spec\": {\n")
	b.WriteString("    \"$schema\": \"https://vega.github.io/schema/vega-lite/v5.json\",\n")
	b.WriteString("    \"description\": \"Monthly review volume\",\n")
	b.WriteString("    \"width\": 600,\n    \"height\": 300,\n")
	b.WriteString("    \"data\": {\n      \"values\": [\n        {\"month\": \"Jan\", \"reviews\": 12},\n")
	b.WriteString("        {\"month\": \"Feb\", \"reviews\": 19},\n        {\"month\": \"Mar\", \"reviews\": 27}\n      ]\n    },\n")
	b.WriteString("    \"mark\": \"bar\",\n")
	b.WriteString("    \"encoding\": {\n      \"x\": {\"field\": \"month\", \"type\": \"ordinal\"},\n")
	b.WriteString("      \"y\": {\"field\": \"reviews\", \"type\": \"quantitative\"}\n")
	b.WriteString("    }\n  }\n}\n```\n\n")
	b.WriteString("- **`description` is critical**: it is rendered as text alongside the image in Slack.\n")
	b.WriteString("- Write *specific, data-driven descriptions*: include actual numbers (totals, averages, top values), trends, and comparisons.\n")
	b.WriteString("- Bad (vague): 'This chart shows review activity.'\n")
	b.WriteString("- Good (specific): '*42 reviews* total. Alice led with *15 reviews*, followed by Bob with *12*. March saw the highest activity with *27 reviews*.'\n\n")

	b.WriteString("#### Multiple Charts\n")
	b.WriteString("If a prompt asks for multiple comparisons or data that is best shown in separate charts, ")
	b.WriteString("output a `reports` array. Each report is rendered as its own PNG image:\n\n")
	b.WriteString("```json\n{\n  \"reports\": [\n    {\n      \"title\": \"Reviews by User\",\n      \"description\": \"*Top reviewers* by count of reviews performed.\",\n")
	b.WriteString("      \"spec\": { \"$schema\": \"...\", \"width\": 600, \"height\": 300, \"data\": { \"values\": [...] }, \"mark\": \"bar\", \"encoding\": {...} }\n")
	b.WriteString("    },\n    {\n      \"title\": \"Reviews by Month\",\n      \"description\": \"*Monthly trend* of review completion.\",\n")
	b.WriteString("      \"spec\": { \"$schema\": \"...\", \"width\": 600, \"height\": 300, \"data\": { \"values\": [...] }, \"mark\": \"line\", \"encoding\": {...} }\n")
	b.WriteString("    }\n  ]\n}\n```\n\n")
	b.WriteString("Each report in the array gets its own `title`, `description` (Slack mrkdwn text), and `spec`.\n\n")

	b.WriteString("Rules for Vega-Lite:\n")
	b.WriteString("- ALWAYS wrap it in the title/subtitle/description/spec format (or reports array for multiple)\n")
	b.WriteString("- ALWAYS embed data in the `data.values` array — do not reference external URLs\n")
	b.WriteString("- Set `width` to 600 and `height` to 300-400 for good Slack display\n")
	b.WriteString("- Use clean, readable marks: `bar`, `line`, `area`, `point`, `arc` (pie), `rect` (heatmap)\n")
	b.WriteString("- Use `color` encoding ONLY for categorical fields (e.g. `{\"field\": \"status\", \"type\": \"nominal\"}`)\n")
	b.WriteString("- Do NOT hardcode color values like `{\"value\": \"#2563EB\"}` — a consistent theme is applied automatically\n")
	b.WriteString("- Use `tooltip` for interactivity\n")
	b.WriteString("- Do NOT put the JSON inside a ```json code block — output it raw\n\n")

	b.WriteString("### Option B: Slack Text Blocks\n")
	b.WriteString("For simple Q&A or non-chart summaries, use Slack mrkdwn-formatted text.\n")
	b.WriteString("Use *bold* headings, bullet lists, `code` for inline values, and `>quotes` for callouts.\n\n")

	b.WriteString("General rules:\n")
	b.WriteString("- For ANY question involving numbers, counts, rankings, comparisons, trends, or aggregated data, use Option A (Vega-Lite image report) by default.\n")
	b.WriteString("- Only use Option B for purely textual/simple Q&A with no data to visualize.\n")
	b.WriteString("- You can and should aggregate, count, sort, and rank data returned by tools. Always provide *specific numbers* in descriptions.\n")
	b.WriteString("- Always request `per_page=200` first. If you see `hasNext: true`, fetch subsequent pages by incrementing `page`.\n")
	b.WriteString("- NEVER say 'data is partial due to pagination' — that is a bug. Always fetch all remaining pages to get complete data.\n")
	b.WriteString("- Use EXACT parameter names from each tool's inputSchema. Reviews uses `per_page` (snake_case), billing uses `total_billable_loc`.\n")
	b.WriteString("- Do not call the same tool repeatedly with the same arguments.\n")
	b.WriteString("- In descriptions, include concrete numbers: totals, averages, top values, comparisons. Not just chart titles.\n")

	return b.String()
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

