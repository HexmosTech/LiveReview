package review

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/livereview/internal/ai"
	"github.com/livereview/internal/batch"
	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/providers"
	"github.com/livereview/pkg/models"
)

type preloadedOnlyProviderFactory struct{}

func (f *preloadedOnlyProviderFactory) CreateProvider(ctx context.Context, config ProviderConfig) (providers.Provider, error) {
	return nil, fmt.Errorf("provider creation should not be called for preloaded changes")
}

func (f *preloadedOnlyProviderFactory) SupportsProvider(providerType string) bool {
	return true
}

type fixedAIProviderFactory struct {
	provider ai.Provider
}

func (f *fixedAIProviderFactory) CreateAIProvider(ctx context.Context, config AIConfig, logger *logging.ReviewLogger) (ai.Provider, error) {
	return f.provider, nil
}

func (f *fixedAIProviderFactory) SupportsAIProvider(aiType string) bool {
	return true
}

type mutatingAIProvider struct{}

func (p *mutatingAIProvider) ReviewCode(ctx context.Context, diffs []*models.CodeDiff) (*models.ReviewResult, error) {
	return &models.ReviewResult{Summary: "mock summary", Comments: []*models.ReviewComment{}}, nil
}

func (p *mutatingAIProvider) ReviewCodeBatch(ctx context.Context, diffs []models.CodeDiff) (*batch.BatchResult, error) {
	return &batch.BatchResult{Summary: "mock summary", Comments: []*models.ReviewComment{}}, nil
}

func (p *mutatingAIProvider) ReviewCodeWithBatching(ctx context.Context, diffs []*models.CodeDiff, batchProcessor *batch.BatchProcessor) (*models.ReviewResult, error) {
	for i := range diffs {
		for j := range diffs[i].Hunks {
			// Emulate provider formatting that rewrites unified lines into table rows.
			diffs[i].Hunks[j].Content = "OLD | NEW | CONTENT\n----|-----|--------\n  1 |     | -old\n    |   1 | +new\n    |   2 | +added"
		}
	}
	return &models.ReviewResult{Summary: "mock summary", Comments: []*models.ReviewComment{}}, nil
}

func (p *mutatingAIProvider) Configure(config map[string]interface{}) error {
	return nil
}

func (p *mutatingAIProvider) Name() string {
	return "mutating-mock"
}

func (p *mutatingAIProvider) MaxTokensPerBatch() int {
	return 10000
}

func TestCalculateBillableLOCFromDiffs_CountsUnifiedDiffLines(t *testing.T) {
	diffs := []*models.CodeDiff{
		{
			FilePath: "main.cpp",
			Hunks: []models.DiffHunk{
				{
					Content: "@@ -1,3 +1,4 @@\n line1\n-old\n+new\n+added\n line3\n--- a/main.cpp\n+++ b/main.cpp",
				},
			},
		},
	}

	got := calculateBillableLOCFromDiffs(diffs)
	var want int64 = 3
	if got != want {
		t.Fatalf("calculateBillableLOCFromDiffs()=%d, want=%d", got, want)
	}
}

func TestProcessReview_PreloadedChanges_PreservesBillableLOCWhenAIReformatsHunks(t *testing.T) {
	preloadedChanges := []*models.CodeDiff{
		{
			FilePath: "main.cpp",
			Hunks: []models.DiffHunk{
				{
					OldStartLine: 1,
					OldLineCount: 3,
					NewStartLine: 1,
					NewLineCount: 4,
					Content:      "@@ -1,3 +1,4 @@\n line1\n-old\n+new\n+added\n line3",
				},
			},
		},
	}

	svc := NewService(
		&preloadedOnlyProviderFactory{},
		&fixedAIProviderFactory{provider: &mutatingAIProvider{}},
		Config{ReviewTimeout: 5 * time.Second, DefaultAI: "mock", DefaultModel: "mock"},
	)

	request := ReviewRequest{
		URL:              "cli://diff",
		ReviewID:         "test-review-id",
		Provider:         ProviderConfig{Type: "cli"},
		AI:               AIConfig{Type: "mock", Model: "mock-model"},
		PreloadedChanges: preloadedChanges,
	}

	result := svc.ProcessReview(context.Background(), request)
	if result == nil {
		t.Fatalf("ProcessReview() returned nil result")
	}
	if result.Error != nil {
		t.Fatalf("ProcessReview() returned error: %v", result.Error)
	}
	if !result.Success {
		t.Fatalf("ProcessReview() returned Success=false")
	}

	var wantLOC int64 = 3
	if result.BillableLOC != wantLOC {
		t.Fatalf("result.BillableLOC=%d, want=%d", result.BillableLOC, wantLOC)
	}

	if !strings.HasPrefix(preloadedChanges[0].Hunks[0].Content, "OLD | NEW | CONTENT") {
		t.Fatalf("expected hunk content to be reformatted by AI provider, got: %q", preloadedChanges[0].Hunks[0].Content)
	}
}
