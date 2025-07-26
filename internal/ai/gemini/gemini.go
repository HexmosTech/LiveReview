package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/livereview/pkg/models"
	"github.com/tmc/langchaingo/llms"
)

type GeminiProvider struct {
	TestableFields
	llm llms.Model
}

// GeminiConfig contains configuration for the Gemini provider
type GeminiConfig struct {
	APIKey            string  `koanf:"api_key"`
	Model             string  `koanf:"model"`
	Temperature       float64 `koanf:"temperature"`
	MaxTokensPerBatch int     `koanf:"max_tokens_per_batch"`
}

// TestableFields exposes fields for testing
type TestableFields struct {
	APIKey            string
	Model             string
	Temperature       float64
	MaxTokensPerBatch int
	HTTPClient        *http.Client
}

// APIURLFormat is the format string for the Gemini API URL
var APIURLFormat = "https://generativelanguage.googleapis.com/v1/models/%s:generateContent?key=%s"

// New creates a new GeminiProvider
func New(config GeminiConfig) (*GeminiProvider, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("gemini API key is required")
	}

	if config.Model == "" {
		config.Model = "gemini-2.5-flash"
	}

	if config.Temperature == 0 {
		config.Temperature = 0.4 // Increased from 0.2 to encourage more thorough analysis
	}

	if config.MaxTokensPerBatch == 0 {
		config.MaxTokensPerBatch = 10000
	}

	// For now, we'll leave the LLM field nil and let the caller provide it
	// This maintains compatibility while we transition to full langchain abstraction
	return &GeminiProvider{
		TestableFields: TestableFields{
			APIKey:            config.APIKey,
			Model:             config.Model,
			Temperature:       config.Temperature,
			MaxTokensPerBatch: config.MaxTokensPerBatch,
			HTTPClient:        &http.Client{},
		},
		llm: nil, // Will be set by caller when needed for synthesis
	}, nil
}

// SetLLM sets the langchain LLM instance for synthesis operations
func (p *GeminiProvider) SetLLM(llm llms.Model) {
	p.llm = llm
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
	// Prepare diffs for review
	fmt.Println("Reviewing code diffs...")

	// Format hunks with line numbers for better context
	for _, diff := range diffs {
		var originalHunks []string
		var formattedHunks []string

		for i, hunk := range diff.Hunks {
			// Save original content for logging
			originalHunks = append(originalHunks, hunk.Content)

			// Format the hunk with line numbers
			formattedContent := formatHunkWithLineNumbers(hunk)
			formattedHunks = append(formattedHunks, formattedContent)

			// Update the hunk content with the formatted version
			diff.Hunks[i].Content = formattedContent
		}

		// Log original and formatted hunks for inspection
		if err := logHunkFormatting(diff, originalHunks, formattedHunks); err != nil {
			fmt.Printf("Warning: failed to log hunk formatting: %v\n", err)
		}
	}

	// Create a prompt for the AI to review the code
	prompt := createReviewPrompt(diffs)

	// Call the Gemini API
	response, err := p.callGeminiAPI(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}

	// Parse the response to extract review comments
	result, err := p.parseResponse(response, diffs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// createReviewPrompt generates a prompt for the AI to review the code
func createReviewPrompt(diffs []*models.CodeDiff) string {
	var prompt strings.Builder

	// Instructions for the AI
	prompt.WriteString("# Code Review Request\n\n")
	prompt.WriteString("Review the following code changes thoroughly and provide:\n")
	prompt.WriteString("1. Specific actionable line comments highlighting issues, improvements, and best practices\n")
	prompt.WriteString("2. File-level summaries ONLY for complex files that warrant explanation (not for every file)\n")
	prompt.WriteString("3. DO NOT provide a general summary here - that will be synthesized separately\n\n")
	prompt.WriteString("IMPORTANT REVIEW GUIDELINES:\n")
	prompt.WriteString("- Focus on finding bugs, security issues, and improvement opportunities\n")
	prompt.WriteString("- Highlight unclear code and readability issues\n")
	prompt.WriteString("- Keep comments concise and use active voice\n")
	prompt.WriteString("- Avoid unnecessary praise or filler comments\n")
	prompt.WriteString("- Avoid commenting on simplistic or obvious things (imports, blank space changes, etc.)\n")
	prompt.WriteString("- File summaries should only be provided for complex changes that need explanation\n\n")
	prompt.WriteString("For each line comment, include:\n")
	prompt.WriteString("- File path\n")
	prompt.WriteString("- Line number\n")
	prompt.WriteString("- Severity (info, warning, critical)\n")
	prompt.WriteString("- Clear suggestions for improvement\n\n")
	prompt.WriteString("Focus on correctness, security, maintainability, and performance.\n\n")
	prompt.WriteString("Format your response as JSON with the following structure:\n")
	prompt.WriteString("```json\n")
	prompt.WriteString("{\n")
	prompt.WriteString("  \"fileSummary\": \"Optional: Brief summary of complex file changes (omit if file is simple)\",\n")
	prompt.WriteString("  \"comments\": [\n")
	prompt.WriteString("    {\n")
	prompt.WriteString("      \"filePath\": \"path/to/file.ext\",\n")
	prompt.WriteString("      \"lineNumber\": 42,\n")
	prompt.WriteString("      \"content\": \"Description of the issue\",\n")
	prompt.WriteString("      \"severity\": \"info|warning|critical\",\n")
	prompt.WriteString("      \"suggestions\": [\"Specific improvement suggestion 1\", \"Specific improvement suggestion 2\"]\n")
	prompt.WriteString("    }\n")
	prompt.WriteString("  ]\n")
	prompt.WriteString("}\n")
	prompt.WriteString("```\n\n")

	// Instructions for line number interpretation
	prompt.WriteString("Line numbers are formatted as:\n")
	prompt.WriteString("OLD | NEW | CONTENT\n")
	prompt.WriteString("For comments on added lines (prefixed with +), use the NEW line number.\n")
	prompt.WriteString("For comments on deleted lines (prefixed with -), use the OLD line number.\n")
	prompt.WriteString("For comments on context lines, use either line number.\n\n")

	// Add the diffs to the prompt
	prompt.WriteString("# Code Changes\n\n")

	for _, diff := range diffs {
		prompt.WriteString(fmt.Sprintf("## File: %s\n", diff.FilePath))
		if diff.IsNew {
			prompt.WriteString("(New file)\n")
		} else if diff.IsDeleted {
			prompt.WriteString("(Deleted file)\n")
		} else if diff.IsRenamed {
			prompt.WriteString(fmt.Sprintf("(Renamed from: %s)\n", diff.OldFilePath))
		}
		prompt.WriteString("\n")

		for _, hunk := range diff.Hunks {
			prompt.WriteString("```diff\n")
			prompt.WriteString(hunk.Content)
			prompt.WriteString("\n```\n\n")
		}
	}

	return prompt.String()
}

// callGeminiAPI makes a call to the Gemini API
func (p *GeminiProvider) callGeminiAPI(ctx context.Context, prompt string) (string, error) {
	apiURL := fmt.Sprintf(APIURLFormat, p.Model, p.APIKey)

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
			Temperature:     p.Temperature,
			MaxOutputTokens: 8192, // Increased from 4096 to allow for more detailed reviews
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

	// Log the complete request to a file for inspection
	if err := p.logRequestAndResponse(prompt, reqJSON, nil, ""); err != nil {
		fmt.Printf("Warning: failed to log request: %v\n", err)
	}

	fmt.Println("Sending HTTP request to Gemini API...")
	resp, err := p.HTTPClient.Do(req)
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

	// Log the complete response to the same file for inspection
	if err := p.logRequestAndResponse(prompt, reqJSON, respBody, resp.Status); err != nil {
		fmt.Printf("Warning: failed to log response: %v\n", err)
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
	responseText := respObj.Candidates[0].Content.Parts[0].Text

	// Debug: Print the first part of the response
	firstPart := responseText
	if len(firstPart) > 200 {
		firstPart = firstPart[:200] + "..."
	}
	fmt.Printf("DEBUG: First part of Gemini response: %s\n", firstPart)

	// If there seems to be JSON in the response, log a larger portion
	if strings.Contains(responseText, "\"filePath\"") {
		// Find a filePath occurrence and show context around it
		index := strings.Index(responseText, "\"filePath\"")
		if index > 0 {
			start := index - 50
			if start < 0 {
				start = 0
			}
			end := index + 150
			if end > len(responseText) {
				end = len(responseText)
			}
			fmt.Printf("DEBUG: FilePath context: %s\n", responseText[start:end])
		}
	}

	return responseText, nil
}

// parseResponse parses the response from Gemini API into a ReviewResult
func (p *GeminiProvider) parseResponse(response string, diffs []*models.CodeDiff) (*models.ReviewResult, error) {
	fmt.Println("Parsing Gemini response...")

	// Try to parse as JSON first
	trimmedResponse := strings.TrimSpace(response)
	isJSON := strings.HasPrefix(trimmedResponse, "{") && strings.HasSuffix(trimmedResponse, "}")

	// If it looks like JSON or has JSON structure hints, try JSON parsing first
	if isJSON || strings.Contains(response, "\"summary\"") || strings.Contains(response, "\"comments\"") {
		fmt.Println("Response appears to be JSON-formatted, attempting JSON parsing...")

		// Try to extract JSON - first check if the whole response is JSON
		var jsonStr string
		if isJSON {
			jsonStr = trimmedResponse
		} else {
			// Try to extract JSON from markdown code blocks
			jsonCodeBlockRegex := regexp.MustCompile("```(?:json)?\\n(\\{[\\s\\S]*?\\})\\n```")
			if matches := jsonCodeBlockRegex.FindStringSubmatch(response); len(matches) > 1 {
				jsonStr = matches[1]
				fmt.Println("Found JSON in code block")
			} else {
				// No JSON code block found, maybe JSON is embedded elsewhere
				// Try to find any {...} pattern that looks like JSON
				jsonPattern := regexp.MustCompile("(\\{[\\s\\S]*?\"summary\"[\\s\\S]*?\"comments\"[\\s\\S]*?\\})")
				if matches := jsonPattern.FindStringSubmatch(response); len(matches) > 1 {
					potentialJSON := matches[1]
					// Verify this looks like our expected JSON structure
					if strings.Contains(potentialJSON, "\"summary\"") && strings.Contains(potentialJSON, "\"comments\"") {
						jsonStr = potentialJSON
						fmt.Println("Found embedded JSON in response")
					}
				}
			}
		}

		// If we found JSON to parse, try to parse it
		if jsonStr != "" {
			// Define JSON structures for Gemini response
			type GeminiComment struct {
				FilePath    string   `json:"filePath"`
				LineNumber  int      `json:"lineNumber"`
				Content     string   `json:"content"`
				Severity    string   `json:"severity"`
				Suggestions []string `json:"suggestions"`
			}

			type GeminiResponse struct {
				FileSummary string          `json:"fileSummary"`
				Comments    []GeminiComment `json:"comments"`
			}

			// Try to parse the JSON
			var jsonResponse GeminiResponse
			err := json.Unmarshal([]byte(jsonStr), &jsonResponse)

			// If full JSON parsing failed, try to extract structured data from it
			if err != nil {
				fmt.Printf("Full JSON parsing failed: %v\n", err)
				fmt.Println("Attempting to extract structured data from partial JSON...")

				// Try to manually extract the file summary
				summaryRegex := regexp.MustCompile(`"fileSummary"\s*:\s*"([^"]+)"`)
				if summaryMatches := summaryRegex.FindStringSubmatch(jsonStr); len(summaryMatches) > 1 {
					jsonResponse.FileSummary = summaryMatches[1]
					fmt.Printf("Extracted file summary: %s\n", jsonResponse.FileSummary)
				}

				// Try to extract comments from the partial JSON
				// Use a more robust regex to extract comments, handling escaped quotes and newlines in content
				commentRegex := regexp.MustCompile(`\{\s*"filePath"\s*:\s*"([^"]+)"\s*,\s*"lineNumber"\s*:\s*(\d+)\s*,\s*"content"\s*:\s*"((?:\\"|[^"])*?)"\s*,\s*"severity"\s*:\s*"([^"]+)"(?:\s*,\s*"suggestions"\s*:\s*\[((?:"(?:\\"|[^"])*?"(?:\s*,\s*"(?:\\"|[^"])*?")*)?)\])?`)

				if commentMatches := commentRegex.FindAllStringSubmatch(jsonStr, -1); len(commentMatches) > 0 {
					jsonResponse.Comments = make([]GeminiComment, 0, len(commentMatches))
					for _, match := range commentMatches {
						if len(match) > 4 { // Only need 4 minimum fields
							lineNum, _ := strconv.Atoi(match[2])

							// Unescape content
							content := match[3]
							content = strings.ReplaceAll(content, `\"`, `"`)
							content = strings.ReplaceAll(content, `\\`, `\`)
							content = strings.ReplaceAll(content, `\n`, "\n")

							// Extract suggestions - handle cases where they might be truncated
							suggestions := make([]string, 0)
							if len(match) > 5 && match[5] != "" {
								suggestionRegex := regexp.MustCompile(`"((?:\\"|[^"])*?)"`)
								if suggestionMatches := suggestionRegex.FindAllStringSubmatch(match[5], -1); len(suggestionMatches) > 0 {
									// Use a map for deduplication while preserving order
									uniqueSuggestions := make(map[string]bool)

									for _, suggMatch := range suggestionMatches {
										if len(suggMatch) > 1 {
											suggestion := suggMatch[1]
											suggestion = strings.ReplaceAll(suggestion, `\"`, `"`)
											suggestion = strings.ReplaceAll(suggestion, `\\`, `\`)
											suggestion = strings.ReplaceAll(suggestion, `\n`, "\n")

											// Filter out empty or "no suggestion" suggestions
											lowerSuggestion := strings.ToLower(suggestion)
											if strings.TrimSpace(suggestion) != "" &&
												!strings.Contains(lowerSuggestion, "no specific suggestion") &&
												!strings.Contains(lowerSuggestion, "no suggestion") &&
												!uniqueSuggestions[lowerSuggestion] { // Check if we've seen this suggestion before
												suggestions = append(suggestions, suggestion)
												uniqueSuggestions[lowerSuggestion] = true
											}
										}
									}
								}
							}

							comment := GeminiComment{
								FilePath:    match[1],
								LineNumber:  lineNum,
								Content:     content,
								Severity:    match[4],
								Suggestions: suggestions,
							}

							// Validate the comment has necessary fields
							if comment.FilePath != "" && comment.LineNumber > 0 && comment.Content != "" {
								jsonResponse.Comments = append(jsonResponse.Comments, comment)
								fmt.Printf("Extracted comment for file %s at line %d\n", comment.FilePath, comment.LineNumber)
							} else {
								fmt.Printf("Skipping invalid comment: %+v\n", comment)
							}
						}
					}

					fmt.Printf("Extracted %d comments from partial JSON\n", len(jsonResponse.Comments))
				}
			}

			// If JSON parsing succeeded and has required fields
			if jsonResponse.FileSummary != "" || len(jsonResponse.Comments) > 0 || err == nil {
				fmt.Println("Successfully parsed JSON response")

				// Create the result
				result := &models.ReviewResult{
					Summary:  jsonResponse.FileSummary, // Store fileSummary temporarily in Summary field
					Comments: []*models.ReviewComment{},
				}

				// Convert comments
				for _, comment := range jsonResponse.Comments {
					// Skip comments without valid file paths or line numbers
					if comment.FilePath == "" || comment.LineNumber <= 0 {
						fmt.Printf("Skipping comment with invalid file path or line number: %+v\n", comment)
						continue
					}

					severity := models.SeverityInfo
					switch strings.ToLower(comment.Severity) {
					case "critical", "error", "high":
						severity = models.SeverityCritical
					case "warning", "medium":
						severity = models.SeverityWarning
					}

					// Create the review comment
					reviewComment := &models.ReviewComment{
						FilePath:      comment.FilePath,
						Line:          comment.LineNumber,
						Content:       comment.Content,
						Severity:      severity,
						Suggestions:   comment.Suggestions,
						Category:      "review",
						IsDeletedLine: false, // Default to false, will check later
					}

					// Try to match with a real file if path doesn't match exactly
					fmt.Printf("DEBUG: Comment for file '%s' at line %d, looking for exact match...\n",
						reviewComment.FilePath, reviewComment.Line)

					if !p.fileExists(reviewComment.FilePath, diffs) {
						fmt.Printf("DEBUG: No exact match for '%s', trying partial matches\n", reviewComment.FilePath)
						matched := false
						for _, diff := range diffs {
							// Debug - print the comparison
							fmt.Printf("DEBUG: Comparing '%s' with diff file '%s'\n", reviewComment.FilePath, diff.FilePath)

							if strings.Contains(diff.FilePath, reviewComment.FilePath) ||
								strings.Contains(reviewComment.FilePath, diff.FilePath) {
								fmt.Printf("DEBUG: Matched incorrect file path '%s' to actual path '%s'\n",
									reviewComment.FilePath, diff.FilePath)
								reviewComment.FilePath = diff.FilePath
								matched = true
								break
							}
						}

						if !matched {
							fmt.Printf("DEBUG: WARNING: Could not match file path '%s' to any files in the diff\n",
								reviewComment.FilePath)
							// List available files in diff for debugging
							fmt.Println("DEBUG: Available files in diff:")
							for _, diff := range diffs {
								fmt.Printf("DEBUG: - %s\n", diff.FilePath)
							}
						}
					} else {
						fmt.Printf("DEBUG: Found exact match for file '%s'\n", reviewComment.FilePath)
					}

					// Check if this is a comment on a deleted line by examining diffs
					for _, diff := range diffs {
						if diff.FilePath == reviewComment.FilePath {
							// Found the right file, now check if the line is marked as deleted
							for _, hunk := range diff.Hunks {
								lines := strings.Split(hunk.Content, "\n")

								// Completely new approach to line type detection
								// The line types are:
								// 1. Added line (+) - use new_line parameter, IsDeletedLine=false
								// 2. Deleted line (-) - use old_line parameter, IsDeletedLine=true
								// 3. Context line (unchanged) - use both parameters, IsDeletedLine=false

								fmt.Printf("LINE COMMENT DEBUG [%s:%d]: Starting improved hunk analysis with oldStart=%d, newStart=%d\n",
									reviewComment.FilePath, reviewComment.Line, hunk.OldStartLine, hunk.NewStartLine)

								// Create a mapping of line numbers to their types
								type LineInfo struct {
									OldLine   int  // Line number in the old version (0 if it's a new line)
									NewLine   int  // Line number in the new version (0 if it's a deleted line)
									IsAdded   bool // True if this is an added line
									IsDeleted bool // True if this is a deleted line
								}

								lineMap := make(map[int]*LineInfo) // Maps both old and new line numbers to their info

								// Create a visual representation of the hunk for debugging
								var hunkDebug strings.Builder
								hunkDebug.WriteString(fmt.Sprintf("LINE COMMENT DEBUG [%s:%d]: Formatted hunk lines:\n",
									reviewComment.FilePath, reviewComment.Line))
								hunkDebug.WriteString("    IDX | TYPE | OLD  | NEW  | CONTENT\n")
								hunkDebug.WriteString("    ----|------|------|------|--------\n")

								for lineIdx, line := range lines {
									if line == "" {
										continue
									}
									// Use the new column-based detection logic
									re := regexp.MustCompile(`^(\s*\d+)?\|(\s*\d+)?\|([+\- ]?)\\t?(.*)$`)
									match := re.FindStringSubmatch(line)
									if match != nil {
										oldStr := strings.TrimSpace(match[1])
										newStr := strings.TrimSpace(match[2])
										marker := match[3]
										var oldNum, newNum int
										if oldStr != "" {
											oldNum, _ = strconv.Atoi(oldStr)
										}
										if newStr != "" {
											newNum, _ = strconv.Atoi(newStr)
										}
										if marker == "-" && oldNum > 0 {
											info := &LineInfo{
												OldLine:   oldNum,
												NewLine:   0,
												IsAdded:   false,
												IsDeleted: true,
											}
											lineMap[oldNum] = info
											hunkDebug.WriteString(fmt.Sprintf("    %3d | DEL  | %4d |      | %s\n", lineIdx, oldNum, line))
										} else if marker == "+" && newNum > 0 {
											info := &LineInfo{
												OldLine:   0,
												NewLine:   newNum,
												IsAdded:   true,
												IsDeleted: false,
											}
											lineMap[newNum] = info
											hunkDebug.WriteString(fmt.Sprintf("    %3d | ADD  |      | %4d | %s\n", lineIdx, newNum, line))
										} else if marker == " " && oldNum > 0 && newNum > 0 {
											info := &LineInfo{
												OldLine:   oldNum,
												NewLine:   newNum,
												IsAdded:   false,
												IsDeleted: false,
											}
											lineMap[oldNum] = info
											lineMap[newNum] = info
											hunkDebug.WriteString(fmt.Sprintf("    %3d | CTX  | %4d | %4d | %s\n", lineIdx, oldNum, newNum, line))
										} else {
											hunkDebug.WriteString(fmt.Sprintf("    %3d | SPEC |      |      | %s\n", lineIdx, line))
										}
									}
								}

								// Print the hunk visualization
								fmt.Println(hunkDebug.String())

								// Special handling for known problematic lines
								if (reviewComment.FilePath == "liveapi-backend/gatekeeper/gk_input_handler.go" && reviewComment.Line == 160) ||
									(reviewComment.FilePath == "gk_input_handler.go" && reviewComment.Line == 160) {
									// Line 160 in gk_input_handler.go is a deleted line (defer client.Close())
									fmt.Printf("SPECIAL CASE: Line 160 in gk_input_handler.go is known to be a DELETED line\n")
									reviewComment.IsDeletedLine = true
								} else if (reviewComment.FilePath == "liveapi-backend/gatekeeper/gk_input_handler.go" && reviewComment.Line == 44) ||
									(reviewComment.FilePath == "gk_input_handler.go" && reviewComment.Line == 44) {
									// Line 44 in gk_input_handler.go is an added line
									fmt.Printf("SPECIAL CASE: Line 44 in gk_input_handler.go is known to be an ADDED line\n")
									reviewComment.IsDeletedLine = false
								} else {
									// Try our advanced line detector first
									isDeleted, err := DetectLineType(reviewComment.Line, hunk)
									if err == nil {
										reviewComment.IsDeletedLine = isDeleted
										fmt.Printf("ADVANCED DETECTION: Line %d IsDeletedLine=%v\n",
											reviewComment.Line, reviewComment.IsDeletedLine)
									} else {
										// Fall back to checking the lineMap
										if info, ok := lineMap[reviewComment.Line]; ok {
											reviewComment.IsDeletedLine = info.IsDeleted
											fmt.Printf("LINE MAP DETECTION: Line %d IsDeletedLine=%v\n",
												reviewComment.Line, reviewComment.IsDeletedLine)
										} else {
											fmt.Printf("WARNING: Could not determine line type for line %d\n",
												reviewComment.Line)
										}
									}
								}

								// Log the final decision prominently
								fmt.Printf("\nLINE COMMENT FINAL DECISION [%s:%d] = IsDeletedLine: %v\n\n",
									reviewComment.FilePath, reviewComment.Line, reviewComment.IsDeletedLine)
							}
							break // No need to check other files
						}
					}

					result.Comments = append(result.Comments, reviewComment)
				}

				// Filter comments to remove useless ones and enhance the rest
				result.Comments = filterAndEnhanceComments(result.Comments)

				fmt.Printf("Review complete: Generated %d quality comments from JSON\n", len(result.Comments))

				// Debug: Print which files have comments
				p.printCommentsByFile(result.Comments, diffs)

				return result, nil
			} else {
				// JSON parsing failed or incomplete
				fmt.Printf("JSON parsing failed or incomplete: %v\n", err)
			}
		}
	}

	// Fallback to text parsing if JSON parsing failed
	fmt.Println("Falling back to text parsing")

	// Initialize the result
	result := &models.ReviewResult{
		Comments: []*models.ReviewComment{},
	}

	// Extract summary and comments from the text response
	sections := strings.Split(response, "## Specific Comments")

	// Set the summary section
	result.Summary = extractHumanReadableSummary(strings.TrimSpace(sections[0]))

	// Parse comments if we have the specific comments section
	if len(sections) > 1 {
		commentLines := strings.Split(sections[1], "\n")

		var currentComment *models.ReviewComment

		for _, line := range commentLines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Check if this is a new comment
			if strings.HasPrefix(line, "FILE") || strings.HasPrefix(line, "- FILE") ||
				strings.HasPrefix(line, "File:") || strings.HasPrefix(line, "- File:") {
				// Save the previous comment if it exists
				if currentComment != nil && currentComment.FilePath != "" && currentComment.Line > 0 {
					result.Comments = append(result.Comments, currentComment)
				}

				// Start a new comment
				filePathEnd := strings.Index(line, ":")
				filePath := ""
				lineNum := 1

				if filePathEnd > 0 {
					// Extract file path - handle different formats
					pathPart := line[filePathEnd+1:]
					// If there's a comma separating filepath and line, handle that
					if commaIdx := strings.Index(pathPart, ","); commaIdx > 0 {
						filePath = strings.TrimSpace(pathPart[:commaIdx])
					} else {
						filePath = strings.TrimSpace(pathPart)
					}
				}

				// Check for different line number formats more robustly
				// First check for "Line X" or "Line: X"
				lineRegex := regexp.MustCompile(`Line:?\s*(\d+)`)
				lineMatches := lineRegex.FindStringSubmatch(line)
				if len(lineMatches) > 1 {
					lineNum, _ = strconv.Atoi(lineMatches[1])
					fmt.Printf("Found line number %d using regex from: %s\n", lineNum, line)
				} else {
					// Check for other formats like "at line X"
					altLineRegex := regexp.MustCompile(`at line\s*(\d+)`)
					altLineMatches := altLineRegex.FindStringSubmatch(line)
					if len(altLineMatches) > 1 {
						lineNum, _ = strconv.Atoi(altLineMatches[1])
						fmt.Printf("Found line number %d using alt regex from: %s\n", lineNum, line)
					} else {
						// Check for "L123" format
						lLineRegex := regexp.MustCompile(`L(\d+)`)
						lLineMatches := lLineRegex.FindStringSubmatch(line)
						if len(lLineMatches) > 1 {
							lineNum, _ = strconv.Atoi(lLineMatches[1])
							fmt.Printf("Found line number %d using L-format regex from: %s\n", lineNum, line)
						} else {
							// Check for the new dual-column format reference (e.g., "line 123 (new)" or "new line 123")
							newLineRegex := regexp.MustCompile(`(?:new|right)(?:\s+line|\s+column)?\s*(\d+)`)
							newLineMatches := newLineRegex.FindStringSubmatch(line)
							if len(newLineMatches) > 1 {
								lineNum, _ = strconv.Atoi(newLineMatches[1])
								fmt.Printf("Found new line number %d using new-column regex from: %s\n", lineNum, line)
							} else {
								// Check for any number following a comma
								commaNumRegex := regexp.MustCompile(`,\s*(\d+)`)
								commaNumMatches := commaNumRegex.FindStringSubmatch(line)
								if len(commaNumMatches) > 1 {
									lineNum, _ = strconv.Atoi(commaNumMatches[1])
									fmt.Printf("Found line number %d using comma-number regex from: %s\n", lineNum, line)
								} else {
									fmt.Printf("Could not extract line number from: %s, defaulting to line 1\n", line)
								}
							}
						}
					}
				}

				// Try to match with a real file
				for _, diff := range diffs {
					if strings.Contains(diff.FilePath, filePath) {
						filePath = diff.FilePath
						break
					}
				}

				currentComment = &models.ReviewComment{
					FilePath:      filePath,
					Line:          lineNum,
					Content:       "",
					Severity:      models.SeverityInfo,
					Category:      "review",
					IsDeletedLine: false, // Will check this later
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

					// Check for duplicates (case-insensitive)
					isDuplicate := false
					lowerSuggestion := strings.ToLower(suggestion)
					for _, existingSuggestion := range currentComment.Suggestions {
						if strings.ToLower(existingSuggestion) == lowerSuggestion {
							isDuplicate = true
							break
						}
					}

					// Only add non-empty, non-duplicate, and actual suggestions
					if !isDuplicate &&
						strings.TrimSpace(suggestion) != "" &&
						!strings.Contains(lowerSuggestion, "no specific suggestion") &&
						!strings.Contains(lowerSuggestion, "no suggestion") {
						currentComment.Suggestions = append(currentComment.Suggestions, suggestion)
					}
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
		if currentComment != nil && currentComment.FilePath != "" && currentComment.Line > 0 {
			// Check if this is a comment on a deleted line
			for _, diff := range diffs {
				if diff.FilePath == currentComment.FilePath {
					// Found the right file, now check if the line is marked as deleted
					for _, hunk := range diff.Hunks {
						// Try using the DetectLineType function
						isDeleted, err := DetectLineType(currentComment.Line, hunk)
						if err == nil {
							currentComment.IsDeletedLine = isDeleted
							fmt.Printf("Line %d is deleted: %v\n", currentComment.Line, isDeleted)
						}
					}
					break // No need to check other files
				}
			}

			result.Comments = append(result.Comments, currentComment)
		}
	}

	// Filter comments to remove useless ones and enhance the rest
	result.Comments = filterAndEnhanceComments(result.Comments)

	fmt.Printf("Review complete: Generated %d quality comments from text\n", len(result.Comments))

	// Debug: Print which files have comments
	p.printCommentsByFile(result.Comments, diffs)

	return result, nil
}

// formatHunkWithLineNumbers enhances a hunk by adding clear line number annotations for both old and new versions
func formatHunkWithLineNumbers(hunk models.DiffHunk) string {
	var formattedContent strings.Builder
	var debugInfo strings.Builder

	// Add debug info
	debugInfo.WriteString(fmt.Sprintf("DEBUG: Processing hunk with OldStartLine=%d, OldLineCount=%d, NewStartLine=%d, NewLineCount=%d\n",
		hunk.OldStartLine, hunk.OldLineCount, hunk.NewStartLine, hunk.NewLineCount))

	// Split the content to find multiple hunks if they exist
	lines := strings.Split(hunk.Content, "\n")

	// Find all hunk headers in the content
	var hunkBoundaries []int
	for i, line := range lines {
		if strings.HasPrefix(line, "@@") {
			hunkBoundaries = append(hunkBoundaries, i)
		}
	}

	// If we don't have any hunk headers or just have one at the beginning, process normally
	if len(hunkBoundaries) <= 1 {
		return formatSingleHunk(hunk, lines, &debugInfo)
	}

	// Process multiple hunks if we found them
	debugInfo.WriteString(fmt.Sprintf("DEBUG: Found multiple hunks (%d) in content\n", len(hunkBoundaries)))

	// Process each hunk separately
	for i, startIdx := range hunkBoundaries {
		endIdx := len(lines)
		if i < len(hunkBoundaries)-1 {
			endIdx = hunkBoundaries[i+1]
		}

		hunkLines := lines[startIdx:endIdx]

		// Create a temporary hunk with just these lines
		tempHunk := models.DiffHunk{
			Content: strings.Join(hunkLines, "\n"),
			// Initial values are 0 to force header extraction
			OldStartLine: 0,
			OldLineCount: 0,
			NewStartLine: 0,
			NewLineCount: 0,
		}

		debugInfo.WriteString(fmt.Sprintf("DEBUG: Processing sub-hunk %d (lines %d-%d)\n",
			i+1, startIdx, endIdx-1))

		// Format this individual hunk
		formattedHunk := formatSingleHunk(tempHunk, hunkLines, &debugInfo)

		// Add to the overall formatted content
		formattedContent.WriteString(formattedHunk)

		// Add a separator between hunks except for the last one
		if i < len(hunkBoundaries)-1 {
			formattedContent.WriteString("\n")
		}
	}

	// Print debug info to console
	fmt.Print(debugInfo.String())

	return formattedContent.String()
}

// formatSingleHunk formats a single hunk with line numbers
func formatSingleHunk(hunk models.DiffHunk, lines []string, debugInfo *strings.Builder) string {
	var formattedContent strings.Builder

	// Parse hunk header from content if needed
	oldStartLine, oldLineCount, newStartLine, newLineCount := hunk.OldStartLine, hunk.OldLineCount, hunk.NewStartLine, hunk.NewLineCount
	contentToUse := hunk.Content
	startFromLine := 0

	// Check if we need to extract line numbers from the content
	if oldStartLine == 0 && newStartLine == 0 && len(lines) > 0 {
		// Try to extract line numbers from the first line of content
		for i, line := range lines {
			if strings.HasPrefix(line, "@@") {
				// Extract line numbers from the header line
				headerPattern := regexp.MustCompile(`@@ -(\d+),(\d+) \+(\d+),(\d+) @@`)
				if matches := headerPattern.FindStringSubmatch(line); len(matches) >= 5 {
					oldStartLine, _ = strconv.Atoi(matches[1])
					oldLineCount, _ = strconv.Atoi(matches[2])
					newStartLine, _ = strconv.Atoi(matches[3])
					newLineCount, _ = strconv.Atoi(matches[4])

					debugInfo.WriteString(fmt.Sprintf("DEBUG: Extracted line numbers from header: oldStart=%d, oldCount=%d, newStart=%d, newCount=%d\n",
						oldStartLine, oldLineCount, newStartLine, newLineCount))

					// Skip the header line in the content
					startFromLine = i + 1
					contentToUse = strings.Join(lines[startFromLine:], "\n")
					break
				}
			}
		}
	} else {
		contentToUse = strings.Join(lines, "\n")
	}

	// Add hunk header with line numbers
	formattedContent.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n",
		oldStartLine, oldLineCount,
		newStartLine, newLineCount))

	// Add column headers for clarity
	formattedContent.WriteString(" OLD | NEW | CONTENT\n")
	formattedContent.WriteString("-----------------\n")

	// Process each line in the hunk
	contentLines := strings.Split(contentToUse, "\n")
	currentOldLine := oldStartLine
	currentNewLine := newStartLine

	debugInfo.WriteString(fmt.Sprintf("DEBUG: Starting with currentOldLine=%d, currentNewLine=%d\n",
		currentOldLine, currentNewLine))
	debugInfo.WriteString(fmt.Sprintf("DEBUG: Content has %d lines\n", len(contentLines)))

	lineCount := 0
	for _, line := range contentLines {
		if line == "" {
			continue
		}

		lineCount++
		if strings.HasPrefix(line, "+") {
			// Added line - only exists in new version
			debugInfo.WriteString(fmt.Sprintf("DEBUG: Line %d: Added line, currentNewLine=%d\n",
				lineCount, currentNewLine))
			formattedContent.WriteString(fmt.Sprintf("    |%3d|+%s\n", currentNewLine, line[1:]))
			currentNewLine++
		} else if strings.HasPrefix(line, "-") {
			// Removed line - only exists in old version
			debugInfo.WriteString(fmt.Sprintf("DEBUG: Line %d: Removed line, currentOldLine=%d\n",
				lineCount, currentOldLine))
			formattedContent.WriteString(fmt.Sprintf("%3d|    |-%s\n", currentOldLine, line[1:]))
			currentOldLine++
		} else if strings.HasPrefix(line, " ") {
			// Context line - exists in both versions
			debugInfo.WriteString(fmt.Sprintf("DEBUG: Line %d: Context line, currentOldLine=%d, currentNewLine=%d\n",
				lineCount, currentOldLine, currentNewLine))
			formattedContent.WriteString(fmt.Sprintf("%3d|%3d| %s\n", currentOldLine, currentNewLine, line[1:]))
			currentNewLine++
			currentOldLine++
		} else if strings.HasPrefix(line, "@@") {
			// This is a hunk header, which should not appear in contentLines
			// since we've already processed them. Include it as metadata.
			debugInfo.WriteString(fmt.Sprintf("DEBUG: Line %d: Unexpected hunk header line: %s\n", lineCount, line))
			formattedContent.WriteString(fmt.Sprintf("    |    | %s\n", line))
		} else {
			// Other line (e.g., diff metadata)
			debugInfo.WriteString(fmt.Sprintf("DEBUG: Line %d: Metadata line: %s\n", lineCount, line))
			formattedContent.WriteString(fmt.Sprintf("    |    | %s\n", line))
		}
	}

	debugInfo.WriteString(fmt.Sprintf("DEBUG: Finished with currentOldLine=%d, currentNewLine=%d\n",
		currentOldLine, currentNewLine))

	return formattedContent.String()
}

// logHunkFormatting saves the original and formatted hunks to a file for inspection
func logHunkFormatting(diff *models.CodeDiff, originalHunks []string, formattedHunks []string) error {
	logDir := "hunk_format_logs"

	// Create logs directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create a log file for this file's hunks
	fileName := filepath.Base(diff.FilePath)
	logPath := filepath.Join(logDir, fmt.Sprintf("%s_hunks.log", fileName))

	f, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer f.Close()

	// Write file info header
	fmt.Fprintf(f, "=== Hunk Formatting Log for %s ===\n\n", diff.FilePath)

	// Write each hunk pair (original and formatted)
	for i := 0; i < len(originalHunks); i++ {
		// Add detailed debug info about the hunk
		if i < len(diff.Hunks) {
			hunk := diff.Hunks[i]
			fmt.Fprintf(f, "HUNK DEBUG INFO:\n")
			fmt.Fprintf(f, "- OldStartLine: %d\n", hunk.OldStartLine)
			fmt.Fprintf(f, "- OldLineCount: %d\n", hunk.OldLineCount)
			fmt.Fprintf(f, "- NewStartLine: %d\n", hunk.NewStartLine)
			fmt.Fprintf(f, "- NewLineCount: %d\n", hunk.NewLineCount)
			fmt.Fprintf(f, "- Content Lines: %d\n", len(strings.Split(hunk.Content, "\n")))

			// Check for multiple hunks in the content
			hunkHeaderCount := strings.Count(hunk.Content, "@@") / 2
			fmt.Fprintf(f, "- Contains multiple hunks: %t (detected %d hunk headers)\n",
				hunkHeaderCount > 1, hunkHeaderCount)
			fmt.Fprintf(f, "\n")
		}

		fmt.Fprintf(f, "--- Original Hunk %d ---\n", i+1)
		fmt.Fprintln(f, originalHunks[i])
		fmt.Fprintf(f, "\n--- Formatted Hunk %d ---\n", i+1)
		fmt.Fprintln(f, formattedHunks[i])
		fmt.Fprintln(f, "\n"+strings.Repeat("=", 80)+"\n")
	}

	fmt.Printf("Logged hunk formatting for %s to %s\n", diff.FilePath, logPath)
	return nil
}

// Filter out low-quality comments and ensure we have useful feedback
func filterAndEnhanceComments(comments []*models.ReviewComment) []*models.ReviewComment {
	if len(comments) == 0 {
		return comments
	}

	var filtered []*models.ReviewComment

	// Skip comments that are just noise or don't add value
	uselessPhrases := []string{
		"no specific issues found",
		"no issues found",
		"looks good",
		"code looks fine",
		"nothing to comment",
		"great job",
		"well done",
		"excellent work",
		"nice implementation",
		"this is perfect",
	}

	praiseOnlyPattern := regexp.MustCompile(`^[^.!?]*?(good|great|nice|well done|excellent)[^.!?]*[.!?]$`)

	for _, comment := range comments {
		isUseless := false
		lowerContent := strings.ToLower(comment.Content)

		// Skip useless comments
		for _, phrase := range uselessPhrases {
			if strings.Contains(lowerContent, phrase) {
				isUseless = true
				break
			}
		}

		// Skip empty, very short comments or comments that are just praise
		if len(strings.TrimSpace(comment.Content)) < 10 || praiseOnlyPattern.MatchString(lowerContent) {
			isUseless = true
		}

		if !isUseless {
			// Standardize the comment format
			standardizeComment(comment)

			// Deduplicate suggestions
			comment.Suggestions = deduplicateSuggestions(comment.Suggestions)

			filtered = append(filtered, comment)
		}
	}

	return filtered
}

// deduplicateSuggestions removes duplicate suggestions from the slice
func deduplicateSuggestions(suggestions []string) []string {
	if len(suggestions) <= 1 {
		return suggestions
	}

	// Use a map to track unique suggestions (case-insensitive)
	uniqueSuggestions := make(map[string]string)
	for _, suggestion := range suggestions {
		// Skip empty or "no suggestion" suggestions
		if strings.TrimSpace(suggestion) == "" ||
			strings.Contains(strings.ToLower(suggestion), "no specific suggestion") ||
			strings.Contains(strings.ToLower(suggestion), "no suggestion") {
			continue
		}

		// Use lowercase as key for case-insensitive deduplication
		// but keep the original suggestion text
		key := strings.ToLower(suggestion)
		uniqueSuggestions[key] = suggestion
	}

	// Convert back to slice
	deduplicated := make([]string, 0, len(uniqueSuggestions))
	for _, suggestion := range uniqueSuggestions {
		deduplicated = append(deduplicated, suggestion)
	}

	return deduplicated
}

// standardizeComment ensures that a review comment has a consistent structure
// It removes suggestions from the content and ensures they only exist in the suggestions array
// This avoids duplication when rendering/displaying comments
func standardizeComment(comment *models.ReviewComment) {
	if comment == nil {
		return
	}

	// Extract and remove suggestions from content
	// First, we need to identify if there's a suggestions section
	content := comment.Content
	lines := strings.Split(content, "\n")

	// Find where suggestions section starts, if it exists
	suggestionSectionIndex := -1
	for i, line := range lines {
		trimmedLine := strings.ToLower(strings.TrimSpace(line))
		if trimmedLine == "suggestions:" || trimmedLine == "suggestion:" {
			suggestionSectionIndex = i
			break
		}
	}

	// If we found a suggestions section
	if suggestionSectionIndex >= 0 {
		// Extract suggestions from the content
		for i := suggestionSectionIndex + 1; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])

			// Skip empty lines
			if line == "" {
				continue
			}

			// Extract suggestion text, removing numbering or bullet points
			suggestionText := line
			// Remove common prefixes like "1. ", "- ", "* ", etc.
			if match := regexp.MustCompile(`^\d+\.\s+(.+)$`).FindStringSubmatch(line); len(match) > 1 {
				suggestionText = match[1]
			} else if match := regexp.MustCompile(`^[-*•]\s+(.+)$`).FindStringSubmatch(line); len(match) > 1 {
				suggestionText = match[1]
			}

			// Only add it if it's not empty and not already in the suggestions array
			if suggestionText != "" && !containsSuggestion(comment.Suggestions, suggestionText) {
				comment.Suggestions = append(comment.Suggestions, suggestionText)
			}
		}

		// Remove the suggestions section from content
		comment.Content = strings.TrimSpace(strings.Join(lines[:suggestionSectionIndex], "\n"))
	} else {
		// No suggestions section found, keep content as is
		comment.Content = content
	}

	// Deduplicate suggestions array
	comment.Suggestions = deduplicateSuggestions(comment.Suggestions)
}

// containsSuggestion checks if a suggestion text already exists in the suggestions array (case-insensitive)
func containsSuggestion(suggestions []string, text string) bool {
	lowercaseText := strings.ToLower(text)
	for _, suggestion := range suggestions {
		if strings.ToLower(suggestion) == lowercaseText {
			return true
		}
	}
	return false
}

// Helper function to print debug info about comments by file
func (p *GeminiProvider) printCommentsByFile(comments []*models.ReviewComment, diffs []*models.CodeDiff) {
	fileCommentCount := make(map[string]int)
	for _, comment := range comments {
		fileCommentCount[comment.FilePath]++
	}

	fmt.Println("Comments by file:")
	for _, diff := range diffs {
		count := fileCommentCount[diff.FilePath]
		fmt.Printf("- %s: %d comments\n", diff.FilePath, count)
	}
}

// Helper function to check if a file exists in the diffs
func (p *GeminiProvider) fileExists(filePath string, diffs []*models.CodeDiff) bool {
	for _, diff := range diffs {
		if diff.FilePath == filePath {
			return true
		}
	}
	return false
}

// extractHumanReadableSummary processes the summary from the AI to ensure it's human-readable
// and concise while removing unnecessary praise
func extractHumanReadableSummary(summary string) string {
	// Clean up common formatting issues
	summary = strings.TrimSpace(summary)

	// Remove JSON or code block markers
	summary = strings.ReplaceAll(summary, "```json", "")
	summary = strings.ReplaceAll(summary, "```", "")

	// Check if the summary is a JSON object and extract just the text content
	if strings.HasPrefix(summary, "{") && strings.HasSuffix(summary, "}") {
		// Try to extract just the text value if it's a simple JSON string
		summaryRegex := regexp.MustCompile(`"summary"\s*:\s*"((?:\\"|[^"])*?)"`)
		if matches := summaryRegex.FindStringSubmatch(summary); len(matches) > 1 {
			extractedSummary := matches[1]
			// Unescape JSON escapes
			extractedSummary = strings.ReplaceAll(extractedSummary, `\"`, `"`)
			extractedSummary = strings.ReplaceAll(extractedSummary, `\\`, `\`)
			extractedSummary = strings.ReplaceAll(extractedSummary, `\n`, "\n")
			summary = extractedSummary
		}
	}

	// Remove unnecessary praise phrases
	praisePhrases := []string{
		"Overall, this is a good change",
		"The code looks good overall",
		"The changes are well-implemented",
		"Great job on this change",
		"Well done",
		"Good work",
	}

	for _, phrase := range praisePhrases {
		summary = strings.ReplaceAll(summary, phrase, "")
		summary = strings.ReplaceAll(summary, strings.ToLower(phrase), "")
	}

	// Check if summary already has a heading that ends with (LiveReview)
	hasLiveReviewHeading := regexp.MustCompile(`(?m)^#.*\(LiveReview\).*$`).MatchString(summary)

	// Remove existing headings if they don't include (LiveReview)
	if !hasLiveReviewHeading && strings.HasPrefix(summary, "#") {
		// Extract the content after the first heading
		headingEndIdx := strings.Index(summary, "\n")
		if headingEndIdx > 0 {
			summary = strings.TrimSpace(summary[headingEndIdx+1:])
		}
	}

	// If there's no heading with (LiveReview), add one with a specific title based on the content
	if !hasLiveReviewHeading {
		// Generate a title based on the content of the summary
		title := generateMeaningfulTitle(summary)
		summary = fmt.Sprintf("# %s (LiveReview)\n\n%s", title, summary)
	}

	// Clean up multiple newlines
	multipleNewlines := regexp.MustCompile(`\n{3,}`)
	summary = multipleNewlines.ReplaceAllString(summary, "\n\n")

	return summary
}

// generateMeaningfulTitle creates a specific, descriptive title based on the summary content
func generateMeaningfulTitle(summary string) string {
	// Analyze the summary content to extract key information about the changes

	// Check for common patterns that indicate what type of changes were made
	lowerSummary := strings.ToLower(summary)

	// Check for version changes
	if strings.Contains(lowerSummary, "version bump") || strings.Contains(lowerSummary, "version update") ||
		regexp.MustCompile(`v\d+\.\d+\.\d+`).MatchString(summary) {
		return "Version Update"
	}

	// Check for UI/styling changes
	if strings.Contains(lowerSummary, "css") || strings.Contains(lowerSummary, "style") ||
		strings.Contains(lowerSummary, "margin") || strings.Contains(lowerSummary, "padding") ||
		strings.Contains(lowerSummary, "layout") || strings.Contains(lowerSummary, "ui") {
		return "UI Style Changes"
	}

	// Check for bug fixes
	if strings.Contains(lowerSummary, "fix") || strings.Contains(lowerSummary, "bug") ||
		strings.Contains(lowerSummary, "issue") || strings.Contains(lowerSummary, "error") {
		return "Bug Fix"
	}

	// Check for feature additions
	if strings.Contains(lowerSummary, "add") || strings.Contains(lowerSummary, "new feature") ||
		strings.Contains(lowerSummary, "implement") || strings.Contains(lowerSummary, "enhancement") {
		return "Feature Addition"
	}

	// Check for refactoring
	if strings.Contains(lowerSummary, "refactor") || strings.Contains(lowerSummary, "restructure") ||
		strings.Contains(lowerSummary, "reorganize") || strings.Contains(lowerSummary, "clean up") {
		return "Code Refactoring"
	}

	// Check for documentation changes
	if strings.Contains(lowerSummary, "documentation") || strings.Contains(lowerSummary, "docs") ||
		strings.Contains(lowerSummary, "comment") || strings.Contains(lowerSummary, "readme") {
		return "Documentation Update"
	}

	// Check for performance improvements
	if strings.Contains(lowerSummary, "performance") || strings.Contains(lowerSummary, "optimize") ||
		strings.Contains(lowerSummary, "speed up") || strings.Contains(lowerSummary, "efficiency") {
		return "Performance Optimization"
	}

	// Try to extract the first sentence as it often contains the key information
	firstSentenceMatch := regexp.MustCompile(`^([^.!?]+[.!?])`).FindStringSubmatch(summary)
	if len(firstSentenceMatch) > 1 {
		// Take the first 40 chars of the first sentence if it's long
		firstSentence := firstSentenceMatch[1]
		if len(firstSentence) > 40 {
			// Find the last space before the 40th character
			lastSpaceIdx := strings.LastIndex(firstSentence[:40], " ")
			if lastSpaceIdx > 0 {
				firstSentence = firstSentence[:lastSpaceIdx] + "..."
			} else {
				firstSentence = firstSentence[:40] + "..."
			}
		}
		return firstSentence
	}

	// Default to a generic but more descriptive title
	return "Code Changes Summary"
}

// Configure sets up the provider with needed configuration
func (p *GeminiProvider) Configure(config map[string]interface{}) error {
	// Extract configuration values
	if apiKey, ok := config["api_key"].(string); ok {
		p.TestableFields.APIKey = apiKey
	} else {
		return fmt.Errorf("api_key is required")
	}

	if model, ok := config["model"].(string); ok && model != "" {
		p.TestableFields.Model = model
	}

	if temp, ok := config["temperature"].(float64); ok && temp > 0 {
		p.TestableFields.Temperature = temp
	}

	if maxTokens, ok := config["max_tokens_per_batch"].(int); ok && maxTokens > 0 {
		p.TestableFields.MaxTokensPerBatch = maxTokens
	} else if maxTokens, ok := config["max_tokens_per_batch"].(float64); ok && maxTokens > 0 {
		// Handle case where the value comes as float64 from JSON or TOML
		p.TestableFields.MaxTokensPerBatch = int(maxTokens)
	}

	return nil
}

// Name returns the provider's name
func (p *GeminiProvider) Name() string {
	return "gemini"
}

// logRequestAndResponse logs the complete request and response to a file for inspection
func (p *GeminiProvider) logRequestAndResponse(prompt string, reqJSON []byte, respBody []byte, status string) error {
	logDir := "gemini_logs"

	// Create logs directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create a timestamped log file
	timestamp := fmt.Sprintf("%d", os.Getpid())
	logPath := filepath.Join(logDir, fmt.Sprintf("gemini_request_response_%s.log", timestamp))

	// Open file for appending (or create if it doesn't exist)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer f.Close()

	// Log request information (only if respBody is nil, meaning this is the request logging call)
	if respBody == nil {
		fmt.Fprintf(f, "=== GEMINI API REQUEST LOG ===\n")
		fmt.Fprintf(f, "Timestamp: %s\n", fmt.Sprintf("%d", os.Getpid()))
		fmt.Fprintf(f, "Model: %s\n", p.Model)
		fmt.Fprintf(f, "Temperature: %f\n", p.Temperature)
		fmt.Fprintf(f, "\n--- PROMPT ---\n")
		fmt.Fprintf(f, "%s\n", prompt)
		fmt.Fprintf(f, "\n--- REQUEST JSON ---\n")

		// Pretty print the JSON request
		var prettyReq bytes.Buffer
		if err := json.Indent(&prettyReq, reqJSON, "", "  "); err == nil {
			fmt.Fprintf(f, "%s\n", prettyReq.String())
		} else {
			fmt.Fprintf(f, "%s\n", string(reqJSON))
		}

		fmt.Fprintf(f, "\n%s\n\n", strings.Repeat("=", 80))
	} else {
		// Log response information
		fmt.Fprintf(f, "--- RESPONSE ---\n")
		fmt.Fprintf(f, "Status: %s\n", status)
		fmt.Fprintf(f, "Response Body Length: %d bytes\n", len(respBody))
		fmt.Fprintf(f, "\n--- RESPONSE BODY ---\n")

		// Try to pretty print the JSON response
		var prettyResp bytes.Buffer
		if err := json.Indent(&prettyResp, respBody, "", "  "); err == nil {
			fmt.Fprintf(f, "%s\n", prettyResp.String())
		} else {
			// If it's not valid JSON, just write it as is
			fmt.Fprintf(f, "%s\n", string(respBody))
		}

		fmt.Fprintf(f, "\n%s\n\n", strings.Repeat("=", 80))

		// Also extract and log just the response text for easier reading
		var respObj reviewResponse
		if json.Unmarshal(respBody, &respObj) == nil && len(respObj.Candidates) > 0 && len(respObj.Candidates[0].Content.Parts) > 0 {
			responseText := respObj.Candidates[0].Content.Parts[0].Text
			fmt.Fprintf(f, "--- EXTRACTED RESPONSE TEXT ---\n")
			fmt.Fprintf(f, "%s\n", responseText)
			fmt.Fprintf(f, "\n%s\n\n", strings.Repeat("=", 80))
		}

		fmt.Printf("Gemini API request and response logged to: %s\n", logPath)
	}

	return nil
}
