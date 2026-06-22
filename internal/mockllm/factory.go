//go:build !production

package mockllm

import (
	"context"

	"github.com/livereview/internal/ai"
	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/review"
)

// MockAIProviderFactory implements review.AIProviderFactory
type MockAIProviderFactory struct{}

func (f *MockAIProviderFactory) CreateAIProvider(ctx context.Context, config review.AIConfig, logger *logging.ReviewLogger) (ai.Provider, error) {
	return &MockAIProvider{logger: logger}, nil
}

func (f *MockAIProviderFactory) SupportsAIProvider(aiType string) bool {
	return true
}
