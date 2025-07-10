package gemini

import (
	"context"
	"fmt"

	"github.com/livereview/pkg/models"
)

// GeminiProvider implements the AI Provider interface for Google's Gemini
type GeminiProvider struct {
	apiKey      string
	model       string
	temperature float64
}

// GeminiConfig contains configuration for the Gemini provider
type GeminiConfig struct {
	APIKey      string  `koanf:"api_key"`
	Model       string  `koanf:"model"`
	Temperature float64 `koanf:"temperature"`
}

// New creates a new GeminiProvider
func New(config GeminiConfig) (*GeminiProvider, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("Gemini API key is required")
	}

	if config.Model == "" {
		config.Model = "gemini-pro"
	}

	if config.Temperature == 0 {
		config.Temperature = 0.2
	}

	return &GeminiProvider{
		apiKey:      config.APIKey,
		model:       config.Model,
		temperature: config.Temperature,
	}, nil
}

// ReviewCode takes code diff information and returns review comments
func (p *GeminiProvider) ReviewCode(ctx context.Context, diffs []*models.CodeDiff) ([]*models.ReviewComment, error) {
	// TODO: Implement Gemini API client
	// This is a stub implementation
	return []*models.ReviewComment{
		{
			FilePath: "example.go",
			Line:     4,
			Content:  "You should import the fmt package when using fmt.Println",
			Severity: models.SeverityWarning,
			Category: "import",
			Suggestions: []string{
				"Add `import \"fmt\"` at the top of the file",
			},
		},
	}, nil
}

// Configure sets up the provider with needed configuration
func (p *GeminiProvider) Configure(config map[string]interface{}) error {
	// Extract configuration values
	if apiKey, ok := config["api_key"].(string); ok {
		p.apiKey = apiKey
	} else {
		return fmt.Errorf("api_key is required")
	}

	if model, ok := config["model"].(string); ok && model != "" {
		p.model = model
	}

	if temp, ok := config["temperature"].(float64); ok && temp > 0 {
		p.temperature = temp
	}

	return nil
}

// Name returns the provider's name
func (p *GeminiProvider) Name() string {
	return "gemini"
}
