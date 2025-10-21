package reviewmodel

import (
	"fmt"
	"strconv"
	"strings"
)

// AnnotateUnifiedDiffHunk adds pseudo line numbers to a single unified diff hunk for readability.
// It parses the @@ -a,b +c,d @@ header and then increments counts for context/added/removed lines.
func AnnotateUnifiedDiffHunk(hunk string) string {
	lines := strings.Split(hunk, "\n")
	if len(lines) == 0 {
		return hunk
	}

	var out []string
	var oldN, newN int
	headerParsed := false

	for i, ln := range lines {
		if i == 0 && strings.HasPrefix(ln, "@@ ") {
			oldN, newN = parseDiffHeaderLineNumbers(ln)
			out = append(out, ln)
			headerParsed = true
			continue
		}

		if !headerParsed {
			out = append(out, ln)
			continue
		}

		if ln == "" {
			out = append(out, ln)
			continue
		}

		switch ln[0] {
		case ' ':
			out = append(out, fmt.Sprintf("%6d %6d | %s", oldN, newN, ln))
			oldN++
			newN++
		case '+':
			out = append(out, fmt.Sprintf("%6s %6d | %s", "-", newN, ln))
			newN++
		case '-':
			out = append(out, fmt.Sprintf("%6d %6s | %s", oldN, "-", ln))
			oldN++
		default:
			out = append(out, ln)
		}
	}

	return strings.Join(out, "\n")
}

// ExtractHunkForLine selects the unified diff hunk containing the target old/new line.
func ExtractHunkForLine(patch string, targetOld, targetNew int) string {
	lines := strings.Split(patch, "\n")
	var out []string
	var cur []string
	var newStart, newCount int
	var oldStart, oldCount int

	flush := func() {
		if len(cur) == 0 {
			return
		}
		if hunkContainsLine(oldStart, oldCount, targetOld) || hunkContainsLine(newStart, newCount, targetNew) {
			out = append(out, cur...)
		}
		cur = nil
	}

	for _, ln := range lines {
		if strings.HasPrefix(ln, "@@ ") && strings.Contains(ln, "@@") {
			flush()
			cur = []string{ln}
			oldStart, oldCount, newStart, newCount = parseDiffHeader(ln)
			continue
		}
		if cur != nil {
			cur = append(cur, ln)
		}
	}

	flush()

	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n")
}

// RenderCodeExcerptWithLineNumbers renders a window of lines around focusLine with line numbers.
func RenderCodeExcerptWithLineNumbers(content string, focusLine, radius int) string {
	if radius <= 0 {
		radius = 6
	}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || focusLine <= 0 {
		return ""
	}

	start := focusLine - radius
	if start < 1 {
		start = 1
	}
	end := focusLine + radius
	if end > len(lines) {
		end = len(lines)
	}

	var b strings.Builder
	b.WriteString("```\n")
	for i := start; i <= end; i++ {
		fmt.Fprintf(&b, "%5d | %s\n", i, lines[i-1])
	}
	b.WriteString("```\n")
	return b.String()
}

func hunkContainsLine(start, count, target int) bool {
	if start == 0 || target == 0 {
		return false
	}
	end := start + count - 1
	return target >= start && target <= end
}

func parseDiffHeader(header string) (oldStart, oldCount, newStart, newCount int) {
	oldStart, oldCount = parseSegment(header, '-')
	newStart, newCount = parseSegment(header, '+')
	return
}

func parseDiffHeaderLineNumbers(header string) (oldStart, newStart int) {
	oldStart, _ = parseSegment(header, '-')
	newStart, _ = parseSegment(header, '+')
	return
}

func parseSegment(header string, marker byte) (start, count int) {
	segmentIdx := strings.IndexByte(header, marker)
	if segmentIdx == -1 {
		return 0, 0
	}
	segment := header[segmentIdx+1:]
	if ws := strings.IndexByte(segment, ' '); ws >= 0 {
		segment = segment[:ws]
	}
	parts := strings.Split(segment, ",")
	if len(parts) > 0 {
		start = atoi(parts[0])
	}
	if len(parts) > 1 {
		count = atoi(parts[1])
	} else {
		count = 1
	}
	return
}

func atoi(value string) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return n
}
