package review

import (
	"context"

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
