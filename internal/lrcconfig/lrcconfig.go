// Package lrcconfig implements LiveReview's server-side enforcement of a
// repository's .lrc/ Repository Rules: concatenating .lrc/rules/*.md into a
// single instruction bundle for the AI prompt, and filtering reviewed diffs
// against .lrc/ignore.
//
// The .lrc/ tree arrives as part of the diff-review zip (see
// internal/api/diff_review.go), already extracted into a Bundle keyed by
// path relative to .lrc/ (e.g. "rules/design.md", "ignore"). git-lrc's
// internal/lrcrules package implements the same BuildRulesBundle
// concatenation rule for local, offline `lrc config check`/`preview`.
package lrcconfig

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/livereview/cmd/mrmodel/lib"
	gitignore "github.com/sabhiram/go-gitignore"
)

// CharLimit is the maximum size, in bytes (UTF-8), of the concatenated rules
// bundle injected into the AI prompt. It is measured via len() on the bundle
// text, matching git-lrc's internal/lrcrules.CharLimit, so multi-byte
// characters count for more than one toward the limit. Bundles exceeding
// this are truncated (with a warning), never causing the review to fail.
const CharLimit = 3000

const rulesPrefix = "rules/"
const rulesReadmePath = rulesPrefix + "README.md"
const rulesInstructionsPath = rulesPrefix + "INSTRUCTIONS.md"
const ignorePath = "ignore"

// Issue describes a problem found while processing a Bundle.
type Issue struct {
	Level   string // "error" | "warning"
	Path    string
	Message string
}

// Bundle holds the raw contents of a repository's .lrc/ directory, keyed by
// path relative to .lrc/ (e.g. "rules/design.md", "ignore").
type Bundle struct {
	Files map[string][]byte
}

// BuildRulesBundle concatenates rules/*.md (direct children only),
// excluding rules/README.md and skipping empty/whitespace-only files.
// rules/INSTRUCTIONS.md, if present and non-empty, is placed first as the
// entry point; every other file follows in lexicographic order. Each
// included file is preceded by a "## rules/<name>.md" header. Returns the
// concatenated text, its character count, and a warning-level Issue if the
// result exceeds CharLimit. Exceeding CharLimit never fails the review here
// (see CharLimit) — callers truncate the text and surface the warning;
// git-lrc's internal/lrcrules package treats the same condition as an error
// for its offline `lrc config check`, where failing fast is appropriate.
func BuildRulesBundle(b Bundle) (string, int, []Issue) {
	var names []string
	hasInstructions := false
	for path := range b.Files {
		if path == rulesReadmePath {
			continue
		}
		if !strings.HasPrefix(path, rulesPrefix) || !strings.HasSuffix(path, ".md") {
			continue
		}
		if strings.Contains(strings.TrimPrefix(path, rulesPrefix), "/") {
			continue // skip nested directories, only direct children of rules/
		}
		if path == rulesInstructionsPath {
			hasInstructions = true
			continue
		}
		names = append(names, path)
	}
	sort.Strings(names)
	if hasInstructions {
		names = append([]string{rulesInstructionsPath}, names...)
	}

	var out strings.Builder
	for _, path := range names {
		trimmed := strings.TrimSpace(string(b.Files[path]))
		if trimmed == "" {
			continue
		}
		if out.Len() > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString("## ")
		out.WriteString(path)
		out.WriteString("\n\n")
		out.WriteString(trimmed)
	}

	text := out.String()
	charCount := len(text)

	var issues []Issue
	if charCount > CharLimit {
		issues = append(issues, Issue{
			Level:   "warning",
			Path:    "rules",
			Message: fmt.Sprintf("concatenated rules bundle is %d characters, exceeding the %d character limit and will be truncated", charCount, CharLimit),
		})
	}

	return text, charCount, issues
}

// LoadIgnorePatterns parses .lrc/ignore (gitignore syntax). Returns nil
// patterns (with no issues) when the ignore file is absent or empty.
func LoadIgnorePatterns(b Bundle) ([]string, []Issue) {
	data, ok := b.Files[ignorePath]
	if !ok {
		return nil, nil
	}

	var patterns []string
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(rawLine, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		patterns = append(patterns, line)
	}

	return patterns, nil
}

// FilterDiffs drops diffs whose NewPath (or OldPath, for deletions) matches
// an ignore pattern. Returns the kept diffs and the paths excluded.
func FilterDiffs(diffs []lib.LocalCodeDiff, patterns []string) ([]lib.LocalCodeDiff, []string) {
	if len(patterns) == 0 {
		return diffs, nil
	}

	matcher := gitignore.CompileIgnoreLines(patterns...)

	kept := make([]lib.LocalCodeDiff, 0, len(diffs))
	var excluded []string
	for _, d := range diffs {
		path := d.NewPath
		if strings.TrimSpace(path) == "" {
			path = d.OldPath
		}
		if matcher.MatchesPath(path) {
			excluded = append(excluded, path)
			continue
		}
		kept = append(kept, d)
	}

	return kept, excluded
}

// TruncateAtLineBoundary truncates text to at most limit bytes, breaking at
// the last newline before the limit so headers/sections aren't cut mid-line.
// limit is a byte count (UTF-8), matching CharLimit. If no newline is found
// before the limit, the cut point is moved back to the nearest UTF-8 rune
// boundary so the result is never invalid UTF-8.
func TruncateAtLineBoundary(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := strings.LastIndex(text[:limit], "\n")
	if cut <= 0 {
		for limit > 0 && !utf8.RuneStart(text[limit]) {
			limit--
		}
		return text[:limit]
	}
	return text[:cut]
}
