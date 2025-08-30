package aiconnectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// OllamaModel represents a model from Ollama API
type OllamaModel struct {
	Name       string       `json:"name"`
	ModifiedAt time.Time    `json:"modified_at"`
	Size       int64        `json:"size"`
	Digest     string       `json:"digest"`
	Details    ModelDetails `json:"details"`
}

// ModelDetails contains model details from Ollama
type ModelDetails struct {
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

// OllamaModelsResponse represents the response from Ollama /api/tags endpoint
type OllamaModelsResponse struct {
	Models []OllamaModel `json:"models"`
}

// FetchOllamaModels fetches available models from an Ollama instance
func FetchOllamaModels(ctx context.Context, baseURL string, jwtToken string) ([]OllamaModel, error) {
	// Default to localhost if no baseURL provided
	if baseURL == "" {
		baseURL = "http://localhost:11434/api"
	}

	// Build the API endpoint URL - just append /tags, handling trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")
	apiURL := fmt.Sprintf("%s/tags", baseURL)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add JWT token if provided
	if jwtToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwtToken))
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ollama at %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama API returned status %d: %s", resp.StatusCode, resp.Status)
	}

	// Parse the response
	var ollamaResp OllamaModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to parse Ollama response: %w", err)
	}

	return ollamaResp.Models, nil
}

// ValidateOllamaConnection validates the connection to an Ollama instance
func ValidateOllamaConnection(ctx context.Context, baseURL string, jwtToken string) error {
	models, err := FetchOllamaModels(ctx, baseURL, jwtToken)
	if err != nil {
		return err
	}

	// Check if at least one model is available
	if len(models) == 0 {
		return fmt.Errorf("no models found in Ollama instance at %s", baseURL)
	}

	return nil
}
