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

	"github.com/pelletier/go-toml/v2"
	"github.com/livereview/cmd/mrmodel/lib"
	"github.com/livereview/pkg/models"
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

// FilterCodeDiffs is the []*models.CodeDiff counterpart of FilterDiffs for
// webhook-triggered reviews where changes are fetched via provider API (not
// from a CLI-uploaded zip). Matching files are dropped; excluded paths are
// returned so callers can log them.
func FilterCodeDiffs(diffs []*models.CodeDiff, patterns []string) ([]*models.CodeDiff, []string) {
	if len(patterns) == 0 {
		return diffs, nil
	}

	matcher := gitignore.CompileIgnoreLines(patterns...)

	kept := make([]*models.CodeDiff, 0, len(diffs))
	var excluded []string
	for _, d := range diffs {
		if d == nil {
			continue
		}
		path := d.FilePath
		if strings.TrimSpace(path) == "" {
			path = d.OldFilePath
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

const policyToolsTomlPath = "policy/tools.toml"
const toolsTomlPath = "tools.toml"

// ToolRuleConfig represents per-tool configuration within policy/tools.toml or tools.toml
type ToolRuleConfig struct {
	Enabled  *bool    `toml:"enabled"`
	Category string   `toml:"category"`
	Include  []string `toml:"include"`
	Exclude  []string `toml:"exclude"`
}

// ParseToolRuleConfigs reads .lrc/policy/tools.toml or .lrc/tools.toml from a Bundle.
// Returns a map of tool_name -> *ToolRuleConfig.
func ParseToolRuleConfigs(b Bundle) (map[string]*ToolRuleConfig, error) {
	data, ok := b.Files[policyToolsTomlPath]
	if !ok || len(data) == 0 {
		data, ok = b.Files[toolsTomlPath]
	}
	if !ok || len(data) == 0 {
		return nil, nil
	}

	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse tools.toml: %w", err)
	}

	results := make(map[string]*ToolRuleConfig)

	processEntry := func(toolName string, val interface{}) {
		toolName = strings.ToLower(toolName)
		switch v := val.(type) {
		case bool:
			bVal := v
			results[toolName] = &ToolRuleConfig{Enabled: &bVal}
		case map[string]interface{}:
			cfg := &ToolRuleConfig{}
			if enabledVal, ok := v["enabled"].(bool); ok {
				cfg.Enabled = &enabledVal
			}
			if catVal, ok := v["category"].(string); ok {
				cfg.Category = catVal
			}
			if incSlice, ok := v["include"].([]interface{}); ok {
				for _, item := range incSlice {
					if str, isStr := item.(string); isStr {
						cfg.Include = append(cfg.Include, str)
					}
				}
			}
			if excSlice, ok := v["exclude"].([]interface{}); ok {
				for _, item := range excSlice {
					if str, isStr := item.(string); isStr {
						cfg.Exclude = append(cfg.Exclude, str)
					}
				}
			}
			results[toolName] = cfg
		}
	}

	for k, v := range raw {
		if k == "tools" {
			if toolsMap, ok := v.(map[string]interface{}); ok {
				for tName, tVal := range toolsMap {
					if _, exists := results[strings.ToLower(tName)]; !exists {
						processEntry(tName, tVal)
					}
				}
			}
		} else {
			processEntry(k, v)
		}
	}

	return results, nil
}

// ShouldRunToolRuleForDiff determines whether a tool should run against the given local diffs based on ToolRuleConfig.
func ShouldRunToolRuleForDiff(cfg *ToolRuleConfig, diffs []lib.LocalCodeDiff) bool {
	if cfg == nil {
		return true
	}

	if cfg.Enabled != nil && !*cfg.Enabled {
		return false
	}

	if len(diffs) == 0 {
		return true
	}

	var includeMatcher *gitignore.GitIgnore
	if len(cfg.Include) > 0 {
		includeMatcher = gitignore.CompileIgnoreLines(cfg.Include...)
	}

	var excludeMatcher *gitignore.GitIgnore
	if len(cfg.Exclude) > 0 {
		excludeMatcher = gitignore.CompileIgnoreLines(cfg.Exclude...)
	}

	if includeMatcher == nil && excludeMatcher == nil {
		return true
	}

	matchingFilesCount := 0
	for _, d := range diffs {
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		if path == "" {
			continue
		}

		if excludeMatcher != nil && excludeMatcher.MatchesPath(path) {
			continue
		}

		if includeMatcher != nil {
			if includeMatcher.MatchesPath(path) {
				matchingFilesCount++
			}
		} else {
			matchingFilesCount++
		}
	}

	return matchingFilesCount > 0
}

// FilterLocalCodeDiffsForTool filters local code diffs according to a tool's ToolRuleConfig.
// It returns a new slice containing only the diffs for file paths that match inclusion and pass exclusion rules.
func FilterLocalCodeDiffsForTool(cfg *ToolRuleConfig, diffs []lib.LocalCodeDiff) []lib.LocalCodeDiff {
	if cfg == nil || len(diffs) == 0 {
		return diffs
	}

	var includeMatcher *gitignore.GitIgnore
	if len(cfg.Include) > 0 {
		includeMatcher = gitignore.CompileIgnoreLines(cfg.Include...)
	}

	var excludeMatcher *gitignore.GitIgnore
	if len(cfg.Exclude) > 0 {
		excludeMatcher = gitignore.CompileIgnoreLines(cfg.Exclude...)
	}

	if includeMatcher == nil && excludeMatcher == nil {
		return diffs
	}

	var filtered []lib.LocalCodeDiff
	for _, d := range diffs {
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		if path == "" {
			continue
		}

		if excludeMatcher != nil && excludeMatcher.MatchesPath(path) {
			continue
		}

		if includeMatcher != nil {
			if includeMatcher.MatchesPath(path) {
				filtered = append(filtered, d)
			}
		} else {
			filtered = append(filtered, d)
		}
	}

	return filtered
}

// FormatLocalDiffs converts a slice of lib.LocalCodeDiff into a standard unified diff string.
func FormatLocalDiffs(diffs []lib.LocalCodeDiff) string {
	var b strings.Builder
	for _, d := range diffs {
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		if path == "" {
			continue
		}

		b.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", path, path))
		if d.OldPath == "/dev/null" || d.OldPath == "" {
			b.WriteString("new file mode 100644\n")
		} else if d.NewPath == "/dev/null" || d.NewPath == "" {
			b.WriteString("deleted file mode 100644\n")
		}
		for _, hunk := range d.Hunks {
			b.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@", hunk.OldStartLine, hunk.OldLineCount, hunk.NewStartLine, hunk.NewLineCount))
			if hunk.HeaderText != "" {
				b.WriteString(" " + hunk.HeaderText)
			}
			b.WriteString("\n")
			for _, line := range hunk.Lines {
				prefix := " "
				if line.LineType == "added" {
					prefix = "+"
				} else if line.LineType == "deleted" {
					prefix = "-"
				}
				b.WriteString(prefix + line.Content + "\n")
			}
		}
	}
	return b.String()
}
