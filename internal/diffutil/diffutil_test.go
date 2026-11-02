package diffutil

import (
	"strings"
	"testing"

	"github.com/livereview/pkg/models"
)

func TestPackageModelDiffsToZipBase64_Roundtrip(t *testing.T) {
	diffs := []*models.CodeDiff{
		{
			FilePath:    "main.go",
			OldFilePath: "main.go",
			NewContent:  "package main\n\nfunc main() {}\n",
			Hunks: []models.DiffHunk{
				{
					OldStartLine: 1,
					OldLineCount: 1,
					NewStartLine: 1,
					NewLineCount: 3,
					Content:      "@@ -1,1 +1,3 @@\n package main\n+\n+func main() {}",
				},
			},
		},
		{
			FilePath: "new_file.py",
			IsNew:    true,
			Hunks: []models.DiffHunk{
				{
					OldStartLine: 0,
					OldLineCount: 0,
					NewStartLine: 1,
					NewLineCount: 2,
					Content:      "@@ -0,0 +1,2 @@\n+import os\n+print('hello')",
				},
			},
		},
	}

	extras := map[string][]byte{
		".lrc/policy/tools.toml": []byte("[ruff]\nenabled = true\n"),
	}

	encoded, err := PackageModelDiffsToZipBase64WithExtras(diffs, extras)
	if err != nil {
		t.Fatalf("PackageModelDiffsToZipBase64WithExtras failed: %v", err)
	}
	if encoded == "" {
		t.Fatalf("expected non-empty base64 string")
	}

	localDiffs, bundle, err := ParseDiffZipBase64(encoded)
	if err != nil {
		t.Fatalf("ParseDiffZipBase64 failed on generated zip: %v", err)
	}

	if len(localDiffs) != 2 {
		t.Fatalf("expected 2 local diffs, got %d", len(localDiffs))
	}

	if localDiffs[0].NewPath != "main.go" {
		t.Errorf("expected first diff to be main.go, got %s", localDiffs[0].NewPath)
	}

	if localDiffs[1].NewPath != "new_file.py" {
		t.Errorf("expected second diff to be new_file.py, got %s", localDiffs[1].NewPath)
	}

	if toolsToml, ok := bundle.Files["policy/tools.toml"]; !ok || !strings.Contains(string(toolsToml), "ruff") {
		t.Errorf("expected .lrc bundle to contain policy/tools.toml with ruff, got %v", bundle.Files)
	}
}

func TestConvertModelDiffsToLocal(t *testing.T) {
	diffs := []*models.CodeDiff{
		{
			FilePath:    "pkg/service.go",
			OldFilePath: "pkg/service.go",
			Hunks: []models.DiffHunk{
				{
					OldStartLine: 10,
					OldLineCount: 2,
					NewStartLine: 10,
					NewLineCount: 3,
					Content:      "@@ -10,2 +10,3 @@\n func Foo() {\n-bar()\n+baz()\n }",
				},
			},
		},
	}

	locals := ConvertModelDiffsToLocal(diffs)
	if len(locals) != 1 {
		t.Fatalf("expected 1 local diff, got %d", len(locals))
	}

	if locals[0].NewPath != "pkg/service.go" {
		t.Errorf("expected new path pkg/service.go, got %s", locals[0].NewPath)
	}

	if len(locals[0].Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(locals[0].Hunks))
	}

	hunk := locals[0].Hunks[0]
	if hunk.OldStartLine != 10 || hunk.NewStartLine != 10 {
		t.Errorf("unexpected hunk line numbers: %+v", hunk)
	}

	if len(hunk.Lines) != 4 {
		t.Errorf("expected 4 lines in hunk, got %d", len(hunk.Lines))
	}
}

func TestBuildUnifiedDiffFromModelDiffs_HunkHeaderSynthesizing(t *testing.T) {
	// Tests both hunks with pre-existing @@ headers and hunks without (requiring synthesized header)
	diffs := []*models.CodeDiff{
		{
			FilePath:    "test.txt",
			OldFilePath: "test.txt",
			Hunks: []models.DiffHunk{
				{
					OldStartLine: 1,
					OldLineCount: 1,
					NewStartLine: 1,
					NewLineCount: 2,
					Content:      " line1\n+line2\n", // No @@ prefix
				},
				{
					OldStartLine: 10,
					OldLineCount: 1,
					NewStartLine: 10,
					NewLineCount: 1,
					Content:      "@@ -10,1 +10,1 @@\n-old\n+new\n", // Has @@ prefix
				},
			},
		},
	}

	unified := BuildUnifiedDiffFromModelDiffs(diffs)
	if !strings.Contains(unified, "@@ -1,1 +1,2 @@") {
		t.Errorf("expected synthesized header for hunk without @@, got: %s", unified)
	}
	if !strings.Contains(unified, "@@ -10,1 +10,1 @@") {
		t.Errorf("expected preserved header for hunk with @@, got: %s", unified)
	}
	// Verify no double @@ in output
	if strings.Contains(unified, "@@\n@@") {
		t.Errorf("found duplicate @@ headers: %s", unified)
	}
}

