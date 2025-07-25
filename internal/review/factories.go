package review

import (
	"context"
	"fmt"
	"log"

	"github.com/livereview/internal/ai"
	"github.com/livereview/internal/ai/gemini"
	"github.com/livereview/internal/providers"
	"github.com/livereview/internal/providers/github"
	"github.com/livereview/internal/providers/gitlab"
)

// StandardProviderFactory implements ProviderFactory for standard providers
type StandardProviderFactory struct{}

// NewStandardProviderFactory creates a new StandardProviderFactory
func NewStandardProviderFactory() *StandardProviderFactory {
	return &StandardProviderFactory{}
}

// CreateProvider creates a provider instance based on configuration
func (f *StandardProviderFactory) CreateProvider(ctx context.Context, config ProviderConfig) (providers.Provider, error) {
	switch config.Type {
	case "gitlab":
		return gitlab.New(gitlab.GitLabConfig{
			URL:   config.URL,
			Token: config.Token,
		})
	case "github":
		log.Printf("[DEBUG] Creating GitHub provider")
		pat, exists := config.Config["pat_token"].(string)
		log.Printf("[DEBUG] pat_token exists: %v, length: %d", exists, len(pat))
		provider := github.NewGitHubProvider(pat)
		// Always configure with pat_token for safety
		if err := provider.Configure(config.Config); err != nil {
			return nil, err
		}
		return provider, nil
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", config.Type)
	}
}

// SupportsProvider checks if the factory supports the given provider type
func (f *StandardProviderFactory) SupportsProvider(providerType string) bool {
	switch providerType {
	case "gitlab":
		return true
	case "github":
		return true
	default:
		return false
	}
}

// StandardAIProviderFactory implements AIProviderFactory for standard AI providers
type StandardAIProviderFactory struct{}

// NewStandardAIProviderFactory creates a new StandardAIProviderFactory
func NewStandardAIProviderFactory() *StandardAIProviderFactory {
	return &StandardAIProviderFactory{}
}

// CreateAIProvider creates an AI provider instance based on configuration
func (f *StandardAIProviderFactory) CreateAIProvider(ctx context.Context, config AIConfig) (ai.Provider, error) {
	switch config.Type {
	case "gemini":
		return gemini.New(gemini.GeminiConfig{
			APIKey:      config.APIKey,
			Model:       config.Model,
			Temperature: config.Temperature,
		})
	default:
		return nil, fmt.Errorf("unsupported AI provider type: %s", config.Type)
	}
}

// SupportsAIProvider checks if the factory supports the given AI provider type
func (f *StandardAIProviderFactory) SupportsAIProvider(aiType string) bool {
	switch aiType {
	case "gemini":
		return true
	default:
		return false
	}
}
