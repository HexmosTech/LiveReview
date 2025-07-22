package review

import (
	"context"
	"fmt"
	"time"

	"github.com/livereview/internal/config"
)

// ConfigurationService handles loading configuration for review requests
type ConfigurationService struct {
	config *config.Config
}

// NewConfigurationService creates a new configuration service
func NewConfigurationService(cfg *config.Config) *ConfigurationService {
	return &ConfigurationService{
		config: cfg,
	}
}

// BuildReviewRequest creates a ReviewRequest from a URL and integration token
func (cs *ConfigurationService) BuildReviewRequest(
	ctx context.Context,
	url string,
	reviewID string,
	providerType string,
	providerURL string,
	accessToken string,
) (*ReviewRequest, error) {

	// Build provider config
	providerConfig := ProviderConfig{
		Type:  providerType,
		URL:   providerURL,
		Token: accessToken,
	}

	// Build AI config from configuration
	aiConfig, err := cs.buildAIConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build AI config: %w", err)
	}

	return &ReviewRequest{
		URL:      url,
		ReviewID: reviewID,
		Provider: providerConfig,
		AI:       aiConfig,
	}, nil
}

// buildAIConfig constructs AI configuration from the loaded config
func (cs *ConfigurationService) buildAIConfig() (AIConfig, error) {
	// Use configured default AI provider
	defaultAI := cs.config.General.DefaultAI
	if defaultAI == "" {
		defaultAI = "gemini"
	}

	// Get AI config for the default provider
	aiProviderConfig, exists := cs.config.AI[defaultAI]
	if !exists {
		return AIConfig{}, fmt.Errorf("AI provider '%s' not configured", defaultAI)
	}

	// Extract common configuration
	apiKey, _ := aiProviderConfig["api_key"].(string)
	model, _ := aiProviderConfig["model"].(string)
	temperature, _ := aiProviderConfig["temperature"].(float64)

	// Set defaults if not specified
	if model == "" {
		model = "gemini-2.5-flash"
	}
	if temperature == 0 {
		temperature = 0.4
	}

	return AIConfig{
		Type:        defaultAI,
		APIKey:      apiKey,
		Model:       model,
		Temperature: temperature,
		Config:      aiProviderConfig,
	}, nil
}

// DefaultReviewConfig returns a sensible default configuration for reviews
func DefaultReviewConfig() Config {
	return Config{
		ReviewTimeout: 10 * time.Minute,
		DefaultAI:     "gemini",
		DefaultModel:  "gemini-2.5-flash",
		Temperature:   0.4,
	}
}
