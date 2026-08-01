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

		if match := headerRe.FindStringSubmatch(trimmed); len(match) > 0 {
			level := len(match[1])
			htext := strings.TrimSpace(match[2])
			if level == 1 {
				sb.WriteString(fmt.Sprintf("**%s**\n\n", htext))
			} else {
				prefix := strings.Repeat("▸ ", level-1)
				sb.WriteString(fmt.Sprintf("**%s%s**\n\n", prefix, htext))
			}
		} else if dividerRe.MatchString(trimmed) {
			sb.WriteString("─────────────\n\n")
		} else if match := bulletRe.FindStringSubmatch(trimmed); len(match) > 0 {
			sb.WriteString(fmt.Sprintf("• %s\n", match[1]))
		} else if match := numberedRe.FindStringSubmatch(trimmed); len(match) > 0 {
			// Preserve the original numbering (e.g. "1." or "2)") rather than
			// dropping it and rendering a bare list.
			sb.WriteString(match[0] + "\n")
		} else if match := fieldRe.FindStringSubmatch(trimmed); len(match) > 0 {
			sb.WriteString(fmt.Sprintf("**%s:** %s\n", match[1], match[2]))
		} else if match := quoteRe.FindStringSubmatch(trimmed); len(match) > 0 {
			sb.WriteString(fmt.Sprintf("> %s\n", match[1]))
		} else if trimmed == "" {
			sb.WriteString("\n")
		} else {
			sb.WriteString(trimmed + "\n")
		}
	}

	result := strings.TrimSpace(sb.String())
	if result == "" {
		return "🤖 (no response)"
	}
	return result
}
