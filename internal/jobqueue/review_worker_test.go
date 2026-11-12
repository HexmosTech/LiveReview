package jobqueue

import (
	"context"
	"testing"

	"github.com/livereview/internal/diffutil"
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

	changes, bundle := worker.extractChangesAndLRC(ctx, req)
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

	changes, bundle := worker.extractChangesAndLRC(ctx, nil)
	if changes != nil {
		t.Errorf("expected nil changes for nil request, got %v", changes)
	}
	if bundle.Files != nil && len(bundle.Files) > 0 {
		t.Errorf("expected empty bundle for nil request, got %v", bundle.Files)
	}

	emptyReq := &review.ReviewRequest{}
	changes, _ = worker.extractChangesAndLRC(ctx, emptyReq)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for empty request, got %d", len(changes))
	}
}
