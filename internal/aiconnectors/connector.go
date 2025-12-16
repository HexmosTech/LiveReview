package aiconnectors

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/cohere"
	"github.com/tmc/langchaingo/llms/googleai" // Use googleai instead of gemini
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"
)

// Provider represents an AI provider type
type Provider string

const (
	// Provider types
	ProviderOpenAI     Provider = "openai"
	ProviderGemini     Provider = "gemini"
	ProviderClaude     Provider = "claude"
	ProviderCohere     Provider = "cohere"
	ProviderOllama     Provider = "ollama"
	ProviderLocalModel Provider = "local"
)

// ModelConfig contains the configuration for a specific model
type ModelConfig struct {
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	TopK        float64 `json:"top_k,omitempty"`
	Model       string  `json:"model,omitempty"`
}

// ConnectorOptions contains options for creating a connector
type ConnectorOptions struct {
	Provider    Provider    `json:"provider"`
	APIKey      string      `json:"api_key"`
	BaseURL     string      `json:"base_url,omitempty"`
	ModelConfig ModelConfig `json:"model_config,omitempty"`
}

// Connector represents a connection to an AI provider
type Connector struct {
	provider Provider
	llm      llms.Model
	options  ConnectorOptions
}

// NewConnector creates a new connector for the specified provider
func NewConnector(ctx context.Context, options ConnectorOptions) (*Connector, error) {
	var model llms.Model
	var err error

	log.Debug().
		Str("provider", string(options.Provider)).
		Str("model", options.ModelConfig.Model).
		Float64("temperature", options.ModelConfig.Temperature).
		Msg("Creating new connector")

	switch options.Provider {
	case ProviderOpenAI:
		model, err = createOpenAIModel(ctx, options)
	case ProviderGemini:
		model, err = createGeminiModel(ctx, options)
	case ProviderClaude:
		model, err = createAnthropicModel(ctx, options)
	case ProviderCohere:
		model, err = createCohereModel(ctx, options)
	case ProviderOllama:
		model, err = createOllamaModel(ctx, options)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", options.Provider)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create model for provider %s: %w", options.Provider, err)
	}

	return &Connector{
		provider: options.Provider,
		llm:      model,
		options:  options,
	}, nil
}

// ValidateAPIKey validates the provided API key against the provider
func ValidateAPIKey(ctx context.Context, provider Provider, apiKey string, baseURL string) (bool, error) {
	log.Debug().
		Str("provider", string(provider)).
		Str("api_key_prefix", apiKey[:min(len(apiKey), 10)]).
		Str("base_url", baseURL).
		Msg("Starting API key validation")

	// For Ollama, validate by trying to fetch models instead of text generation
	if provider == ProviderOllama {
		log.Debug().Msg("Validating Ollama by fetching models")
		_, err := FetchOllamaModels(ctx, baseURL, apiKey)
		if err != nil {
			log.Error().Err(err).
				Str("base_url", baseURL).
				Msg("Ollama validation failed - could not fetch models")
			return false, nil // Invalid credentials, but not a system error
		}
		log.Debug().Msg("Ollama validation successful - models fetched")
		return true, nil
	}

	// Create temporary options with minimum configuration
	options := ConnectorOptions{
		Provider: provider,
		APIKey:   apiKey,
		BaseURL:  baseURL,
		ModelConfig: ModelConfig{
			Temperature: 0.7,
			MaxTokens:   100,
		},
	}

	// Set default model based on provider
	switch provider {
	case ProviderOpenAI:
		options.ModelConfig.Model = "gpt-3.5-turbo"
	case ProviderGemini:
		options.ModelConfig.Model = "gemini-2.5-flash"
		log.Debug().Msg("Using Gemini 2.5 Flash model for validation")
	case ProviderClaude:
		options.ModelConfig.Model = "claude-3-sonnet-20240229"
	case ProviderCohere:
		options.ModelConfig.Model = "command"
	case ProviderOllama:
		options.ModelConfig.Model = "llama3"
	default:
		log.Error().Str("provider", string(provider)).Msg("Unsupported provider")
		return false, fmt.Errorf("unsupported provider: %s", provider)
	}

	log.Debug().
		Str("provider", string(provider)).
		Str("model", options.ModelConfig.Model).
		Float64("temperature", options.ModelConfig.Temperature).
		Int("max_tokens", options.ModelConfig.MaxTokens).
		Msg("Creating connector with validation options")

	// Create a connector with the API key to validate
	connector, err := NewConnector(ctx, options)
	if err != nil {
		log.Error().Err(err).
			Str("provider", string(provider)).
			Str("model", options.ModelConfig.Model).
			Msg("Failed to create connector during validation")
		return false, fmt.Errorf("failed to create connector: %w", err)
	}

	log.Debug().
		Str("provider", string(provider)).
		Str("model", options.ModelConfig.Model).
		Msg("Connector created successfully, attempting to generate text")

	// Test the connection with a simple call
	promptText := "test"
	log.Debug().
		Str("prompt", promptText).
		Str("model", options.ModelConfig.Model).
		Int("max_tokens", 10).
		Msg("Making validation API call with simple prompt")

	var generateOptions []llms.CallOption
	generateOptions = append(generateOptions, llms.WithMaxTokens(10))

	// For Gemini, explicitly specify the model in the call
	if provider == ProviderGemini {
		generateOptions = append(generateOptions, llms.WithModel(options.ModelConfig.Model))
		log.Debug().Str("model", options.ModelConfig.Model).Msg("Explicitly setting model for Gemini call")
	}

	_, err = llms.GenerateFromSinglePrompt(ctx, connector.llm, promptText, generateOptions...)
	if err != nil {
		log.Error().Err(err).
			Str("provider", string(provider)).
			Str("model", options.ModelConfig.Model).
			Str("api_key_prefix", apiKey[:min(len(apiKey), 10)]).
			Str("error_type", fmt.Sprintf("%T", err)).
			Str("full_error", fmt.Sprintf("%+v", err)).
			Msg("API key validation failed with error")

		// Check if error is quota-related (contains "429" or "quota")
		errStr := err.Error()
		if contains(errStr, "429") || contains(errStr, "quota") || contains(errStr, "Quota exceeded") {
			return false, fmt.Errorf("quota exceeded - this typically means the API key is valid but has reached its rate limit: %w", err)
		}

		return false, nil // API key is invalid, but don't return an error
	}

	log.Debug().
		Str("provider", string(provider)).
		Str("model", options.ModelConfig.Model).
		Str("api_key_prefix", apiKey[:min(len(apiKey), 10)]).
		Msg("API key validation successful")

	return true, nil // API key is valid
}

// Helper functions to create models for specific providers

func createOpenAIModel(ctx context.Context, options ConnectorOptions) (llms.Model, error) {
	// The OpenAI library doesn't have all the options we want to set directly as constructor options
	// We'll just use the basic options available
	opts := []openai.Option{
		openai.WithModel(options.ModelConfig.Model),
		openai.WithToken(options.APIKey),
	}

	// Add custom base URL if provided
	if options.BaseURL != "" {
		opts = append(opts, openai.WithBaseURL(options.BaseURL))
	}

	return openai.New(opts...)
}

func createGeminiModel(ctx context.Context, options ConnectorOptions) (llms.Model, error) {
	log.Debug().
		Str("api_key_prefix", options.APIKey[:min(len(options.APIKey), 10)]).
		Str("model", options.ModelConfig.Model).
		Msg("Creating Gemini model with options")

	// Start with minimal options to avoid potential parameter conflicts
	opts := []googleai.Option{
		googleai.WithAPIKey(options.APIKey),
	}

	// Try without setting default model to see if that's causing the issue
	// opts = append(opts, googleai.WithDefaultModel(options.ModelConfig.Model))

	log.Debug().Msg("Calling googleai.New to create client with minimal options")
	model, err := googleai.New(ctx, opts...)
	if err != nil {
		log.Error().Err(err).
			Str("api_key_prefix", options.APIKey[:min(len(options.APIKey), 10)]).
			Str("model", options.ModelConfig.Model).
			Str("error_type", fmt.Sprintf("%T", err)).
			Str("full_error", fmt.Sprintf("%+v", err)).
			Msg("Failed to create Gemini model")
		return nil, fmt.Errorf("failed to create Gemini model: %w", err)
	}

	log.Debug().Msg("Gemini model created successfully")
	return model, nil
}

func createAnthropicModel(ctx context.Context, options ConnectorOptions) (llms.Model, error) {
	opts := []anthropic.Option{
		anthropic.WithToken(options.APIKey),
		anthropic.WithModel(options.ModelConfig.Model),
	}

	return anthropic.New(opts...)
}

func createCohereModel(ctx context.Context, options ConnectorOptions) (llms.Model, error) {
	opts := []cohere.Option{
		cohere.WithToken(options.APIKey),
		cohere.WithModel(options.ModelConfig.Model),
	}

	// Add custom base URL if provided
	if options.BaseURL != "" {
		opts = append(opts, cohere.WithBaseURL(options.BaseURL))
	}

	return cohere.New(opts...)
}

func createOllamaModel(ctx context.Context, options ConnectorOptions) (llms.Model, error) {
	// Set default server URL if not provided
	if options.BaseURL == "" {
		options.BaseURL = "http://localhost:11434"
	}

	opts := []ollama.Option{
		ollama.WithServerURL(options.BaseURL),
		ollama.WithModel(options.ModelConfig.Model),
	}

	// Note: Ollama doesn't have direct options for temperature, tokens, or top_p
	// We'll need to use llms.CallOption when making calls instead

	return ollama.New(opts...)
}

// Call calls the LLM with the given input and returns the response
func (c *Connector) Call(ctx context.Context, input string, options ...llms.CallOption) (string, error) {
	// Add default options based on connector configuration
	callOptions := []llms.CallOption{
		llms.WithTemperature(c.options.ModelConfig.Temperature),
	}

	if c.options.ModelConfig.MaxTokens > 0 {
		callOptions = append(callOptions, llms.WithMaxTokens(c.options.ModelConfig.MaxTokens))
	}

	if c.options.ModelConfig.TopP > 0 {
		callOptions = append(callOptions, llms.WithTopP(c.options.ModelConfig.TopP))
	}

	if c.options.ModelConfig.TopK > 0 {
		callOptions = append(callOptions, llms.WithTopK(int(c.options.ModelConfig.TopK)))
	}

	// Append any additional options passed to the Call function
	callOptions = append(callOptions, options...)

	// Use GenerateFromSinglePrompt which is the recommended approach
	return llms.GenerateFromSinglePrompt(ctx, c.llm, input, callOptions...)
}

// GetProvider returns the provider of this connector
func (c *Connector) GetProvider() Provider {
	return c.provider
}

// GetModel returns the model name from the config
func (c *Connector) GetModel() string {
	return c.options.ModelConfig.Model
}

// Helper function to get minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Helper function to check if string contains substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
