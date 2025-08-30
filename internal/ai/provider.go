package ai

import (
	"context"

	"github.com/livereview/internal/batch"
	"github.com/livereview/pkg/models"
)

// Provider represents an AI service that can review code
type Provider interface {
	// ReviewCode takes code diff information and returns a review result with summary and comments
	// This is the legacy method that doesn't use batching
	ReviewCode(ctx context.Context, diffs []*models.CodeDiff) (*models.ReviewResult, error)

	// ReviewCodeBatch processes a single batch of code diffs
	// Used internally by ReviewCodeWithBatching
	ReviewCodeBatch(ctx context.Context, diffs []models.CodeDiff) (*batch.BatchResult, error)

	// ReviewCodeWithBatching processes code diffs in batches for large merge requests
	// Uses a batch processor to split the diffs into manageable chunks
	ReviewCodeWithBatching(ctx context.Context, diffs []*models.CodeDiff, batchProcessor *batch.BatchProcessor) (*models.ReviewResult, error)

	// Configure sets up the provider with needed configuration
	Configure(config map[string]interface{}) error

	// Name returns the provider's name
	Name() string

	// MaxTokensPerBatch returns the maximum number of tokens allowed per batch
	MaxTokensPerBatch() int
}

// Factory creates AI providers based on configuration
type Factory interface {
	// Create creates a new AI provider based on the given name
	Create(name string, config map[string]interface{}) (Provider, error)
}

// DefaultFactory is the default implementation of Factory
type DefaultFactory struct {
	providers map[string]Provider
}

// NewDefaultFactory creates a new DefaultFactory
func NewDefaultFactory() *DefaultFactory {
	return &DefaultFactory{
		providers: make(map[string]Provider),
	}
}

// Register registers a provider with the factory
func (f *DefaultFactory) Register(name string, provider Provider) {
	f.providers[name] = provider
}

// Create creates a new AI provider based on the given name
func (f *DefaultFactory) Create(name string, config map[string]interface{}) (Provider, error) {
	provider, ok := f.providers[name]
	if !ok {
		return nil, ErrProviderNotFound
	}

	if err := provider.Configure(config); err != nil {
		return nil, err
	}

	return provider, nil
}

// Errors
var (
	ErrProviderNotFound = error(ErrorProviderNotFound("ai provider not found"))
)

// ErrorProviderNotFound is returned when an AI provider is not found
type ErrorProviderNotFound string

func (e ErrorProviderNotFound) Error() string {
	return string(e)
}
