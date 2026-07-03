package slackbot

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

// FormatSlackResponse converts LLM response text into clean Slack Block Kit blocks.
// If the text is a JSON blocks payload ({"blocks": [...]}), it renders that
// directly. Otherwise it parses markdown-style text into simple, elegant blocks.
func FormatSlackResponse(text string) []slack.Block {
	trimmed := strings.TrimSpace(text)

	if structured, ok := renderStructured(trimmed); ok {
		return structured
	}

	return parseRichText(trimmed)
}

// ---------------------------------------------------------------------------
// Rich text parser — converts markdown-style text into Slack Block Kit blocks

type lineBlock int

const (
	blockUnknown  lineBlock = iota
	blockHeader
	blockDivider
	blockBullet
	blockNumbered
	blockField
	blockQuote
	blockStatus
	blockCodeStart
	blockCodeEnd
	blockCodeContent
	blockParagraph
)

type blockGroup struct {
	kind  lineBlock
	lines []string
}
// ---------------------------------------------------------------------------

var (
	headerRe    = regexp.MustCompile(`^(#{1,3})\s+(.+)$`)
	dividerRe   = regexp.MustCompile(`^[-*_]{3,}$`)
	bulletRe    = regexp.MustCompile(`^[\-\*•]\s+(.+)$`)
	numberedRe  = regexp.MustCompile(`^\d+[.\)]\s+(.+)$`)
	fieldRe     = regexp.MustCompile(`^\*{1,2}(.+?)\*{1,2}:\s*(.+)$`)
	quoteRe     = regexp.MustCompile(`^>\s?(.*)$`)
	statusRe    = regexp.MustCompile(`^([✅🟢🟡🔴❌⚠️🚀🎉📊📈📋🔍✨💡🏆⭐]+)\s*(.*)$`)
)

func parseRichText(text string) []slack.Block {
	lines := strings.Split(text, "\n")
	var blocks []slack.Block

	// Group lines into logical blocks
	var groups []blockGroup

	inCodeBlock := false
	var codeLines []string

	flushCode := func() {
		if len(codeLines) > 0 {
			groups = append(groups, blockGroup{kind: blockCodeContent, lines: codeLines})
			codeLines = nil
		}
	}

	for _, raw := range lines {
		line := raw

		if inCodeBlock {
			if strings.TrimSpace(line) == "```" {
				inCodeBlock = false
				flushCode()
				continue
			}
			codeLines = append(codeLines, line)
			continue
		}

		if strings.TrimSpace(line) == "```" {
			flushCode()
			inCodeBlock = true
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flushCode()
			groups = append(groups, blockGroup{kind: blockDivider})
			continue
		}

		switch {
		case headerRe.MatchString(trimmed):
			flushCode()
			groups = append(groups, blockGroup{kind: blockHeader, lines: []string{trimmed}})

		case dividerRe.MatchString(trimmed):
			flushCode()
			groups = append(groups, blockGroup{kind: blockDivider, lines: []string{"---"}})

		case bulletRe.MatchString(trimmed):
			flushCode()
			groups = append(groups, blockGroup{kind: blockBullet, lines: []string{trimmed}})

		case numberedRe.MatchString(trimmed):
			flushCode()
			groups = append(groups, blockGroup{kind: blockNumbered, lines: []string{trimmed}})

		case statusRe.MatchString(trimmed):
			flushCode()
			groups = append(groups, blockGroup{kind: blockStatus, lines: []string{trimmed}})

		case fieldRe.MatchString(trimmed):
			flushCode()
			groups = append(groups, blockGroup{kind: blockField, lines: []string{trimmed}})

		case quoteRe.MatchString(trimmed):
			flushCode()
			groups = append(groups, blockGroup{kind: blockQuote, lines: []string{trimmed}})

		default:
			flushCode()
			groups = append(groups, blockGroup{kind: blockParagraph, lines: []string{trimmed}})
		}
	}
	flushCode()

	// Merge consecutive groups of the same kind
	var merged []blockGroup
	for _, g := range groups {
		if g.kind == blockDivider {
			merged = append(merged, g)
			continue
		}
		if len(merged) > 0 && merged[len(merged)-1].kind == g.kind {
			merged[len(merged)-1].lines = append(merged[len(merged)-1].lines, g.lines...)
		} else {
			merged = append(merged, g)
		}
	}

	// Render each merged group
	for _, g := range merged {
		rendered := renderGroup(g)
		blocks = append(blocks, rendered...)
	}

	return blocks
}

func renderGroup(g blockGroup) []slack.Block {
	switch g.kind {
	case blockHeader:
		return renderHeader(g.lines[0])
	case blockDivider:
		return []slack.Block{slack.NewDividerBlock()}
	case blockBullet:
		return renderBulletList(g.lines)
	case blockNumbered:
		return renderNumberedList(g.lines)
	case blockField:
		return renderFields(g.lines)
	case blockQuote:
		return renderQuote(g.lines)
	case blockStatus:
		return renderStatus(g.lines)
	case blockCodeContent:
		return renderCodeBlock(g.lines)
	case blockParagraph:
		return renderParagraph(g.lines)
	}
	return nil
}

func renderHeader(line string) []slack.Block {
	m := headerRe.FindStringSubmatch(line)
	if len(m) < 3 {
		return nil
	}
	level := len(m[1])
	text := strings.TrimSpace(m[2])

	if level == 1 {
		return []slack.Block{
			slack.NewHeaderBlock(slack.NewTextBlockObject("plain_text", text, false, false)),
		}
	}
	prefix := ""
	for i := 0; i < level-1; i++ {
		prefix += "▸ "
	}
	return []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*%s%s*", prefix, text), false, false),
			nil, nil,
		),
	}
}

func renderBulletList(lines []string) []slack.Block {
	var items []string
	for _, l := range lines {
		m := bulletRe.FindStringSubmatch(strings.TrimSpace(l))
		if len(m) >= 2 {
			items = append(items, "•  "+m[1])
		} else {
			items = append(items, "•  "+l)
		}
	}
	return []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", strings.Join(items, "\n"), false, false),
			nil, nil,
		),
	}
}

func renderNumberedList(lines []string) []slack.Block {
	var items []string
	for i, l := range lines {
		m := numberedRe.FindStringSubmatch(strings.TrimSpace(l))
		if len(m) >= 2 {
			items = append(items, fmt.Sprintf("%d.  %s", i+1, m[1]))
		} else {
			items = append(items, fmt.Sprintf("%d.  %s", i+1, l))
		}
	}
	return []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", strings.Join(items, "\n"), false, false),
			nil, nil,
		),
	}
}

func renderFields(lines []string) []slack.Block {
	type fieldPair struct {
		label string
		value string
	}
	var fields []fieldPair
	for _, l := range lines {
		m := fieldRe.FindStringSubmatch(strings.TrimSpace(l))
		if len(m) >= 3 {
			fields = append(fields, fieldPair{label: m[1], value: m[2]})
		}
	}

	if len(fields) == 0 {
		return nil
	}

	// Group into pairs for side-by-side display (max 2 per row)
	var blocks []slack.Block
	for i := 0; i < len(fields); i += 2 {
		var row []*slack.TextBlockObject
		row = append(row, slack.NewTextBlockObject("mrkdwn",
			fmt.Sprintf("*%s*\n%s", fields[i].label, fields[i].value), false, false))
		if i+1 < len(fields) {
			row = append(row, slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("*%s*\n%s", fields[i+1].label, fields[i+1].value), false, false))
		}
		blocks = append(blocks, slack.NewSectionBlock(nil, row, nil))
	}
	return blocks
}

func renderQuote(lines []string) []slack.Block {
	var quoted []string
	for _, l := range lines {
		m := quoteRe.FindStringSubmatch(strings.TrimSpace(l))
		if len(m) >= 2 {
			quoted = append(quoted, "> "+m[1])
		} else {
			quoted = append(quoted, "> "+l)
		}
	}
	return []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", strings.Join(quoted, "\n"), false, false),
			nil, nil,
		),
	}
}

func renderStatus(lines []string) []slack.Block {
	var items []string
	for _, l := range lines {
		m := statusRe.FindStringSubmatch(strings.TrimSpace(l))
		if len(m) >= 3 {
			text := m[2]
			if text == "" {
				items = append(items, m[1])
			} else {
				items = append(items, m[1]+"  "+m[2])
			}
		} else {
			items = append(items, l)
		}
	}
	return []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", strings.Join(items, "\n"), false, false),
			nil, nil,
		),
	}
}

func renderCodeBlock(lines []string) []slack.Block {
	code := strings.Join(lines, "\n")
	return []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("```\n%s\n```", code), false, false),
			nil, nil,
		),
	}
}

func renderParagraph(lines []string) []slack.Block {
	text := strings.Join(lines, "\n")
	if text == "" {
		return nil
	}
	const maxLen = 2900
	if len(text) <= maxLen {
		return []slack.Block{
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn", text, false, false),
				nil, nil,
			),
		}
	}
	var blocks []slack.Block
	for _, chunk := range chunkString(text, maxLen) {
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", chunk, false, false),
			nil, nil,
		))
	}
	return blocks
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

func chunkString(s string, size int) []string {
	if len(s) <= size {
		return []string{s}
	}
	var chunks []string
	for len(s) > 0 {
		if len(s) <= size {
			chunks = append(chunks, s)
			break
		}
		cut := strings.LastIndex(s[:size], "\n")
		if cut < 0 {
			cut = size
		}
		chunks = append(chunks, s[:cut])
		s = s[cut:]
	}
	return chunks
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		secs := int(d.Seconds())
		if secs < 1 {
			return "less than a second"
		}
		return fmt.Sprintf("%ds", secs)
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	if mins == 1 {
		return fmt.Sprintf("1 min %ds", secs)
	}
	return fmt.Sprintf("%d mins %ds", mins, secs)
}

