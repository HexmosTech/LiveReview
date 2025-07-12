package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/livereview/pkg/models"
)

// GeminiProvider implements the AI Provider interface for Google's Gemini
type GeminiProvider struct {
	TestableFields
}

// GeminiConfig contains configuration for the Gemini provider
type GeminiConfig struct {
	APIKey            string  `koanf:"api_key"`
	Model             string  `koanf:"model"`
	Temperature       float64 `koanf:"temperature"`
	MaxTokensPerBatch int     `koanf:"max_tokens_per_batch"`
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

	if config.MaxTokensPerBatch == 0 {
		config.MaxTokensPerBatch = 10000
	}

	return &GeminiProvider{
		TestableFields: TestableFields{
			APIKey:            config.APIKey,
			Model:             config.Model,
			Temperature:       config.Temperature,
			MaxTokensPerBatch: config.MaxTokensPerBatch,
			HTTPClient:        &http.Client{},
		},
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
	prompt += "IMPORTANT: Format your response as a valid JSON object with the following structure:\n"
	prompt += "{\n"
	prompt += "  \"summary\": \"Overall summary of the changes\",\n"
	prompt += "  \"filesChanged\": [\"file1.ext\", \"file2.ext\"],\n"
	prompt += "  \"comments\": [\n"
	prompt += "    {\n"
	prompt += "      \"filePath\": \"path/to/file.ext\",\n"
	prompt += "      \"lineNumber\": 42,\n"
	prompt += "      \"content\": \"Your detailed comment about the code\",\n"
	prompt += "      \"severity\": \"critical|warning|info\",\n"
	prompt += "      \"suggestions\": [\"Suggestion 1\", \"Suggestion 2\"]\n"
	prompt += "    }\n"
	prompt += "  ]\n"
	prompt += "}\n\n"
	prompt += "CRITICAL RULES (MUST FOLLOW):\n"
	prompt += "1. Ensure the response is STRICTLY VALID JSON that can be parsed - escape quotes in content properly.\n"
	prompt += "2. ALWAYS place comments in specific files at specific lines, NEVER create general comments.\n"
	prompt += "3. For issues that apply to multiple lines, create separate comments for each specific line.\n"
	prompt += "4. Use EXACT file paths from the diffs provided without any modifications.\n"
	prompt += "5. Always use actual integers for lineNumber, corresponding to the 'L' numbers shown in the code.\n"
	prompt += "6. Avoid creating comments that refer to multiple files or multiple line numbers at once.\n"
	prompt += "7. If you need to reference other lines in your comment, do so in the content text but still attach the comment to a specific line.\n\n"
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

		// Add hunks with enhanced line number information
		for _, hunk := range diff.Hunks {
			// Add hunk header with line numbers
			prompt += fmt.Sprintf("@@ -L%d,%d +L%d,%d @@\n",
				hunk.OldStartLine, hunk.OldLineCount,
				hunk.NewStartLine, hunk.NewLineCount)

			// Add the hunk content with explicit line numbers for easier reference
			lines := strings.Split(hunk.Content, "\n")
			currentOldLine := hunk.OldStartLine
			currentNewLine := hunk.NewStartLine

			for _, line := range lines {
				if strings.HasPrefix(line, "+") {
					// Added line - only exists in new version
					prompt += fmt.Sprintf("L%-5d %s\n", currentNewLine, line)
					currentNewLine++
				} else if strings.HasPrefix(line, "-") {
					// Removed line - only exists in old version
					prompt += fmt.Sprintf("L%-5d %s\n", currentOldLine, line)
					currentOldLine++
				} else if strings.HasPrefix(line, " ") {
					// Context line - exists in both versions
					prompt += fmt.Sprintf("L%-5d %s\n", currentNewLine, line)
					currentNewLine++
					currentOldLine++
				} else {
					// Other line (e.g., diff metadata)
					prompt += fmt.Sprintf("      %s\n", line)
				}
			}
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

// APIURLFormat is the format string for the Gemini API URL
var APIURLFormat = "https://generativelanguage.googleapis.com/v1/models/%s:generateContent?key=%s"

// TestableFields exposes fields for testing
type TestableFields struct {
	APIKey            string
	Model             string
	Temperature       float64
	MaxTokensPerBatch int
	HTTPClient        *http.Client
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
			jsonCodeBlockRegex := regexp.MustCompile("```(?:json)?\n(\\{[\\s\\S]*?\\})\n```")
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
				Summary      string          `json:"summary"`
				FilesChanged []string        `json:"filesChanged"`
				Comments     []GeminiComment `json:"comments"`
			}

			// Try to parse the JSON
			var jsonResponse GeminiResponse
			err := json.Unmarshal([]byte(jsonStr), &jsonResponse)

			// If full JSON parsing failed, try to extract structured data from it
			if err != nil {
				fmt.Printf("Full JSON parsing failed: %v\n", err)
				fmt.Println("Attempting to extract structured data from partial JSON...")

				// Try to manually extract the summary
				summaryRegex := regexp.MustCompile(`"summary"\s*:\s*"([^"]+)"`)
				if summaryMatches := summaryRegex.FindStringSubmatch(jsonStr); len(summaryMatches) > 1 {
					jsonResponse.Summary = summaryMatches[1]
					fmt.Printf("Extracted summary: %s\n", jsonResponse.Summary)
				}

				// Try to extract file paths
				filesRegex := regexp.MustCompile(`"filesChanged"\s*:\s*\[\s*"([^"]+)"`)
				if filesMatches := filesRegex.FindAllStringSubmatch(jsonStr, -1); len(filesMatches) > 0 {
					jsonResponse.FilesChanged = make([]string, 0)
					for _, match := range filesMatches {
						if len(match) > 1 {
							jsonResponse.FilesChanged = append(jsonResponse.FilesChanged, match[1])
						}
					}
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
									for _, suggMatch := range suggestionMatches {
										if len(suggMatch) > 1 {
											suggestion := suggMatch[1]
											suggestion = strings.ReplaceAll(suggestion, `\"`, `"`)
											suggestion = strings.ReplaceAll(suggestion, `\\`, `\`)
											suggestion = strings.ReplaceAll(suggestion, `\n`, "\n")
											suggestions = append(suggestions, suggestion)
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
			if jsonResponse.Summary != "" && (len(jsonResponse.Comments) > 0 || err == nil) {
				fmt.Println("Successfully parsed JSON response")

				// Create the result
				result := &models.ReviewResult{
					Summary:  jsonResponse.Summary,
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
						FilePath:    comment.FilePath,
						Line:        comment.LineNumber,
						Content:     comment.Content,
						Severity:    severity,
						Suggestions: comment.Suggestions,
						Category:    "review",
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

					result.Comments = append(result.Comments, reviewComment)
				}

				// Add generic comments for any files without specific comments
				p.ensureCommentsForAllFiles(result, diffs)

				fmt.Printf("Review complete: Generated %d comments from JSON\n", len(result.Comments))

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
	result.Summary = strings.TrimSpace(sections[0])

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
				if currentComment != nil {
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

				// Look for different line number formats more robustly
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

				// Try to match with a real file
				for _, diff := range diffs {
					if strings.Contains(diff.FilePath, filePath) {
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

	// Add generic comments for any files without specific comments
	p.ensureCommentsForAllFiles(result, diffs)

	fmt.Printf("Review complete: Generated %d comments from text\n", len(result.Comments))

	// Debug: Print which files have comments
	p.printCommentsByFile(result.Comments, diffs)

	return result, nil
}

// Helper function to ensure we have at least one comment for each file
func (p *GeminiProvider) ensureCommentsForAllFiles(result *models.ReviewResult, diffs []*models.CodeDiff) {
	// If no specific comments were found, add a generic comment for each file
	if len(result.Comments) == 0 && len(diffs) > 0 {
		fmt.Println("No comments found, adding generic comments for all files")
		// Add a comment for each non-deleted file
		for _, diff := range diffs {
			if !diff.IsDeleted {
				// Default to line 1, but try to use a more meaningful line number if available
				lineNum := 1
				if len(diff.Hunks) > 0 && diff.Hunks[0].NewStartLine > 0 {
					lineNum = diff.Hunks[0].NewStartLine
				}

				result.Comments = append(result.Comments, &models.ReviewComment{
					FilePath: diff.FilePath,
					Line:     lineNum,
					Content:  "No specific issues found in this file.",
					Severity: models.SeverityInfo,
					Category: "general",
				})
				fmt.Printf("Added generic comment for file %s at line %d\n", diff.FilePath, lineNum)
			}
		}
		return
	}

	// Ensure we have at least one comment for each file that was changed
	reviewedFiles := make(map[string]bool)

	// Mark files that already have comments
	for _, comment := range result.Comments {
		reviewedFiles[comment.FilePath] = true
	}

	// Add generic comments for files without specific comments
	for _, diff := range diffs {
		if !diff.IsDeleted && !reviewedFiles[diff.FilePath] {
			// Default to line 1, but try to use a more meaningful line number if available
			lineNum := 1
			if len(diff.Hunks) > 0 && diff.Hunks[0].NewStartLine > 0 {
				lineNum = diff.Hunks[0].NewStartLine
			}

			result.Comments = append(result.Comments, &models.ReviewComment{
				FilePath: diff.FilePath,
				Line:     lineNum,
				Content:  "No specific issues found in this file.",
				Severity: models.SeverityInfo,
				Category: "general",
			})
			fmt.Printf("Added generic comment for file %s at line %d\n", diff.FilePath, lineNum)
		}
	}
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

// Helper function to get min of two ints
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
