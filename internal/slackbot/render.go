package slackbot

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/slack-go/slack"
)

type renderBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Emoji    string          `json:"emoji,omitempty"`
	Fields   []renderField   `json:"fields,omitempty"`
	Elements []string        `json:"elements,omitempty"`
	Items    []string        `json:"items,omitempty"`
	Blocks   []renderBlock   `json:"blocks,omitempty"`
}

type renderField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type renderPayload struct {
	Blocks []renderBlock `json:"blocks"`
}

// renderStructured converts a JSON block specification from the LLM into Slack Block Kit blocks.
// It tries multiple parsing strategies:
// 1. Try the whole string as raw JSON
// 2. Look for a ```json ... ``` code block and parse its content
func renderStructured(raw string) ([]slack.Block, bool) {
	candidates := []string{raw}

	// Also try extracting from a ```json ... ``` block
	if idx := strings.Index(raw, "```json"); idx >= 0 {
		start := idx + len("```json")
		end := strings.Index(raw[start:], "```")
		if end >= 0 {
			candidates = append(candidates, strings.TrimSpace(raw[start:start+end]))
		}
	}

	for _, c := range candidates {
		var payload renderPayload
		if err := json.Unmarshal([]byte(c), &payload); err != nil || len(payload.Blocks) == 0 {
			continue
		}
		var blocks []slack.Block
		for _, b := range payload.Blocks {
			converted := renderBlockToSlack(b)
			blocks = append(blocks, converted...)
		}
		if len(blocks) > 0 {
			return blocks, true
		}
	}
	return nil, false
}

func renderBlockToSlack(b renderBlock) []slack.Block {
	switch b.Type {
	case "header":
		return []slack.Block{
			slack.NewHeaderBlock(slack.NewTextBlockObject("plain_text", b.Text, false, false)),
		}

	case "divider":
		return []slack.Block{slack.NewDividerBlock()}

	case "section":
		return renderSection(b)

	case "context":
		if len(b.Elements) == 0 && b.Text != "" {
			b.Elements = []string{b.Text}
		}
		var mixed []slack.MixedElement
		for _, e := range b.Elements {
			mixed = append(mixed, slack.NewTextBlockObject("mrkdwn", e, false, false))
		}
		if len(mixed) > 0 {
			return []slack.Block{slack.NewContextBlock("", mixed...)}
		}
		return nil

	case "list":
		if len(b.Items) == 0 {
			return nil
		}
		var sb strings.Builder
		for _, item := range b.Items {
			sb.WriteString("• ")
			sb.WriteString(item)
			sb.WriteString("\n")
		}
		return []slack.Block{
			slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", sb.String(), false, false), nil, nil),
		}

	case "status":
		text := b.Text
		if b.Emoji != "" {
			text = b.Emoji + "  " + text
		}
		return []slack.Block{
			slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", text, false, false), nil, nil),
		}

	case "metric":
		var fields []*slack.TextBlockObject
		for _, f := range b.Fields {
			fields = append(fields, slack.NewTextBlockObject("mrkdwn", "*"+f.Label+"*\n"+f.Value, false, false))
		}
		if len(fields) > 0 {
			return []slack.Block{slack.NewSectionBlock(nil, fields, nil)}
		}
		return nil

	case "quote":
		return []slack.Block{
			slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", ">"+b.Text, false, false), nil, nil),
		}

	default:
		log.Printf("[SlackBot] Unknown renderBlock type %q — skipping", b.Type)
		return nil
	}
}

func renderSection(b renderBlock) []slack.Block {
	text := b.Text
	if b.Emoji != "" {
		text = b.Emoji + "  " + text
	}

	if len(text) > slackMaxTextLen {
		text = text[:slackMaxTextLen] + "…"
	}

	var fields []*slack.TextBlockObject
	for _, f := range b.Fields {
		fields = append(fields, slack.NewTextBlockObject("mrkdwn", "*"+f.Label+"*\n"+f.Value, false, false))
	}

	if text != "" && len(fields) > 0 {
		return []slack.Block{
			slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", text, false, false), fields, nil),
		}
	}
	if text != "" {
		return []slack.Block{
			slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", text, false, false), nil, nil),
		}
	}
	if len(fields) > 0 {
		return []slack.Block{slack.NewSectionBlock(nil, fields, nil)}
	}
	return nil
}
