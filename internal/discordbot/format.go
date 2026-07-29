package discordbot

import (
	"fmt"
	"regexp"
	"strings"
)

const discordMaxMessageLen = 2000

func FormatDiscordResponse(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "🤖 (empty response)"
	}

	return parseRichText(trimmed)
}

type lineKind int

const (
	lineUnknown  lineKind = iota
	lineHeader
	lineDivider
	lineBullet
	lineNumbered
	lineField
	lineQuote
	lineCode
	lineParagraph
)

var (
	headerRe   = regexp.MustCompile(`^(#{1,3})\s+(.+)$`)
	dividerRe  = regexp.MustCompile(`^[-*_]{3,}$`)
	bulletRe   = regexp.MustCompile(`^[\-\*•]\s+(.+)$`)
	numberedRe = regexp.MustCompile(`^\d+[.\)]\s+(.+)$`)
	fieldRe    = regexp.MustCompile(`^\*{1,2}(.+?)\*{1,2}:\s*(.+)$`)
	quoteRe    = regexp.MustCompile(`^>\s?(.*)$`)
)

func parseRichText(text string) string {
	lines := strings.Split(text, "\n")
	var sb strings.Builder
	inCode := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			if inCode {
				sb.WriteString("```\n")
				inCode = false
			} else {
				sb.WriteString("```\n")
				inCode = true
			}
			continue
		}

		if inCode {
			sb.WriteString(trimmed + "\n")
			continue
		}

		switch {
		case headerRe.MatchString(trimmed):
			m := headerRe.FindStringSubmatch(trimmed)
			level := len(m[1])
			htext := strings.TrimSpace(m[2])
			if level == 1 {
				sb.WriteString(fmt.Sprintf("**%s**\n\n", htext))
			} else {
				prefix := strings.Repeat("▸ ", level-1)
				sb.WriteString(fmt.Sprintf("**%s%s**\n\n", prefix, htext))
			}

		case dividerRe.MatchString(trimmed):
			sb.WriteString("─────────────\n\n")

		case bulletRe.MatchString(trimmed):
			m := bulletRe.FindStringSubmatch(trimmed)
			sb.WriteString(fmt.Sprintf("• %s\n", m[1]))

		case numberedRe.MatchString(trimmed):
			m := numberedRe.FindStringSubmatch(trimmed)
			sb.WriteString(fmt.Sprintf("%s\n", m[1]))

		case fieldRe.MatchString(trimmed):
			m := fieldRe.FindStringSubmatch(trimmed)
			sb.WriteString(fmt.Sprintf("**%s:** %s\n", m[1], m[2]))

		case quoteRe.MatchString(trimmed):
			m := quoteRe.FindStringSubmatch(trimmed)
			sb.WriteString(fmt.Sprintf("> %s\n", m[1]))

		case trimmed == "":
			sb.WriteString("\n")

		default:
			sb.WriteString(trimmed + "\n")
		}
	}

	result := strings.TrimSpace(sb.String())
	if result == "" {
		return "🤖 (no response)"
	}
	return result
}
