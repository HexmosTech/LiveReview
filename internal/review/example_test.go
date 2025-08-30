package review

import (
	"context"
	"testing"
	"time"

	"github.com/livereview/internal/ai"
	"github.com/livereview/internal/batch"
	"github.com/livereview/internal/providers"
	"github.com/livereview/pkg/models"
)

// Example test to show how the new decoupled architecture would work

// MockProvider is a test provider implementation
type MockProvider struct {
	mrDetails *providers.MergeRequestDetails
	changes   []*models.CodeDiff
	comments  []*models.ReviewComment
}

func (m *MockProvider) GetMergeRequestDetails(ctx context.Context, mrURL string) (*providers.MergeRequestDetails, error) {
	return m.mrDetails, nil
}

func (m *MockProvider) GetMergeRequestChanges(ctx context.Context, mrID string) ([]*models.CodeDiff, error) {
	return m.changes, nil
}

func (m *MockProvider) PostComment(ctx context.Context, mrID string, comment *models.ReviewComment) error {
	m.comments = append(m.comments, comment)
	return nil
}

func (m *MockProvider) PostComments(ctx context.Context, mrID string, comments []*models.ReviewComment) error {
	m.comments = append(m.comments, comments...)
	return nil
}

func (m *MockProvider) Name() string {
	return "mock"
}

func (m *MockProvider) Configure(config map[string]interface{}) error {
	return nil
}

// MockAIProvider is a test AI provider implementation
type MockAIProvider struct {
	result *models.ReviewResult
}

func (m *MockAIProvider) ReviewCode(ctx context.Context, diffs []*models.CodeDiff) (*models.ReviewResult, error) {
	return m.result, nil
}

func (m *MockAIProvider) ReviewCodeBatch(ctx context.Context, diffs []models.CodeDiff) (*batch.BatchResult, error) {
	// Convert to pointer slice for ReviewCode call
	ptrDiffs := make([]*models.CodeDiff, len(diffs))
	for i := range diffs {
		ptrDiffs[i] = &diffs[i]
	}

	result, err := m.ReviewCode(ctx, ptrDiffs)
	if err != nil {
		return nil, err
	}

	return &batch.BatchResult{
		Summary:  result.Summary,
		Comments: result.Comments,
	}, nil
}

func (m *MockAIProvider) ReviewCodeWithBatching(ctx context.Context, diffs []*models.CodeDiff, batchProcessor *batch.BatchProcessor) (*models.ReviewResult, error) {
	// For mock, just call ReviewCode directly
	return m.ReviewCode(ctx, diffs)
}

func (m *MockAIProvider) Configure(config map[string]interface{}) error {
	return nil
}

func (m *MockAIProvider) Name() string {
	return "mock-ai"
}

func (m *MockAIProvider) MaxTokensPerBatch() int {
	return 10000
}

// MockProviderFactory creates mock providers
type MockProviderFactory struct {
	provider providers.Provider
}

func (f *MockProviderFactory) CreateProvider(ctx context.Context, config ProviderConfig) (providers.Provider, error) {
	return f.provider, nil
}

func (f *MockProviderFactory) SupportsProvider(providerType string) bool {
	return true
}

// MockAIProviderFactory creates mock AI providers
type MockAIProviderFactory struct {
	aiProvider ai.Provider
}

func (f *MockAIProviderFactory) CreateAIProvider(ctx context.Context, config AIConfig) (ai.Provider, error) {
	return f.aiProvider, nil
}

func (f *MockAIProviderFactory) SupportsAIProvider(aiType string) bool {
	return true
}

// ExampleDecoupledReview shows how the new architecture works
func ExampleDecoupledReview(t *testing.T) {
	// Setup mock data
	mockMR := &providers.MergeRequestDetails{
		ID:        "123",
		Title:     "Test MR",
		ProjectID: "project1",
	}

	mockChanges := []*models.CodeDiff{
		{
			FilePath:   "main.go",
			NewContent: "package main\n\nfunc main() {\n    fmt.Println(\"Hello\")\n}",
		},
	}

	mockReviewResult := &models.ReviewResult{
		Summary: "Code looks good with minor improvements needed",
		Comments: []*models.ReviewComment{
			{
				FilePath: "main.go",
				Line:     4,
				Content:  "Consider adding error handling",
				Severity: models.SeverityWarning,
			},
		},
	}

	// Create mock providers
	mockProvider := &MockProvider{
		mrDetails: mockMR,
		changes:   mockChanges,
	}

	mockAIProvider := &MockAIProvider{
		result: mockReviewResult,
	}

	// Create factories
	providerFactory := &MockProviderFactory{provider: mockProvider}
	aiProviderFactory := &MockAIProviderFactory{aiProvider: mockAIProvider}

	// Create service with config
	config := Config{
		ReviewTimeout: 5 * time.Minute,
		DefaultAI:     "mock-ai",
	}

	service := NewService(providerFactory, aiProviderFactory, config)

	// Create review request
	request := ReviewRequest{
		URL:      "https://git.example.com/group/project/-/merge_requests/123",
		ReviewID: "test-review-123",
		Provider: ProviderConfig{
			Type:  "mock",
			URL:   "https://git.example.com",
			Token: "test-token",
		},
		AI: AIConfig{
			Type:   "mock-ai",
			APIKey: "test-key",
			Model:  "test-model",
		},
	}

	// Process review
	ctx := context.Background()
	result := service.ProcessReview(ctx, request)

	// Verify results
	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}

	if result.CommentsCount != 1 {
		t.Errorf("Expected 1 comment, got %d", result.CommentsCount)
	}

	if len(mockProvider.comments) != 2 { // 1 summary + 1 specific comment
		t.Errorf("Expected 2 posted comments, got %d", len(mockProvider.comments))
	}

	t.Logf("Review completed successfully in %v", result.Duration)
	t.Logf("Posted %d comments total", len(mockProvider.comments))
}
