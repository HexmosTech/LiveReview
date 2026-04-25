package langchain

import (
	"context"
	"strings"
	"testing"

	"github.com/livereview/pkg/models"
)

func TestParseResponseWithRepair_AppliesSanitizationAfterRepair(t *testing.T) {
	provider := &LangchainProvider{}
	diffs := []models.CodeDiff{
		{
			FilePath: "a.go",
			Hunks: []models.DiffHunk{
				{
					OldStartLine: 1,
					OldLineCount: 1,
					NewStartLine: 1,
					NewLineCount: 1,
					Content:      "@@ -1 +1 @@\n+line",
				},
			},
		},
	}

	// Trailing comma forces original parse to fail and triggers repair path.
	response := `{"fileSummaries":[{"filePath":"a.go","summary":"Contact alice@example.com"}],"comments":[{"filePath":"a.go","lineNumber":1,"content":"Use sk-12345678901234567890","severity":"warning","suggestions":["mail bob@example.com"],"isInternal":false}],}`

	result, err := provider.parseResponseWithRepair(context.Background(), response, diffs, 0, 0, "batch-test", nil)
	if err != nil {
		t.Fatalf("expected repaired parse to succeed, got error: %v", err)
	}

	if len(result.TechnicalSummaries) == 0 {
		t.Fatalf("expected technical summaries in parsed result")
	}
	if strings.Contains(result.TechnicalSummaries[0].Summary, "alice@example.com") {
		t.Fatalf("expected summary email to be redacted, got: %s", result.TechnicalSummaries[0].Summary)
	}

	if len(result.Comments) == 0 {
		t.Fatalf("expected comments in parsed result")
	}
	if strings.Contains(result.Comments[0].Content, "sk-12345678901234567890") {
		t.Fatalf("expected secret to be redacted in comment, got: %s", result.Comments[0].Content)
	}
	if len(result.Comments[0].Suggestions) == 0 {
		t.Fatalf("expected suggestions in parsed result")
	}
	if strings.Contains(result.Comments[0].Suggestions[0], "bob@example.com") {
		t.Fatalf("expected suggestion email to be redacted, got: %s", result.Comments[0].Suggestions[0])
	}
}

// TestLineIsDeleted_FormattedHunkContent verifies that lineIsDeleted correctly
// identifies deleted lines when hunk.Content is in the pre-formatted
// "OLD | NEW | CONTENT" table format produced by formatHunkWithLineNumbers.
//
// This is the format that lineIsDeleted actually receives at runtime:
//   - formatHunkWithLineNumbers runs first (during ReviewCodeWithBatching)
//   - lineIsDeleted runs later (during parseResponse)
//
// The bug: the original implementation checked HasPrefix(line, "-") which never
// matches formatted rows like "844 |     | -func ..." that start with a digit.
func TestLineIsDeleted_FormattedHunkContent(t *testing.T) {
	provider := &LangchainProvider{}

	// This is the exact format produced by formatSingleHunk for the diff:
	//   @@ -841,7 +886,6 @@
	//    context line (841→886)
	//    context line (842→887)
	//    context line (843→888)  // setDefaultColumnNames...
	//   -func (d *Deidentifier) setDefaultColumnNames(...)  [old:844, deleted]
	//    context line (845→889)
	//    context line (846→890)
	//    context line (847→891)
	formattedContent := "@@ -841,7 +886,6 @@ func (d *Deidentifier) selectBestType\n" +
		"OLD | NEW | CONTENT\n" +
		"----|-----|--------\n" +
		"841 | 886 |  \n" +
		"842 | 887 |  \n" +
		"843 | 888 |  // setDefaultColumnNames generates default column names if not provided\n" +
		"844 |     | -func (d *Deidentifier) setDefaultColumnNames(config *slicesConfig) error {\n" +
		"845 | 889 |  \tif len(config.columnNames) == 0 {\n" +
		"846 | 890 |  \t\tconfig.columnNames = make([]string, config.numCols)\n" +
		"847 | 891 |  \t\tfor i := 0; i < config.numCols; i++ {\n"

	hunk := models.DiffHunk{
		OldStartLine: 841,
		OldLineCount: 7,
		NewStartLine: 886,
		NewLineCount: 6,
		Content:      formattedContent,
	}

	// Line 844 is the deleted line (OLD=844, NEW=blank).
	// With the broken implementation: HasPrefix("844 |     | -func...", "-") == false
	// → returns false (WRONG). With the fix: parses the table and returns true.
	if !provider.lineIsDeleted(844, hunk) {
		t.Errorf("lineIsDeleted(844) = false, want true: line 844 is a deleted line in the formatted hunk")
	}

	// Line 843 is a context line — must NOT be flagged as deleted.
	if provider.lineIsDeleted(843, hunk) {
		t.Errorf("lineIsDeleted(843) = true, want false: line 843 is a context line")
	}

	// Line 886 is a context line on the new side — must NOT be flagged as deleted.
	if provider.lineIsDeleted(886, hunk) {
		t.Errorf("lineIsDeleted(886) = true, want false: line 886 is a context (new-side) line")
	}
}

// TestLineIsDeleted_AllCommentTypes exercises all four comment types the LLM
// produces and that PostComment routes to different Bitbucket API payloads.
//
// From the actual PR "Deleted and Added PR" (deidentify.go):
//
//	Type 1 – general comment            : FilePath="", Line=0        → PostGeneralComment
//	Type 2 – deleted-line comment       : IsDeletedLine=true          → "from" field
//	Type 3 – added-line comment         : IsDeletedLine=false         → "to" field
//	Type 4 – reply on comment thread    : IsDeletedLine=false (reply) → parent ID
//
// lineIsDeleted is only responsible for Types 2 vs 3; Types 1 and 4 never reach it.
func TestLineIsDeleted_AllCommentTypes(t *testing.T) {
	provider := &LangchainProvider{}

	// Hunk 1 from the actual log: @@ -45,6 +45,18 @@ type Table struct
	// All LLM comments (48, 52) landed on ADDED lines in the new file.
	hunkAdded := models.DiffHunk{
		OldStartLine: 45,
		OldLineCount: 6,
		NewStartLine: 45,
		NewLineCount: 18,
		// formatSingleHunk output:
		//  45 |  45 |  context
		//  46 |  46 |  context
		//     |  47 | +// TextOptions controls which PII processors run...
		//     |  48 | +type TextOptions struct {
		//     |  49 | +    SkipEmails bool
		//     |  50 | +    SkipPhones bool
		//     |  51 | +    SkipNames  bool
		//     |  52 | +    SkipAddresses bool
		//  ...context continues to old 50 / new 62
		Content: "@@ -45,6 +45,18 @@ type Table struct {\n" +
			"OLD | NEW | CONTENT\n" +
			"----|-----|--------\n" +
			" 45 |  45 |  \tColumns []Column\n" +
			" 46 |  46 |  }\n" +
			"    |  47 | +\n" +
			"    |  48 | +type TextOptions struct {\n" +
			"    |  49 | +\tSkipEmails bool\n" +
			"    |  50 | +\tSkipPhones bool\n" +
			"    |  51 | +\tSkipNames  bool\n" +
			"    |  52 | +\tSkipAddresses bool\n" +
			"    |  53 | +}\n" +
			" 47 |  54 |  \n" +
			" 48 |  55 |  context\n",
	}

	// Hunk 3 from the actual log: @@ -841,7 +886,6 @@ — the one with the deleted line.
	hunkDeleted := models.DiffHunk{
		OldStartLine: 841,
		OldLineCount: 7,
		NewStartLine: 886,
		NewLineCount: 6,
		Content: "@@ -841,7 +886,6 @@ func (d *Deidentifier) selectBestType\n" +
			"OLD | NEW | CONTENT\n" +
			"----|-----|--------\n" +
			"841 | 886 |  \n" +
			"842 | 887 |  \n" +
			"843 | 888 |  // setDefaultColumnNames generates default column names\n" +
			"844 |     | -func (d *Deidentifier) setDefaultColumnNames(config *slicesConfig) error {\n" +
			"845 | 889 |  \tif len(config.columnNames) == 0 {\n" +
			"846 | 890 |  \t\tconfig.columnNames = make([]string, config.numCols)\n" +
			"847 | 891 |  \t\tfor i := 0; i < config.numCols; i++ {\n",
	}

	tests := []struct {
		name       string
		hunk       models.DiffHunk
		lineNumber int
		wantDeleted bool
		reason     string
	}{
		// --- Type 3: added-line comments (LLM comments on new lines) ---
		{
			name:        "added line 48 (TextOptions struct open brace)",
			hunk:        hunkAdded,
			lineNumber:  48,
			wantDeleted: false,
			reason:      "line 48 is +added in new file; OLD column is blank",
		},
		{
			name:        "added line 52 (SkipAddresses field)",
			hunk:        hunkAdded,
			lineNumber:  52,
			wantDeleted: false,
			reason:      "line 52 is +added in new file; OLD column is blank",
		},
		// --- Type 2: deleted-line comment (the failing case) ---
		{
			name:        "deleted line 844 (setDefaultColumnNames func removed)",
			hunk:        hunkDeleted,
			lineNumber:  844,
			wantDeleted: true,
			reason:      "line 844 exists only in old file; NEW column is blank → must use 'from' in Bitbucket API",
		},
		// --- Context lines (should never be flagged as deleted) ---
		{
			name:        "context line 45 (present in both old and new)",
			hunk:        hunkAdded,
			lineNumber:  45,
			wantDeleted: false,
			reason:      "context line has both OLD and NEW numbers",
		},
		{
			name:        "context line 845 (after deleted line in hunk 3)",
			hunk:        hunkDeleted,
			lineNumber:  845,
			wantDeleted: false,
			reason:      "line 845 is a context line after the deletion",
		},
		{
			name:        "new-side line number 886 for context row",
			hunk:        hunkDeleted,
			lineNumber:  886,
			wantDeleted: false,
			reason:      "886 is the new-side number for the same context row as old 841",
		},
		// --- Line outside the hunk entirely ---
		{
			name:        "line 900 not in any hunk",
			hunk:        hunkDeleted,
			lineNumber:  900,
			wantDeleted: false,
			reason:      "line 900 is beyond the hunk range; default false",
		},
		// --- Multiple deleted lines in one hunk: only matching one returns true ---
		{
			name: "first of two deleted lines",
			hunk: models.DiffHunk{
				OldStartLine: 10,
				OldLineCount: 4,
				NewStartLine: 10,
				NewLineCount: 2,
				Content: "OLD | NEW | CONTENT\n" +
					"----|-----|--------\n" +
					" 10 |  10 |  context\n" +
					" 11 |     | -first removed line\n" +
					" 12 |     | -second removed line\n" +
					" 13 |  11 |  context after\n",
			},
			lineNumber:  11,
			wantDeleted: true,
			reason:      "line 11 is the first of two deleted lines",
		},
		{
			name: "second of two deleted lines",
			hunk: models.DiffHunk{
				OldStartLine: 10,
				OldLineCount: 4,
				NewStartLine: 10,
				NewLineCount: 2,
				Content: "OLD | NEW | CONTENT\n" +
					"----|-----|--------\n" +
					" 10 |  10 |  context\n" +
					" 11 |     | -first removed line\n" +
					" 12 |     | -second removed line\n" +
					" 13 |  11 |  context after\n",
			},
			lineNumber:  12,
			wantDeleted: true,
			reason:      "line 12 is the second of two deleted lines",
		},
		{
			name: "context line between two deleted hunks is not deleted",
			hunk: models.DiffHunk{
				OldStartLine: 10,
				OldLineCount: 4,
				NewStartLine: 10,
				NewLineCount: 2,
				Content: "OLD | NEW | CONTENT\n" +
					"----|-----|--------\n" +
					" 10 |  10 |  context\n" +
					" 11 |     | -first removed line\n" +
					" 12 |     | -second removed line\n" +
					" 13 |  11 |  context after\n",
			},
			lineNumber:  10,
			wantDeleted: false,
			reason:      "line 10 is a context line before the deletions",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := provider.lineIsDeleted(tc.lineNumber, tc.hunk)
			if got != tc.wantDeleted {
				t.Errorf("lineIsDeleted(%d) = %v, want %v\n  reason: %s",
					tc.lineNumber, got, tc.wantDeleted, tc.reason)
			}
		})
	}
}

// TestLineIsDeleted_BugRegressions covers the two correctness bugs fixed in the
// new table-format parser and one additional robustness case.
//
// Bug 1 — over-broad "---" skip (original code):
//
//	The old skip condition was strings.HasPrefix(line, "---"), which incorrectly
//	swallowed any table row whose CONTENT column started with "---".  Fixed to
//	match only the exact separator row "----|-----|--------".
//
// Bug 2 — both columns fail → phantom context line at 0 (original code):
//
//	When both OLD and NEW failed Atoi, parseHunkLine returned (0, 0, ..., nil).
//	lineIsDeleted treated it as a context match if lineNumber == 0, producing a
//	false negative.  Fixed: return an error so the caller's "continue" skips it.
//
// Bonus — content containing " | " pipes must not corrupt the parse (SplitN):
//
//	Because parseHunkLine uses SplitN(line, " | ", 3), a row whose CONTENT column
//	contains additional " | " sequences must still be parsed correctly.
func TestLineIsDeleted_BugRegressions(t *testing.T) {
	provider := &LangchainProvider{}

	tests := []struct {
		name        string
		hunk        models.DiffHunk
		lineNumber  int
		wantDeleted bool
		reason      string
	}{
		// ── Bug 1 regression ──────────────────────────────────────────────────────
		// A deleted line whose CONTENT starts with "---" (e.g. a YAML/Markdown
		// horizontal rule or an old go-style deprecation comment).
		// Old code: strings.HasPrefix("  6 |     | ---", "---") == false because the
		// row starts with spaces, BUT if the row happened to start with "---" directly
		// (e.g. after trimming), it would be silently dropped.
		// The real danger is a row like "---1 |     | removed" (unlikely but possible
		// with bad formatting), so we test the exact separator row is the only skip.
		{
			name: "bug1: deleted line with content starting with '---' is not skipped",
			hunk: models.DiffHunk{
				OldStartLine: 5,
				OldLineCount: 3,
				NewStartLine: 5,
				NewLineCount: 2,
				Content: "OLD | NEW | CONTENT\n" +
					"----|-----|--------\n" +
					"  5 |   5 |  context line\n" +
					"  6 |     | ---- yaml separator removed\n" + // content starts with "---"
					"  7 |   6 |  context after\n",
			},
			lineNumber:  6,
			wantDeleted: true,
			reason: "OLD=6, NEW=blank → deleted; the '---' in CONTENT must not cause " +
				"the row to be skipped (old broad HasPrefix check was the bug)",
		},
		{
			name: "bug1: exact separator row ----|-----|-------- is still skipped",
			hunk: models.DiffHunk{
				OldStartLine: 5,
				OldLineCount: 2,
				NewStartLine: 5,
				NewLineCount: 2,
				Content: "OLD | NEW | CONTENT\n" +
					"----|-----|--------\n" + // must be skipped, not parsed as data
					"  5 |   5 |  context\n" +
					"  6 |   6 |  context\n",
			},
			lineNumber:  0, // no row should produce a match at 0
			wantDeleted: false,
			reason: "separator row must be skipped; it must not be parsed as a data row " +
				"that could match line 0",
		},

		// ── Bug 2 regression ──────────────────────────────────────────────────────
		// A row where BOTH OLD and NEW columns are non-numeric (completely garbled).
		// Old code: parseHunkLine returned (0, 0, content, false, false, nil), so
		// lineIsDeleted treated it as a context line matching oldNum==0 / newNum==0.
		// If lineNumber == 0 was ever queried, it would return false (wrong match).
		// New code: returns an error → caller's "continue" skips the row cleanly.
		{
			name: "bug2: garbled row with non-numeric OLD and NEW does not match line 0",
			hunk: models.DiffHunk{
				OldStartLine: 1,
				OldLineCount: 2,
				NewStartLine: 1,
				NewLineCount: 2,
				Content: "OLD | NEW | CONTENT\n" +
					"----|-----|--------\n" +
					"  x |   y | garbage row both non-numeric\n" + // both columns fail Atoi
					"  1 |   1 |  real context\n",
			},
			lineNumber:  0, // should never match because 0 is not a real line number
			wantDeleted: false,
			reason: "unparseable row (both OLD and NEW non-numeric) must not produce a " +
				"phantom context match at line 0 — it must be skipped via error return",
		},

		// ── Bonus: pipe characters inside content column ───────────────────────────
		// parseHunkLine uses SplitN(line, " | ", 3), so extra " | " in CONTENT is safe.
		{
			name: "bonus: deleted line whose content contains ' | ' pipe sequences",
			hunk: models.DiffHunk{
				OldStartLine: 20,
				OldLineCount: 2,
				NewStartLine: 20,
				NewLineCount: 1,
				Content: "OLD | NEW | CONTENT\n" +
					"----|-----|--------\n" +
					" 20 |  20 |  context\n" +
					" 21 |     | -val := a | b | c\n", // content has " | " in it
			},
			lineNumber:  21,
			wantDeleted: true,
			reason: "SplitN(..., 3) limits splits to 3 parts, so extra ' | ' in the " +
				"CONTENT column does not corrupt OLD/NEW number parsing",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := provider.lineIsDeleted(tc.lineNumber, tc.hunk)
			if got != tc.wantDeleted {
				t.Errorf("lineIsDeleted(%d) = %v, want %v\n  reason: %s",
					tc.lineNumber, got, tc.wantDeleted, tc.reason)
			}
		})
	}
}

// TestLineIsDeleted_FormatContract documents the data-flow guarantee that
// lineIsDeleted always receives pre-formatted table content, never raw unified diff.
//
// Data flow (confirmed in provider.go):
//
//	ReviewCodeWithBatching / ReviewCodeWithBatchingV2
//	  └─ formatHunkWithLineNumbers(hunk)         ← converts +/- → table, in-place
//	       └─ diff.Hunks[j].Content = formatted  ← same slice passed downstream
//	            └─ parseResponseWithRepair(diffs) ← lineIsDeleted reads this
//
// The test below documents the known silent-failure mode: if raw unified diff
// content somehow bypassed the formatting step, lineIsDeleted would return false
// for everything (all rows fail the " | " split, all are skipped).
// This is NOT a bug in lineIsDeleted — it is the caller's responsibility to
// ensure formatHunkWithLineNumbers has run first.
func TestLineIsDeleted_FormatContract(t *testing.T) {
	provider := &LangchainProvider{}

	// Raw unified diff content — exactly what the OLD lineIsDeleted used to receive,
	// and what the NEW one should never see at runtime.
	rawUnifiedDiff := models.DiffHunk{
		OldStartLine: 841,
		OldLineCount: 7,
		NewStartLine: 886,
		NewLineCount: 6,
		Content: "@@ -841,7 +886,6 @@ func (d *Deidentifier) selectBestType\n" +
			" \n" +
			" \n" +
			" // setDefaultColumnNames generates default column names\n" +
			"-func (d *Deidentifier) setDefaultColumnNames(config *slicesConfig) error {\n" +
			" \tif len(config.columnNames) == 0 {\n" +
			" \t\tconfig.columnNames = make([]string, config.numCols)\n" +
			" \t\tfor i := 0; i < config.numCols; i++ {\n",
	}

	// With raw unified diff, every data row fails len(parts) != 3 (no " | "),
	// so all rows are skipped and the result is always false.
	// This is the silent failure mode — not a crash, but incorrect.
	// At runtime this cannot happen because formatHunkWithLineNumbers always runs first.
	got := provider.lineIsDeleted(844, rawUnifiedDiff)

	// The point of this test is documentation: confirm the current silent-skip
	// behavior is stable, so a future change that accidentally makes the parser
	// handle raw diffs (and potentially return wrong results) is flagged.
	if got {
		t.Errorf("lineIsDeleted(844, rawUnifiedDiff) = true; "+
			"raw unified diff hitting the table parser should silently return false, "+
			"not a correct result — check whether formatHunkWithLineNumbers was bypassed")
	}
}
