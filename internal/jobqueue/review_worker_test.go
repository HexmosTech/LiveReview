package jobqueue

import (
	"context"
	"testing"

	"github.com/livereview/internal/diffutil"
	"github.com/livereview/internal/providers"
	"github.com/livereview/internal/review"
	"github.com/livereview/pkg/models"
)

func TestManualReviewWorker_ExtractChangesAndLRC_Preloaded(t *testing.T) {
	ctx := context.Background()
	worker := &ManualReviewWorker{}

	expectedDiffs := []*models.CodeDiff{
		{
			FilePath:    "pkg/api/handler.go",
			OldFilePath: "pkg/api/handler.go",
			NewContent:  "package api\n",
			Hunks: []models.DiffHunk{
				{
					OldStartLine: 1,
					OldLineCount: 1,
					NewStartLine: 1,
					NewLineCount: 2,
					Content:      "@@ -1,1 +1,2 @@\n package api\n+var Version = \"1.0.0\"\n",
				},
			},
		},
	}

	req := &review.ReviewRequest{
		PreloadedChanges: expectedDiffs,
	}

	changes, bundle, _ := worker.extractChangesAndLRC(ctx, req)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].FilePath != "pkg/api/handler.go" {
		t.Errorf("expected FilePath pkg/api/handler.go, got %s", changes[0].FilePath)
	}
	if bundle.Files != nil && len(bundle.Files) > 0 {
		t.Errorf("expected empty bundle for preloaded changes, got %v", bundle.Files)
	}

	// Verify packaging to zip
	b64Zip, err := diffutil.PackageModelDiffsToZipBase64WithExtras(changes, bundle.Files)
	if err != nil {
		t.Fatalf("PackageModelDiffsToZipBase64WithExtras failed: %v", err)
	}
	if b64Zip == "" {
		t.Fatalf("expected non-empty base64 zip string")
	}

	localDiffs := diffutil.ConvertModelDiffsToLocal(changes)
	if len(localDiffs) != 1 || localDiffs[0].NewPath != "pkg/api/handler.go" {
		t.Errorf("unexpected local diffs: %+v", localDiffs)
	}
}

func TestManualReviewWorker_ExtractChangesAndLRC_Nil(t *testing.T) {
	ctx := context.Background()
	worker := &ManualReviewWorker{}

	changes, bundle, _ := worker.extractChangesAndLRC(ctx, nil)
	if changes != nil {
		t.Errorf("expected nil changes for nil request, got %v", changes)
	}
	if bundle.Files != nil && len(bundle.Files) > 0 {
		t.Errorf("expected empty bundle for nil request, got %v", bundle.Files)
	}

	emptyReq := &review.ReviewRequest{}
	changes, _, _ = worker.extractChangesAndLRC(ctx, emptyReq)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for empty request, got %d", len(changes))
	}
}

func TestExtractLRCFromDiffs(t *testing.T) {
	diffs := []*models.CodeDiff{
		{
			FilePath: ".lrc/policy/tools.toml",
			Hunks: []models.DiffHunk{
				{
					Content: "@@ -1,2 +1,3 @@\n [trufflehog]\n+enabled = true\n include = [\"*.py\"]\n\\ No newline at end of file\n",
				},
			},
		},
		{
			FilePath:   ".lrc/rules/security.md",
			NewContent: "# Security Rules\nDo not hardcode secrets.\n",
		},
		{
			FilePath:  ".lrc/ignore",
			IsDeleted: true,
		},
		{
			FilePath: "src/main.py",
			Hunks: []models.DiffHunk{
				{
					Content: "@@ -1,1 +1,2 @@\n print('hello')\n",
				},
			},
		},
	}

	files := extractLRCFromDiffs(diffs)
	if len(files) != 2 {
		t.Fatalf("expected 2 .lrc files extracted, got %d: %+v", len(files), files)
	}

	policyBytes, ok := files["policy/tools.toml"]
	if !ok {
		t.Fatalf("expected policy/tools.toml to be extracted")
	}
	policyStr := string(policyBytes)
	expectedPolicy := "[trufflehog]\nenabled = true\ninclude = [\"*.py\"]\n"
	if policyStr != expectedPolicy {
		t.Errorf("unexpected policy content: %q, want %q", policyStr, expectedPolicy)
	}

	rulesBytes, ok := files["rules/security.md"]
	if !ok {
		t.Fatalf("expected rules/security.md to be extracted")
	}
	if string(rulesBytes) != "# Security Rules\nDo not hardcode secrets.\n" {
		t.Errorf("unexpected rules content: %q", string(rulesBytes))
	}
}

func TestResolvePostingID(t *testing.T) {
	tests := []struct {
		name      string
		req       *review.ReviewRequest
		mrDetails *providers.MergeRequestDetails
		expected  string
	}{
		{
			name: "Bitbucket PR URL",
			req: &review.ReviewRequest{
				URL:      "https://bitbucket.org/myworkspace/myrepo/pull-requests/42",
				Provider: review.ProviderConfig{Type: "bitbucket"},
			},
			expected: "myworkspace/myrepo/42",
		},
		{
			name: "GitHub PR URL",
			req: &review.ReviewRequest{
				URL:      "https://github.com/myorg/myrepo/pull/101",
				Provider: review.ProviderConfig{Type: "github"},
			},
			expected: "myorg/myrepo/101",
		},
		{
			name: "GitLab MR URL",
			req: &review.ReviewRequest{
				URL:      "https://gitlab.com/group/subgroup/project/-/merge_requests/77",
				Provider: review.ProviderConfig{Type: "gitlab"},
			},
			expected: "77",
		},
		{
			name: "From MRDetails takes priority",
			req: &review.ReviewRequest{
				URL:      "https://bitbucket.org/other/repo/pull-requests/1",
				Provider: review.ProviderConfig{Type: "bitbucket"},
			},
			mrDetails: &providers.MergeRequestDetails{
				ProviderType: "bitbucket",
				URL:          "https://bitbucket.org/customworkspace/customrepo/pull-requests/99",
				ID:           "99",
			},
			expected: "customworkspace/customrepo/99",
		},
		{
			name:     "Nil request",
			req:      nil,
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolvePostingID(tc.req, tc.mrDetails)
			if got != tc.expected {
				t.Errorf("resolvePostingID() = %q, want %q", got, tc.expected)
			}
		})
	}
}
