package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/livereview/pkg/models"
)

// GeminiProvider implements the AI Provider interface for Google's Gemini
type GeminiProvider struct {
	apiKey      string
	model       string
	temperature float64
	httpClient  *http.Client
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
		return nil, fmt.Errorf("gemini API key is required")
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
		httpClient:  &http.Client{},
	}, nil
}

// reviewRequest represents a request to the Gemini API
type reviewRequest struct {
	Contents         []requestContent `json:"contents"`
	SafetySettings   []safetySettings `json:"safetySettings,omitempty"`
	GenerationConfig generationConfig `json:"generationConfig"`
}

type requestContent struct {
	Parts []contentPart `json:"parts"`
}

type contentPart struct {
	Text string `json:"text,omitempty"`
}

type safetySettings struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type generationConfig struct {
	Temperature     float64 `json:"temperature"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
	TopK            int     `json:"topK"`
	TopP            float64 `json:"topP"`
}

// reviewResponse represents a response from the Gemini API
type reviewResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason,omitempty"`
	} `json:"promptFeedback"`
}

// ReviewCode takes code diff information and returns a review result with summary and comments
func (p *GeminiProvider) ReviewCode(ctx context.Context, diffs []*models.CodeDiff) (*models.ReviewResult, error) {
	// If no diffs, return an empty review
	if len(diffs) == 0 {
		return &models.ReviewResult{
			Summary:  "# AI Review Summary\n\nNo changes were found in this merge request.",
			Comments: []*models.ReviewComment{},
		}, nil
	}

	fmt.Println("Preparing code review prompt for", len(diffs), "changed files...")

	// Format the diffs for the prompt
	prompt := "You are an expert code reviewer. Review the following code changes and provide detailed feedback.\n\n"
	prompt += "Format your response like this:\n"
	prompt += "# AI Review Summary\n"
	prompt += "<overall summary of the changes>\n\n"
	prompt += "## Files Changed\n"
	prompt += "<list of files changed>\n\n"
	prompt += "## Specific Comments\n"
	prompt += "<specific comments on issues found, including file paths and line numbers>\n\n"
	prompt += "Here are the code changes to review:\n\n"

	// Add each diff to the prompt
	for i, diff := range diffs {
		fmt.Printf("Processing file %d of %d: %s\n", i+1, len(diffs), diff.FilePath)
		prompt += fmt.Sprintf("FILE %d: %s\n", i+1, diff.FilePath)

		if diff.IsNew {
			prompt += "[NEW FILE]\n"
		} else if diff.IsDeleted {
			prompt += "[DELETED FILE]\n"
		} else if diff.IsRenamed {
			prompt += fmt.Sprintf("[RENAMED FROM: %s]\n", diff.OldFilePath)
		}

		// Add hunks
		for _, hunk := range diff.Hunks {
			prompt += hunk.Content + "\n"
		}
		prompt += "\n---\n\n"
	}

	// Make the request to Gemini API
	fmt.Println("Sending request to Gemini API...")
	response, err := p.callGeminiAPI(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to call Gemini API: %w", err)
	}

	fmt.Println("Parsing Gemini API response...")

	// Parse the response to extract summary and comments
	return p.parseResponse(response, diffs)
}

// callGeminiAPI makes a call to the Gemini API
func (p *GeminiProvider) callGeminiAPI(ctx context.Context, prompt string) (string, error) {
	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1/models/%s:generateContent?key=%s", p.model, p.apiKey)

	// Create the request
	reqBody := reviewRequest{
		Contents: []requestContent{
			{
				Parts: []contentPart{
					{
						Text: prompt,
					},
				},
			},
		},
		GenerationConfig: generationConfig{
			Temperature:     p.temperature,
			MaxOutputTokens: 4096,
			TopK:            40,
			TopP:            0.95,
		},
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create and send the HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(reqJSON))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	fmt.Println("Sending HTTP request to Gemini API...")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	fmt.Println("Received response with status:", resp.Status)

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	fmt.Println("Response body length:", len(respBody))

	// Parse the response
	var respObj reviewResponse
	if err := json.Unmarshal(respBody, &respObj); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Check if the response was blocked
	if respObj.PromptFeedback.BlockReason != "" {
		return "", fmt.Errorf("API request was blocked: %s", respObj.PromptFeedback.BlockReason)
	}

	// Check if we got a valid response
	if len(respObj.Candidates) == 0 {
		return "", fmt.Errorf("API returned empty response (no candidates): %s", string(respBody))
	}

	if len(respObj.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("API returned response with no content parts: %s", string(respBody))
	}

	// Return the text from the response
	return respObj.Candidates[0].Content.Parts[0].Text, nil
}

// parseResponse parses the response from Gemini API into a ReviewResult
func (p *GeminiProvider) parseResponse(response string, diffs []*models.CodeDiff) (*models.ReviewResult, error) {
	// Extract summary and comments from the response
	sections := strings.Split(response, "## Specific Comments")

	// Initialize the result with the summary section
	result := &models.ReviewResult{
		Summary:  strings.TrimSpace(sections[0]),
		Comments: []*models.ReviewComment{},
	}

	// If there are specific comments, parse them
	if len(sections) > 1 {
		commentLines := strings.Split(sections[1], "\n")

		var currentComment *models.ReviewComment

		for _, line := range commentLines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Check if this is a new comment
			if strings.HasPrefix(line, "FILE") || strings.HasPrefix(line, "- FILE") {
				// Save the previous comment if it exists
				if currentComment != nil {
					result.Comments = append(result.Comments, currentComment)
				}

				// Start a new comment
				filePathEnd := strings.Index(line, ":")
				lineParts := strings.Split(line, "Line")

				filePath := ""
				lineNum := 1

				if filePathEnd > 0 {
					// Extract file path
					filePath = strings.TrimSpace(line[filePathEnd+1:])
					if len(lineParts) > 1 {
						// Try to extract line number
						fmt.Sscanf(lineParts[1], "%d", &lineNum)
					}
				}

				// Try to match with a real file
				for _, diff := range diffs {
					if strings.Contains(filePath, diff.FilePath) {
						filePath = diff.FilePath
						break
					}
				}

				currentComment = &models.ReviewComment{
					FilePath: filePath,
					Line:     lineNum,
					Content:  "",
					Severity: models.SeverityInfo,
					Category: "review",
				}
			} else if currentComment != nil {
				// This is part of the current comment
				if strings.HasPrefix(line, "Severity:") {
					// Extract severity
					severity := strings.TrimSpace(strings.TrimPrefix(line, "Severity:"))
					switch strings.ToLower(severity) {
					case "critical", "error", "high":
						currentComment.Severity = models.SeverityCritical
					case "warning", "medium":
						currentComment.Severity = models.SeverityWarning
					default:
						currentComment.Severity = models.SeverityInfo
					}
				} else if strings.HasPrefix(line, "Suggestion:") || strings.HasPrefix(line, "- Suggestion:") {
					// Extract suggestion
					suggestion := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "- "), "Suggestion:"))
					currentComment.Suggestions = append(currentComment.Suggestions, suggestion)
				} else {
					// This is part of the comment content
					if currentComment.Content != "" {
						currentComment.Content += "\n"
					}
					currentComment.Content += line
				}
			}
		}

		// Add the last comment if it exists
		if currentComment != nil {
			result.Comments = append(result.Comments, currentComment)
		}
	}

	// If no specific comments were found, add a generic comment on the first file
	if len(result.Comments) == 0 && len(diffs) > 0 {
		var firstFile string
		for _, diff := range diffs {
			if !diff.IsDeleted {
				firstFile = diff.FilePath
				break
			}
		}

		if firstFile != "" {
			result.Comments = append(result.Comments, &models.ReviewComment{
				FilePath: firstFile,
				Line:     1,
				Content:  "No specific issues found in the code changes.",
				Severity: models.SeverityInfo,
				Category: "general",
			})
		}
	}

	fmt.Printf("Review complete: Generated %d comments\n", len(result.Comments))

	return result, nil
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
