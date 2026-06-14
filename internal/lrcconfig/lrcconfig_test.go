package lrcconfig

import (
	"strings"
	"testing"

	"github.com/livereview/cmd/mrmodel/lib"
)

func TestBuildRulesBundle(t *testing.T) {
	b := Bundle{Files: map[string][]byte{
		"rules/README.md":     []byte("should be excluded"),
		"rules/design.md":     []byte("  Use hexagonal architecture.  "),
		"rules/empty.md":      []byte("   \n  "),
		"rules/security.md":   []byte("No secrets in logs."),
		"rules/sub/nested.md": []byte("should be ignored (nested)"),
		"ignore":              []byte("*.log"),
	}}

	text, charCount, issues := BuildRulesBundle(b)
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %v", issues)
	}

	want := "## rules/design.md\n\nUse hexagonal architecture.\n\n## rules/security.md\n\nNo secrets in logs."
	if text != want {
		t.Fatalf("unexpected bundle text:\ngot:  %q\nwant: %q", text, want)
	}
	if charCount != len(want) {
		t.Fatalf("charCount = %d, want %d", charCount, len(want))
	}
}

func TestBuildRulesBundleEmpty(t *testing.T) {
	text, charCount, issues := BuildRulesBundle(Bundle{})
	if text != "" || charCount != 0 || issues != nil {
		t.Fatalf("expected empty result for empty bundle, got text=%q charCount=%d issues=%v", text, charCount, issues)
	}
}

func TestBuildRulesBundleOverLimit(t *testing.T) {
	b := Bundle{Files: map[string][]byte{
		"rules/design.md": []byte(strings.Repeat("x", CharLimit+100)),
	}}

	_, charCount, issues := BuildRulesBundle(b)
	if charCount <= CharLimit {
		t.Fatalf("expected charCount > %d, got %d", CharLimit, charCount)
	}

	found := false
	for _, issue := range issues {
		if issue.Level == "warning" && issue.Path == "rules" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a warning issue for exceeding the char limit, got %v", issues)
	}
}

func TestLoadIgnorePatterns(t *testing.T) {
	b := Bundle{Files: map[string][]byte{
		"ignore": []byte("# comment\n\nnode_modules/\n*.log\n!important.log\n"),
	}}

	patterns, issues := LoadIgnorePatterns(b)
	if issues != nil {
		t.Fatalf("unexpected issues: %v", issues)
	}

	want := []string{"node_modules/", "*.log", "!important.log"}
	if len(patterns) != len(want) {
		t.Fatalf("patterns = %v, want %v", patterns, want)
	}
	for i := range want {
		if patterns[i] != want[i] {
			t.Fatalf("patterns[%d] = %q, want %q", i, patterns[i], want[i])
		}
	}
}

func TestLoadIgnorePatternsMissing(t *testing.T) {
	patterns, issues := LoadIgnorePatterns(Bundle{})
	if patterns != nil || issues != nil {
		t.Fatalf("expected nil/nil for missing ignore file, got patterns=%v issues=%v", patterns, issues)
	}
}

func TestFilterDiffs(t *testing.T) {
	diffs := []lib.LocalCodeDiff{
		{NewPath: "src/main.go"},
		{NewPath: "vendor/lib/thing.go"},
		{OldPath: "deleted.log", NewPath: ""},
		{NewPath: "node_modules/pkg/index.js"},
	}

	patterns := []string{"vendor/", "*.log", "node_modules/"}
	kept, excluded := FilterDiffs(diffs, patterns)

	if len(kept) != 1 || kept[0].NewPath != "src/main.go" {
		t.Fatalf("kept = %v, want only src/main.go", kept)
	}

	wantExcluded := []string{"vendor/lib/thing.go", "deleted.log", "node_modules/pkg/index.js"}
	if len(excluded) != len(wantExcluded) {
		t.Fatalf("excluded = %v, want %v", excluded, wantExcluded)
	}
}

func TestFilterDiffsNoPatterns(t *testing.T) {
	diffs := []lib.LocalCodeDiff{{NewPath: "src/main.go"}}
	kept, excluded := FilterDiffs(diffs, nil)
	if len(kept) != 1 || excluded != nil {
		t.Fatalf("expected diffs unchanged with no patterns, got kept=%v excluded=%v", kept, excluded)
	}
}

// TestFilterDiffsNegationPattern verifies that a later "!pattern" re-includes
// a file excluded by an earlier pattern, per gitignore semantics. This
// matters for billing: a negated file must remain in both billableLOC and
// the AI input.
func TestFilterDiffsNegationPattern(t *testing.T) {
	diffs := []lib.LocalCodeDiff{
		{NewPath: "debug.log"},
		{NewPath: "important.log"},
	}

	patterns := []string{"*.log", "!important.log"}
	kept, excluded := FilterDiffs(diffs, patterns)

	if len(kept) != 1 || kept[0].NewPath != "important.log" {
		t.Fatalf("kept = %v, want only important.log", kept)
	}
	if len(excluded) != 1 || excluded[0] != "debug.log" {
		t.Fatalf("excluded = %v, want only debug.log", excluded)
	}
}

// TestFilterDiffsAnchoredPattern verifies that a leading-slash pattern only
// matches at the repo root, not in nested directories.
func TestFilterDiffsAnchoredPattern(t *testing.T) {
	diffs := []lib.LocalCodeDiff{
		{NewPath: "build/output.bin"},
		{NewPath: "src/build/output.bin"},
	}

	patterns := []string{"/build"}
	kept, excluded := FilterDiffs(diffs, patterns)

	if len(kept) != 1 || kept[0].NewPath != "src/build/output.bin" {
		t.Fatalf("kept = %v, want only src/build/output.bin", kept)
	}
	if len(excluded) != 1 || excluded[0] != "build/output.bin" {
		t.Fatalf("excluded = %v, want only build/output.bin", excluded)
	}
}

// TestFilterDiffsMalformedPattern verifies that a malformed/garbage ignore
// pattern (e.g. an unterminated character class) is ignored rather than
// causing a panic or affecting other diffs.
func TestFilterDiffsMalformedPattern(t *testing.T) {
	diffs := []lib.LocalCodeDiff{
		{NewPath: "src/main.go"},
		{NewPath: "debug.log"},
	}

	patterns := []string{"[[[unterminated", "*.log", ""}
	kept, excluded := FilterDiffs(diffs, patterns)

	if len(kept) != 1 || kept[0].NewPath != "src/main.go" {
		t.Fatalf("kept = %v, want only src/main.go", kept)
	}
	if len(excluded) != 1 || excluded[0] != "debug.log" {
		t.Fatalf("excluded = %v, want only debug.log", excluded)
	}
}

func TestTruncateAtLineBoundary(t *testing.T) {
	text := "## rules/a.md\n\nfirst section\n\n## rules/b.md\n\nsecond section"

	got := TruncateAtLineBoundary(text, len("## rules/a.md\n\nfirst section\n\n## rules/b.md"))
	want := "## rules/a.md\n\nfirst section\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTruncateAtLineBoundaryUnderLimit(t *testing.T) {
	text := "short text"
	if got := TruncateAtLineBoundary(text, 100); got != text {
		t.Fatalf("got %q, want %q", got, text)
	}
}
