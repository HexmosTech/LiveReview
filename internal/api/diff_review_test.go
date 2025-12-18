package api

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/livereview/cmd/mrmodel/lib"
	"github.com/livereview/pkg/models"
)

func TestConvertLocalToModelDiffUsesOldPathWhenNewEmpty(t *testing.T) {
	local := lib.LocalCodeDiff{
		OldPath: "old/foo.go",
		NewPath: " ",
		Hunks: []lib.LocalDiffHunk{
			{
				OldStartLine: 1,
				OldLineCount: 1,
				NewStartLine: 2,
				NewLineCount: 1,
				Lines:        []lib.LocalDiffLine{{Content: "line", LineType: "context", OldLineNo: 1, NewLineNo: 2}},
			},
		},
	}

	result := convertLocalToModelDiff(local)

	if result.FilePath != "old/foo.go" {
		t.Fatalf("expected FilePath to fallback to old path, got %s", result.FilePath)
	}
	if result.FileType != ".go" {
		t.Fatalf("expected FileType .go, got %s", result.FileType)
	}
	if len(result.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(result.Hunks))
	}
}

func TestConvertLocalHunkFormatting(t *testing.T) {
	hunk := lib.LocalDiffHunk{
		OldStartLine: 10,
		OldLineCount: 2,
		NewStartLine: 12,
		NewLineCount: 3,
		HeaderText:   "func foo",
		Lines: []lib.LocalDiffLine{
			{Content: "line one", LineType: "context", OldLineNo: 10, NewLineNo: 12},
			{Content: "added line", LineType: "added", OldLineNo: 0, NewLineNo: 13},
			{Content: "old line", LineType: "deleted", OldLineNo: 11, NewLineNo: 0},
		},
	}

	result := convertLocalHunk(hunk)

	expected := "@@ -10,2 +12,3 @@ func foo\n line one\n+added line\n-old line"
	if result.Content != expected {
		t.Fatalf("unexpected hunk content:\nexpected:\n%s\ngot:\n%s", expected, result.Content)
	}
}

func TestLineWithinHunks(t *testing.T) {
	hunks := []models.DiffHunk{{NewStartLine: 10, NewLineCount: 3}}

	cases := map[int]bool{
		9:  false,
		10: true,
		12: true,
		13: true,
		14: false,
	}

	for line, expected := range cases {
		if got := lineWithinHunks(line, hunks); got != expected {
			t.Fatalf("line %d expected %t got %t", line, expected, got)
		}
	}
}

func TestFilterCommentsForFile(t *testing.T) {
	hunks := []models.DiffHunk{{NewStartLine: 5, NewLineCount: 2}}
	comments := []*models.ReviewComment{
		{FilePath: "file.go", Line: 5, Content: "a", Severity: models.SeverityWarning, Category: "style"},
		{FilePath: "file.go", Line: 8, Content: "b", Severity: models.SeverityInfo, Category: "out"},
		{FilePath: "other.go", Line: 5, Content: "c", Severity: models.SeverityCritical, Category: "other"},
	}

	filtered := filterCommentsForFile("file.go", hunks, comments)

	if len(filtered) != 2 {
		t.Fatalf("expected 2 comments for file.go, got %d", len(filtered))
	}
	if filtered[0]["line"].(int) != 5 {
		t.Fatalf("expected first comment line 5, got %v", filtered[0]["line"])
	}
	if filtered[1]["line"].(int) != 8 {
		t.Fatalf("expected second comment line 8, got %v", filtered[1]["line"])
	}
}

func TestBuildDiffFiles(t *testing.T) {
	diffs := []models.CodeDiff{
		{
			FilePath: "file.go",
			Hunks: []models.DiffHunk{
				{OldStartLine: 1, OldLineCount: 1, NewStartLine: 2, NewLineCount: 2, Content: "@@ -1,1 +2,2 @@\n+line"},
			},
		},
	}

	comments := []*models.ReviewComment{
		{FilePath: "file.go", Line: 2, Content: "note", Severity: models.SeverityInfo, Category: "nit"},
	}

	files := buildDiffFiles(diffs, comments)

	if len(files) != 1 {
		t.Fatalf("expected 1 file entry, got %d", len(files))
	}

	entry := files[0]
	if entry["file_path"].(string) != "file.go" {
		t.Fatalf("expected file.go, got %v", entry["file_path"])
	}

	hunks, ok := entry["hunks"].([]map[string]interface{})
	if !ok || len(hunks) != 1 {
		t.Fatalf("expected 1 hunk map, got %v", entry["hunks"])
	}

	cmts, ok := entry["comments"].([]map[string]interface{})
	if !ok || len(cmts) != 1 {
		t.Fatalf("expected 1 comment map, got %v", entry["comments"])
	}
	if cmts[0]["content"].(string) != "note" {
		t.Fatalf("expected comment content note, got %v", cmts[0]["content"])
	}
}

func TestDiffReviewContractExample(t *testing.T) {
	// Build a tiny unified diff and wrap it in a zip, exactly like the client sends.
	diff := "diff --git a/foo.txt b/foo.txt\n" +
		"--- a/foo.txt\n" +
		"+++ b/foo.txt\n" +
		"@@ -0,0 +1,2 @@\n" +
		"+hello\n" +
		"+world\n"

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("patch.diff")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	if _, err := w.Write([]byte(diff)); err != nil {
		t.Fatalf("failed to write diff: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

	// Parse the payload using the same helper the handler uses.
	localDiffs, err := parseDiffZipBase64(encoded)
	if err != nil {
		t.Fatalf("parseDiffZipBase64 failed: %v", err)
	}
	if len(localDiffs) != 1 {
		t.Fatalf("expected 1 local diff, got %d", len(localDiffs))
	}

	modelDiffs := convertLocalDiffs(localDiffs)
	comments := []*models.ReviewComment{{
		FilePath: modelDiffs[0].FilePath,
		Line:     modelDiffs[0].Hunks[0].NewStartLine,
		Content:  "Example review note",
		Severity: models.SeverityInfo,
		Category: "example",
	}}

	response := map[string]interface{}{
		"status":  "completed",
		"summary": "Example summary for contract test",
		"files":   buildDiffFiles(modelsSlice(modelDiffs), comments),
	}

	// Assert core contract fields.
	files := response["files"].([]map[string]interface{})
	if len(files) != 1 {
		t.Fatalf("expected 1 file in response, got %d", len(files))
	}
	if files[0]["file_path"].(string) != "foo.txt" {
		t.Fatalf("expected file_path foo.txt, got %s", files[0]["file_path"])
	}

	// Print the request/response contract to make it obvious and human-readable.
	pretty, _ := json.MarshalIndent(response, "", "  ")
	t.Logf("Example client payload (base64 zip length=%d): %s", len(encoded), encoded)
	t.Logf("Example handler response:\n%s", string(pretty))
}

// modelsSlice converts []*models.CodeDiff to []models.CodeDiff for helper compatibility.
func modelsSlice(in []*models.CodeDiff) []models.CodeDiff {
	out := make([]models.CodeDiff, 0, len(in))
	for _, d := range in {
		if d != nil {
			out = append(out, *d)
		}
	}
	return out
}
