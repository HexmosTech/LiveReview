package slackbot

import (
	"strings"
	"testing"

	"github.com/slack-go/slack"
)

// TestFormatSlackResponse_ConvertsMarkdownBold guards against the literal
// "**text**" showing up unrendered in Slack (Slack's mrkdwn bold syntax is
// single asterisks), which is what every "**Label:**"-style LLM response
// looked like before toSlackMrkdwn was introduced.
func TestFormatSlackResponse_ConvertsMarkdownBold(t *testing.T) {
	input := "Hi! Here's how to add a **Git Provider**:\n\n1. **Prepare a Bot User:** create a dedicated account.\n2. **Grant Permissions:** add access.\n\n- **Note:** double check this.\n\n## **Important** heading"

	blocks := FormatSlackResponse(input)
	if len(blocks) == 0 {
		t.Fatal("expected at least one block")
	}

	var all strings.Builder
	for _, b := range blocks {
		sb, ok := b.(*slack.SectionBlock)
		if !ok || sb.Text == nil {
			continue
		}
		all.WriteString(sb.Text.Text)
		all.WriteString("\n")
	}
	rendered := all.String()

	if strings.Contains(rendered, "**") {
		t.Errorf("rendered Slack text still contains literal '**' (unconverted markdown bold):\n%s", rendered)
	}
	if !strings.Contains(rendered, "*Git Provider*") {
		t.Errorf("expected '*Git Provider*' (Slack bold) in rendered text, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "*Prepare a Bot User:*") {
		t.Errorf("expected '*Prepare a Bot User:*' in rendered text, got:\n%s", rendered)
	}
}

// TestToSlackMrkdwn_BoldSpanContainingDelimiter guards against a regression
// where the conversion regex's character class excluded the delimiter (* or
// _) from appearing anywhere inside the bolded span, so "**foo*bar**" or
// "__foo_bar__" failed to match at all and were left as literal, unconverted
// Markdown.
func TestToSlackMrkdwn_BoldSpanContainingDelimiter(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"star inside star-bold", "**use *args here**", "*use *args here*"},
		{"underscore inside underscore-bold", "__my_var__", "*my_var*"},
		{"two separate bold spans", "**alpha** and **beta**", "*alpha* and *beta*"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toSlackMrkdwn(tc.input)
			if strings.Contains(got, "**") || strings.Contains(got, "__") {
				t.Errorf("toSlackMrkdwn(%q) = %q, still contains an unconverted Markdown bold delimiter", tc.input, got)
			}
			if got != tc.want {
				t.Errorf("toSlackMrkdwn(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
