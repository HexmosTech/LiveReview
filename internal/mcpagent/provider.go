package mcpagent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/livereview/internal/aiconnectors"
	"github.com/rs/zerolog/log"
	"github.com/tmc/langchaingo/llms"
)

type Provider struct {
	connector *aiconnectors.Connector
}

// TokenUsage carries per-call token counts pulled from the provider's raw
// response. Providers vary in which GenerationInfo keys they populate
// (OpenAI: PromptTokens/CompletionTokens, Anthropic: InputTokens/OutputTokens,
// Google AI: input_tokens/output_tokens), so extraction checks all of them.
// Zero means the provider did not report usage, not that usage was zero.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
}

func firstInt(gi map[string]any, keys ...string) int {
	for _, k := range keys {
		v, ok := gi[k]
		if !ok || v == nil {
			continue
		}
		switch n := v.(type) {
		case int:
			return n
		case int32:
			return int(n)
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return 0
}

func extractTokenUsage(gi map[string]any) TokenUsage {
	if gi == nil {
		return TokenUsage{}
	}
	return TokenUsage{
		InputTokens:  firstInt(gi, "InputTokens", "PromptTokens", "input_tokens", "prompt_tokens"),
		OutputTokens: firstInt(gi, "OutputTokens", "CompletionTokens", "output_tokens", "completion_tokens"),
	}
}

func NewProvider(connector *aiconnectors.Connector) *Provider {
	return &Provider{connector: connector}
}

// Describe returns a short "provider/model" string for logging.
func (p *Provider) Describe() string {
	if p == nil || p.connector == nil {
		return "unknown"
	}
	return fmt.Sprintf("%s/%s", p.connector.GetProvider(), p.connector.GetModel())
}

// FormatTools converts MCP tool definitions into langchaingo tool schemas.
func (p *Provider) FormatTools(tools []MCPToolDef) []llms.Tool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]llms.Tool, len(tools))
	for i, t := range tools {
		schema := map[string]any{}
		if t.InputSchema != nil {
			schemaBytes, err := json.Marshal(t.InputSchema)
			if err == nil {
				json.Unmarshal(schemaBytes, &schema)
			}
		}

		result[i] = llms.Tool{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  schema,
			},
		}
	}
	return result
}

// Complete sends the conversation to the LLM and returns the response text
// plus the token usage the provider reported for this call.
// Tool calls from the LLM (via WithTools) are converted to ReAct JSON blocks
// embedded in the returned text so the agent can parse them.
// extraOpts lets a specific caller (e.g. classify's WithJSONMode) add call
// options without changing every other caller's behavior.
func (p *Provider) Complete(ctx context.Context, history []HistoryEntry, tools []llms.Tool, extraOpts ...llms.CallOption) (string, TokenUsage, error) {
	messages := p.historyToMessages(history)

	var opts []llms.CallOption
	if len(tools) > 0 {
		opts = append(opts, llms.WithTools(tools))
	}
	opts = append(opts, extraOpts...)

	resp, err := p.connector.GenerateContent(ctx, messages, opts...)
	if err != nil {
		return "", TokenUsage{}, err
	}

	if len(resp.Choices) == 0 {
		return "", TokenUsage{}, fmt.Errorf("no choices in LLM response")
	}

	choice := resp.Choices[0]
	usage := extractTokenUsage(choice.GenerationInfo)

	if len(choice.ToolCalls) > 0 {
		// Convert structured tool calls to ReAct JSON block
		text := ""
		for _, tc := range choice.ToolCalls {
			if tc.FunctionCall == nil {
				continue
			}
			if text != "" {
				text += "\n"
			}
			block := fmt.Sprintf("```json\n{\"tool\": \"%s\", \"arguments\": %s}\n```",
				tc.FunctionCall.Name, tc.FunctionCall.Arguments)
			text += block
		}
		log.Debug().Str("text", text).Msg("LLM returned tool calls, converted to ReAct block")
		return text, usage, nil
	}

	return choice.Content, usage, nil
}

// historyToMessages converts generic history entries to langchaingo MessageContent.
// Uses only text-based roles: system, user, assistant.
// Tool calls and results are embedded as text in the conversation.
func (p *Provider) historyToMessages(history []HistoryEntry) []llms.MessageContent {
	var messages []llms.MessageContent
	for _, entry := range history {
		role, ok := entry["role"].(string)
		if !ok {
			continue
		}

		switch role {
		case "system":
			content := ""
			if c, ok := entry["content"].(string); ok {
				content = c
			}
			messages = append(messages, llms.MessageContent{
				Role:  llms.ChatMessageTypeSystem,
				Parts: []llms.ContentPart{llms.TextContent{Text: content}},
			})

		case "user":
			content := ""
			if c, ok := entry["content"].(string); ok {
				content = c
			}
			messages = append(messages, llms.MessageContent{
				Role:  llms.ChatMessageTypeHuman,
				Parts: []llms.ContentPart{llms.TextContent{Text: content}},
			})

		case "assistant":
			text := ""
			if t, ok := entry["text"].(string); ok {
				text = t
			}
			messages = append(messages, llms.MessageContent{
				Role:  llms.ChatMessageTypeAI,
				Parts: []llms.ContentPart{llms.TextContent{Text: text}},
			})
		}
	}
	return messages
}
