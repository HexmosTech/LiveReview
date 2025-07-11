package gemini

import (
	"context"
	"strconv"

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
	// This mimics the prompt construction from ReviewCode but returns the prompt
	// instead of sending it to the API
	if len(diffs) == 0 {
		return "No changes were found in this merge request.", nil
	}

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
	for _, diff := range diffs {
		prompt += "FILE " + diff.FilePath + "\n"

		if diff.IsNew {
			prompt += "[NEW FILE]\n"
		} else if diff.IsDeleted {
			prompt += "[DELETED FILE]\n"
		} else if diff.IsRenamed {
			prompt += "[RENAMED FROM: " + diff.OldFilePath + "]\n"
		}

		// Add hunks with enhanced line number information
		for _, hunk := range diff.Hunks {
			// Add hunk header with line numbers
			prompt += "@@ -L" + strconv.Itoa(hunk.OldStartLine) + "," + strconv.Itoa(hunk.OldLineCount) +
				" +L" + strconv.Itoa(hunk.NewStartLine) + "," + strconv.Itoa(hunk.NewLineCount) + " @@\n"

			// Add the hunk content
			prompt += hunk.Content + "\n"
		}
		prompt += "\n---\n\n"
	}

	return prompt, nil
}
