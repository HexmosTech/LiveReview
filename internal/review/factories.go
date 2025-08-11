package review

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/livereview/internal/ai"
	"github.com/livereview/internal/ai/langchain"
	"github.com/livereview/internal/providers"
	"github.com/livereview/internal/providers/bitbucket"
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
	// Handle GitLab variants (gitlab, gitlab-self-hosted, etc.)
	if strings.HasPrefix(config.Type, "gitlab") {
		return gitlab.New(gitlab.GitLabConfig{
			URL:   config.URL,
			Token: config.Token,
		})
	}

	// Handle GitHub variants
	if strings.HasPrefix(config.Type, "github") {
		log.Printf("[DEBUG] Creating GitHub provider")
		pat, exists := config.Config["pat_token"].(string)
		log.Printf("[DEBUG] pat_token exists: %v, length: %d", exists, len(pat))
		provider := github.NewGitHubProvider(pat)
		// Always configure with pat_token for safety
		if err := provider.Configure(config.Config); err != nil {
			return nil, err
		}
		return provider, nil
	}

	// Handle Bitbucket variants
	if strings.HasPrefix(config.Type, "bitbucket") {
		log.Printf("[DEBUG] Creating Bitbucket provider")
		apiToken, _ := config.Config["pat_token"].(string)
		email, _ := config.Config["email"].(string)
		log.Printf("[DEBUG] Bitbucket token exists: %v, email: %s", len(apiToken) > 0, email)
		provider := bitbucket.NewBitbucketProvider(apiToken, email)
		if err := provider.Configure(config.Config); err != nil {
			return nil, err
		}
		return provider, nil
	}

	return nil, fmt.Errorf("unsupported provider type: %s", config.Type)
}

// SupportsProvider checks if the factory supports the given provider type
func (f *StandardProviderFactory) SupportsProvider(providerType string) bool {
	// Support GitLab variants (gitlab, gitlab-self-hosted, etc.)
	if strings.HasPrefix(providerType, "gitlab") {
		return true
	}
	// Support GitHub variants
	if strings.HasPrefix(providerType, "github") {
		return true
	}
	// Support Bitbucket variants
	if strings.HasPrefix(providerType, "bitbucket") {
		return true
	}
	return false
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
	case "langchain":
		return langchain.New(langchain.Config{
			APIKey:    config.APIKey,
			ModelName: config.Model,
			MaxTokens: 30000, // Default max tokens
		}), nil
	default:
		// Default to langchain for any unrecognized type
		return langchain.New(langchain.Config{
			APIKey:    config.APIKey,
			ModelName: config.Model,
			MaxTokens: 30000,
		}), nil
	}
}

// SupportsAIProvider checks if the factory supports the given AI provider type
func (f *StandardAIProviderFactory) SupportsAIProvider(aiType string) bool {
	switch aiType {
	case "langchain":
		return true
	default:
		// Default to true since we default to langchain
		return true
	}
}
