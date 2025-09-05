package gemini

import (
	"context"

	"github.com/livereview/internal/prompts"
	vendorpack "github.com/livereview/internal/prompts/vendor"
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
	// Build prompt using Manager.Render + code changes section to mirror production path
	mgr := prompts.NewManager(nil, vendorpack.New())
	base, err := mgr.Render(context.Background(), prompts.Context{OrgID: 0}, "code_review", nil)
	if err != nil {
		return "", err
	}
	return base + "\n\n" + prompts.BuildCodeChangesSection(diffs), nil
}
