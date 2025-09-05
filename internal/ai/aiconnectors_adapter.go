package ai

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/prompts"
	vendorpack "github.com/livereview/internal/prompts/vendor"
	"github.com/livereview/pkg/models"
	"github.com/rs/zerolog/log"
)

// AIConnectorsAdapter implements the AI Provider interface using the aiconnectors module
type AIConnectorsAdapter struct {
	storage  *aiconnectors.Storage
	provider aiconnectors.Provider
	model    string
}

// NewAIConnectorsAdapter creates a new adapter for the aiconnectors module
func NewAIConnectorsAdapter(db *sql.DB, provider aiconnectors.Provider, model string) (*AIConnectorsAdapter, error) {
	storage := aiconnectors.NewStorage(db)

	return &AIConnectorsAdapter{
		storage:  storage,
		provider: provider,
		model:    model,
	}, nil
}

// ReviewCode takes code diff information and returns a review result with summary and comments
func (a *AIConnectorsAdapter) ReviewCode(ctx context.Context, diffs []*models.CodeDiff) (*models.ReviewResult, error) {
	// Get all connectors for this provider
	connectors, err := a.storage.GetConnectorsByProvider(ctx, a.provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get connectors: %w", err)
	}

	if len(connectors) == 0 {
		return nil, fmt.Errorf("no connectors found for provider %s", a.provider)
	}

	// Use the first connector (could be enhanced to use a strategy for selecting the connector)
	connector := connectors[0]

	// Create connector options
	options := connector.GetConnectorOptions()

	// Override model if specified
	if a.model != "" {
		options.ModelConfig.Model = a.model
	}

	// Create a connector instance
	client, err := aiconnectors.NewConnector(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create connector: %w", err)
	}

	// Build prompt via new Manager.Render + code changes section (no DB dependency yet)
	mgr := prompts.NewManager(nil, vendorpack.New())
	base, err := mgr.Render(ctx, prompts.Context{OrgID: 0}, "code_review", map[string]string{
		"style_guide":         "",
		"security_guidelines": "",
	})
	if err != nil {
		return nil, fmt.Errorf("build prompt failed: %w", err)
	}
	prompt := base + "\n\n" + prompts.BuildCodeChangesSection(diffs)

	// Call the AI provider
	log.Info().
		Str("provider", string(a.provider)).
		Str("model", options.ModelConfig.Model).
		Int("diff_files", len(diffs)).
		Msg("Sending code review request to AI provider")

	response, err := client.Call(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("AI provider call failed: %w", err)
	}

	// Parse the response to extract review comments
	result, err := parseResponse(response, diffs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// parseResponse parses the response from the AI provider into a ReviewResult
// This reuses the existing parseResponse function from the gemini provider
func parseResponse(response string, diffs []*models.CodeDiff) (*models.ReviewResult, error) {
	// TODO: Reimplement this function from the gemini provider
	// For now, we'll use a simplified version
	return &models.ReviewResult{
		Summary:  "Review completed",
		Comments: []*models.ReviewComment{},
	}, nil
}

// Configure sets up the provider with needed configuration
func (a *AIConnectorsAdapter) Configure(config map[string]interface{}) error {
	// The adapter doesn't need to be configured since it gets its configuration
	// from the connector in the database
	return nil
}

// Name returns the provider's name
func (a *AIConnectorsAdapter) Name() string {
	return string(a.provider)
}
