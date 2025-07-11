package gemini

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/livereview/pkg/models"
)

// Attempt to parse a JSON response from Gemini
func (p *GeminiProvider) parseJSONResponse(response string, diffs []*models.CodeDiff) (*models.ReviewResult, error) {
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

	// Try to extract JSON - first check if the whole response is JSON
	trimmedResponse := strings.TrimSpace(response)
	isJSON := strings.HasPrefix(trimmedResponse, "{") && strings.HasSuffix(trimmedResponse, "}")

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
			// No JSON found
			return nil, fmt.Errorf("no valid JSON found in response")
		}
	}

	// Try to parse the JSON
	var jsonResponse GeminiResponse
	if err := json.Unmarshal([]byte(jsonStr), &jsonResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	// Validate the parsed JSON
	if jsonResponse.Summary == "" || len(jsonResponse.Comments) == 0 {
		return nil, fmt.Errorf("parsed JSON is missing required fields")
	}

	// Create the result
	result := &models.ReviewResult{
		Summary:  jsonResponse.Summary,
		Comments: []*models.ReviewComment{},
	}

	// Convert comments
	for _, comment := range jsonResponse.Comments {
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

		fmt.Printf("DEBUG: Processing comment - original FilePath=%s, Line=%d\n", comment.FilePath, comment.LineNumber)

		// Try to match with a real file if path doesn't match exactly
		found := false
		for _, diff := range diffs {
			if diff.FilePath == reviewComment.FilePath {
				found = true
				fmt.Printf("DEBUG: Exact file path match found: %s\n", diff.FilePath)
				break
			}
		}

		if !found {
			fmt.Printf("DEBUG: No exact match found, trying partial matches\n")
			for _, diff := range diffs {
				if strings.Contains(diff.FilePath, reviewComment.FilePath) ||
					strings.Contains(reviewComment.FilePath, diff.FilePath) {
					fmt.Printf("DEBUG: Partial match - changing FilePath from '%s' to '%s'\n",
						reviewComment.FilePath, diff.FilePath)
					reviewComment.FilePath = diff.FilePath
					break
				}
			}
		}

		result.Comments = append(result.Comments, reviewComment)
	}

	fmt.Printf("Parsed %d comments from JSON response\n", len(result.Comments))
	return result, nil
}

// Parse a text (non-JSON) response from Gemini
func (p *GeminiProvider) parseTextResponse(response string, diffs []*models.CodeDiff) (*models.ReviewResult, error) {
	// Extract summary and comments from the text response
	sections := strings.Split(response, "## Specific Comments")

	// Initialize the result
	result := &models.ReviewResult{
		Summary:  strings.TrimSpace(sections[0]),
		Comments: []*models.ReviewComment{},
	}

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

	// If no specific comments were found, add a generic comment for each file
	if len(result.Comments) == 0 && len(diffs) > 0 {
		// Add a comment for each non-deleted file
		for _, diff := range diffs {
			if !diff.IsDeleted {
				result.Comments = append(result.Comments, &models.ReviewComment{
					FilePath: diff.FilePath,
					Line:     1,
					Content:  "No specific issues found in this file.",
					Severity: models.SeverityInfo,
					Category: "general",
				})
			}
		}
	} else {
		// Ensure we have at least one comment for each file that was changed
		reviewedFiles := make(map[string]bool)

		// Mark files that already have comments
		for _, comment := range result.Comments {
			reviewedFiles[comment.FilePath] = true
		}

		// Add generic comments for files without specific comments
		for _, diff := range diffs {
			if !diff.IsDeleted && !reviewedFiles[diff.FilePath] {
				result.Comments = append(result.Comments, &models.ReviewComment{
					FilePath: diff.FilePath,
					Line:     1,
					Content:  "No specific issues found in this file.",
					Severity: models.SeverityInfo,
					Category: "general",
				})
			}
		}
	}

	return result, nil
}
