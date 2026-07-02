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

func NewProvider(connector *aiconnectors.Connector) *Provider {
	return &Provider{connector: connector}
}

// FormatTools converts MCP tool definitions into langchaingo tool schemas.
func (p *Provider) FormatTools(tools []MCPToolDef) []llms.Tool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]llms.Tool, len(tools))
	for i, t := range tools {
		schemaBytes, _ := json.Marshal(t.InputSchema)
		var schema map[string]any
		if err := json.Unmarshal(schemaBytes, &schema); err != nil {
			schema = map[string]any{}
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

// Complete sends the conversation to the LLM and returns the response text.
// Tool calls from the LLM (via WithTools) are converted to ReAct JSON blocks
// embedded in the returned text so the agent can parse them.
func (p *Provider) Complete(ctx context.Context, history []HistoryEntry, tools []llms.Tool) (string, error) {
	messages := p.historyToMessages(history)

	var opts []llms.CallOption
	if len(tools) > 0 {
		opts = append(opts, llms.WithTools(tools))
	}

	resp, err := p.connector.GenerateContent(ctx, messages, opts...)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices in LLM response")
	}

	choice := resp.Choices[0]

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
		return text, nil
	}

	return choice.Content, nil
}

// historyToMessages converts generic history entries to langchaingo MessageContent.
// Uses only text-based roles: system, user, assistant.
// Tool calls and results are embedded as text in the conversation.
func (p *Provider) historyToMessages(history []HistoryEntry) []llms.MessageContent {
	var messages []llms.MessageContent
	for _, entry := range history {
		role, _ := entry["role"].(string)

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
