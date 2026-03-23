package aisanitize

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	rawHTMLTagPattern = regexp.MustCompile(`(?i)</?[a-z][a-z0-9:-]*(\s[^>]*)?>`)
)

func sanitizeMarkdownForOutput(text string) (string, bool) {
	out := text

	out = rawHTMLTagPattern.ReplaceAllStringFunc(out, func(tag string) string {
		repl := strings.ReplaceAll(tag, "<", "&lt;")
		repl = strings.ReplaceAll(repl, ">", "&gt;")
		return repl
	})

	out = sanitizeMarkdownLinks(out)

	return out, out != text
}

func sanitizeMarkdownLinks(text string) string {
	if text == "" {
		return text
	}

	var b strings.Builder
	b.Grow(len(text))

	i := 0
	for i < len(text) {
		isImage := false
		prefixStart := i
		if text[i] == '!' && i+1 < len(text) && text[i+1] == '[' {
			isImage = true
			prefixStart = i
			i++
		}

		if text[i] != '[' {
			if isImage {
				b.WriteByte('!')
			}
			b.WriteByte(text[i])
			i++
			continue
		}

		labelEnd := findMatchingBracket(text, i)
		if labelEnd == -1 || labelEnd+1 >= len(text) || text[labelEnd+1] != '(' {
			if isImage {
				b.WriteByte('!')
			}
			b.WriteByte(text[i])
			i++
			continue
		}

		targetEnd := findMatchingParen(text, labelEnd+1)
		if targetEnd == -1 {
			if isImage {
				b.WriteByte('!')
			}
			b.WriteByte(text[i])
			i++
			continue
		}

		label := text[i+1 : labelEnd]
		target := text[labelEnd+2 : targetEnd]
		destination := extractMarkdownDestination(target)
		if isSafeMarkdownDestination(destination) {
			b.WriteString(text[prefixStart : targetEnd+1])
		} else if isImage {
			b.WriteString(fmt.Sprintf("![%s](#)", label))
		} else {
			b.WriteString(fmt.Sprintf("[%s](#)", label))
		}

		i = targetEnd + 1
	}

	return b.String()
}

func findMatchingBracket(text string, openIdx int) int {
	if openIdx < 0 || openIdx >= len(text) || text[openIdx] != '[' {
		return -1
	}

	depth := 0
	escaped := false
	for i := openIdx; i < len(text); i++ {
		ch := text[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '[' {
			depth++
			continue
		}
		if ch == ']' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

func findMatchingParen(text string, openIdx int) int {
	if openIdx < 0 || openIdx >= len(text) || text[openIdx] != '(' {
		return -1
	}

	depth := 0
	escaped := false
	inAngle := false
	for i := openIdx; i < len(text); i++ {
		ch := text[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '<' {
			inAngle = true
		}
		if ch == '>' {
			inAngle = false
		}
		if inAngle {
			continue
		}
		if ch == '(' {
			depth++
			continue
		}
		if ch == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

func extractMarkdownDestination(target string) string {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return ""
	}

	if space := strings.IndexAny(trimmed, " \t"); space >= 0 {
		trimmed = trimmed[:space]
	}

	return strings.Trim(trimmed, "<>")
}

func isSafeMarkdownDestination(raw string) bool {
	dest := strings.TrimSpace(raw)
	if dest == "" {
		return false
	}

	if strings.HasPrefix(dest, "#") || strings.HasPrefix(dest, "/") || strings.HasPrefix(dest, "./") || strings.HasPrefix(dest, "../") {
		return true
	}

	parsed, err := url.Parse(dest)
	if err != nil {
		return false
	}

	if parsed.Scheme == "" {
		return true
	}

	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "http", "https", "mailto":
		return true
	default:
		return false
	}
}
