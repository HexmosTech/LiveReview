package gemini

import (
	"context"

	"github.com/livereview/internal/prompts"
	"github.com/livereview/pkg/models"
)

// TestParseResponse exposes the parseResponse method for testing
func (p *GeminiProvider) TestParseResponse(response string, diffs []*models.CodeDiff) (*models.ReviewResult, error) {
	return p.parseResponse(response, diffs)
}

// TestCallGeminiAPI exposes the callGeminiAPI method for testing
func (p *GeminiProvider) TestCallGeminiAPI(prompt string) (string, error) {
	return p.callGeminiAPI(context.Background(), prompt)
}

// TestConstructPrompt extracts the prompt construction logic for testing
func (p *GeminiProvider) TestConstructPrompt(diffs []*models.CodeDiff) (string, error) {
	// Use centralized prompt building for testing consistency
	promptBuilder := prompts.NewPromptBuilder()
	return promptBuilder.BuildCodeReviewPrompt(diffs), nil
}
