package review

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/pkg/models"
)

type helperTransformComment struct {
	Index   int    `json:"index"`
	Content string `json:"content"`
}

type helperTransformResponse struct {
	Comments []helperTransformComment `json:"comments"`
}

func (s *Service) applyHelperStage(ctx context.Context, helperConfig AIConfig, helperMode string, leaderResult *models.ReviewResult) (*models.ReviewResult, *AIStageUsage, error) {
	options, providerName, err := connectorOptionsFromAIConfig(helperConfig, len(leaderResult.Comments))
	if err != nil {
		return nil, nil, err
	}

	connector, err := aiconnectors.NewConnector(ctx, options)
	if err != nil {
		return nil, nil, fmt.Errorf("create helper connector: %w", err)
	}

	prompt := buildHelperPrompt(helperMode, leaderResult)
	response, err := connector.Call(ctx, prompt)
	if err != nil {
		return nil, nil, fmt.Errorf("call helper model: %w", err)
	}

	parsed, err := parseHelperTransformResponse(response, len(leaderResult.Comments))
	if err != nil {
		return nil, nil, err
	}

	// leaderResult.Summary is produced by a separate, full-prose synthesis
	// call (see internal/batch/batch.go's "summary" synthesis step) that
	// concise mode never touches — it's already full-length by the time it
	// gets here, so there's nothing for the helper to expand. Pass it
	// through unchanged instead of round-tripping it through the helper for
	// no benefit.
	transformed := cloneReviewResult(leaderResult)
	for _, comment := range parsed.Comments {
		if comment.Index < 0 || comment.Index >= len(transformed.Comments) {
			return nil, nil, fmt.Errorf("helper response contained out-of-range comment index %d", comment.Index)
		}
		content := strings.TrimSpace(comment.Content)
		if content == "" {
			return nil, nil, fmt.Errorf("helper response returned empty content for comment index %d", comment.Index)
		}
		transformed.Comments[comment.Index].Content = content
	}

	inputTokens, outputTokens, costUSD := estimateUsageFromPromptExchange(prompt, response, providerName, helperConfig.Model)
	usage := &AIStageUsage{
		Stage:          "helper",
		Provider:       providerName,
		Model:          helperConfig.Model,
		PricingVersion: "v1_estimated",
		InputTokens:    &inputTokens,
		OutputTokens:   &outputTokens,
		CostUSD:        &costUSD,
	}

	return transformed, usage, nil
}

func connectorOptionsFromAIConfig(config AIConfig, commentCount int) (aiconnectors.ConnectorOptions, string, error) {
	providerName, _ := config.Config["provider_name"].(string)
	providerType, ok := config.Config["ai_provider_type"].(string)
	if !ok || strings.TrimSpace(providerType) == "" {
		providerType = providerName
	}
	if strings.TrimSpace(providerType) == "" {
		return aiconnectors.ConnectorOptions{}, "", fmt.Errorf("helper AI provider type is missing")
	}

	baseURL, _ := config.Config["base_url"].(string)
	projectID, _ := config.Config["gcp_project_id"].(string)
	location, _ := config.Config["gcp_location"].(string)
	awsAccessKeyID, _ := config.Config["aws_access_key_id"].(string)
	awsRegion, _ := config.Config["aws_region"].(string)

	return aiconnectors.ConnectorOptions{
		Provider:       aiconnectors.Provider(providerType),
		APIKey:         config.APIKey,
		BaseURL:        aiconnectors.ResolveBaseURLForProviderName(providerType, baseURL),
		GCPProjectID:   projectID,
		GCPLocation:    location,
		AWSAccessKeyID: awsAccessKeyID,
		AWSRegion:      awsRegion,
		ModelConfig: aiconnectors.ModelConfig{
			Temperature: config.Temperature,
			MaxTokens:   helperMaxTokens(commentCount),
			Model:       config.Model,
		},
	}, strings.TrimSpace(providerName), nil
}

// helperMaxTokens sizes the helper stage's output budget to the number of
// comments it has to rewrite in a single batched call (see buildHelperPrompt),
// since a flat cap risks truncating the JSON response — and therefore
// silently losing the helper's wording polish via the parse-mismatch
// fallback in applyHelperStage's caller — on reviews with many findings.
func helperMaxTokens(commentCount int) int {
	const (
		base = 1024
		// perComment: rewritten comments in practice run ~60-90 tokens
		// (docs/adaptive_review_overview.html's live examples), plus
		// per-item JSON structure overhead and headroom for longer ones.
		perComment = 220
		ceiling    = 32768
	)
	tokens := base + commentCount*perComment
	if tokens < 4096 {
		return 4096
	}
	if tokens > ceiling {
		return ceiling
	}
	return tokens
}

func buildHelperPrompt(helperMode string, leaderResult *models.ReviewResult) string {
	mode := strings.TrimSpace(helperMode)
	if mode == "" {
		mode = "concise_then_expand"
	}

	// Only send the fields the helper actually needs to expand wording: the
	// index (to map back) and the concise content itself. filePath, line,
	// severity, category, and subcategory don't affect wording expansion and
	// stay untouched on leaderResult, so sending them here would just be
	// tokens billed for no reason.
	comments := make([]map[string]interface{}, 0, len(leaderResult.Comments))
	for idx, comment := range leaderResult.Comments {
		comments = append(comments, map[string]interface{}{
			"index":   idx,
			"content": comment.Content,
		})
	}

	payload, _ := json.MarshalIndent(map[string]interface{}{
		"comments": comments,
	}, "", "  ")

	modeInstruction := "Expand each terse, fragment-style comment into a clear, grammatical review comment without changing its meaning."
	if mode == "polish_only" {
		modeInstruction = "Polish the wording of each review comment while preserving its meaning and roughly its length."
	}

	return strings.TrimSpace(fmt.Sprintf(`You are the Helper model in LiveReview.

Your task is to rewrite the "content" of each comment below without adding or removing findings. You only see the concise text; you do not need file, line, or severity context to do this. There is no review summary in this payload — do not write one.
%s

Rules:
- Keep the exact same number of comments.
- Keep each comment mapped to the same index.
- Do not invent new issues, suggestions, files, lines, or severity changes.
- Return valid JSON only.
- The JSON format must be: {"comments":[{"index":0,"content":"..."}]}

Input review payload:
%s`, modeInstruction, string(payload)))
}

func parseHelperTransformResponse(raw string, expectedComments int) (*helperTransformResponse, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var parsed helperTransformResponse
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return nil, fmt.Errorf("parse helper response: %w", err)
	}

	if len(parsed.Comments) != expectedComments {
		return nil, fmt.Errorf("helper response returned %d comments, expected %d", len(parsed.Comments), expectedComments)
	}

	return &parsed, nil
}

func cloneReviewResult(result *models.ReviewResult) *models.ReviewResult {
	if result == nil {
		return &models.ReviewResult{}
	}

	cloned := &models.ReviewResult{
		Summary:          result.Summary,
		Comments:         make([]*models.ReviewComment, 0, len(result.Comments)),
		InternalComments: make([]*models.ReviewComment, 0, len(result.InternalComments)),
	}

	for _, comment := range result.Comments {
		if comment == nil {
			cloned.Comments = append(cloned.Comments, nil)
			continue
		}
		copied := *comment
		if len(comment.Suggestions) > 0 {
			copied.Suggestions = append([]string(nil), comment.Suggestions...)
		}
		cloned.Comments = append(cloned.Comments, &copied)
	}

	for _, comment := range result.InternalComments {
		if comment == nil {
			cloned.InternalComments = append(cloned.InternalComments, nil)
			continue
		}
		copied := *comment
		if len(comment.Suggestions) > 0 {
			copied.Suggestions = append([]string(nil), comment.Suggestions...)
		}
		cloned.InternalComments = append(cloned.InternalComments, &copied)
	}

	return cloned
}
