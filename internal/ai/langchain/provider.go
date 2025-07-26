package langchain

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"

	"github.com/livereview/internal/batch"
	"github.com/livereview/internal/logging"
	"github.com/livereview/pkg/models"
)

// LangchainProvider implements the AI Provider interface using langchain abstractions
type LangchainProvider struct {
	llm       llms.Model
	apiKey    string
	modelName string
	maxTokens int
}

// minInt returns the minimum of two integers
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// formatHunkWithLineNumbers processes a diff hunk to add line number annotations
// This is CRITICAL for proper comment positioning - copied from gemini provider
func (p *LangchainProvider) formatHunkWithLineNumbers(hunk models.DiffHunk) string {
	logger := logging.GetCurrentLogger()

	// Extract the original hunk content
	content := hunk.Content
	lines := strings.Split(content, "\n")

	// Find the @@ header line to extract line numbers
	var oldStart, oldCount, newStart, newCount int
	headerPattern := `@@ -(\d+),(\d+) \+(\d+),(\d+) @@`
	re := regexp.MustCompile(headerPattern)

	headerFound := false
	headerLine := ""
	contentStartIndex := 0

	for i, line := range lines {
		if matches := re.FindStringSubmatch(line); matches != nil {
			var err error
			oldStart, err = strconv.Atoi(matches[1])
			if err != nil {
				if logger != nil {
					logger.LogError("Failed to parse old start: %v", err)
				}
				oldStart = hunk.OldStartLine
			}

			oldCount, err = strconv.Atoi(matches[2])
			if err != nil {
				if logger != nil {
					logger.LogError("Failed to parse old count: %v", err)
				}
				oldCount = hunk.OldLineCount
			}

			newStart, err = strconv.Atoi(matches[3])
			if err != nil {
				if logger != nil {
					logger.LogError("Failed to parse new start: %v", err)
				}
				newStart = hunk.NewStartLine
			}

			newCount, err = strconv.Atoi(matches[4])
			if err != nil {
				if logger != nil {
					logger.LogError("Failed to parse new count: %v", err)
				}
				newCount = hunk.NewLineCount
			}

			headerFound = true
			headerLine = line
			contentStartIndex = i + 1
			break
		}
	}

	// Fallback to hunk metadata if header parsing fails
	if !headerFound {
		oldStart = hunk.OldStartLine
		oldCount = hunk.OldLineCount
		newStart = hunk.NewStartLine
		newCount = hunk.NewLineCount

		if logger != nil {
			logger.Log("No @@ header found, using hunk metadata: old=%d+%d, new=%d+%d",
				oldStart, oldCount, newStart, newCount)
		}
	}

	return p.formatSingleHunk(lines[contentStartIndex:], oldStart, newStart, headerLine)
}

// formatSingleHunk formats a single hunk with line numbers
// Returns content formatted as "OLD | NEW | CONTENT" table
func (p *LangchainProvider) formatSingleHunk(lines []string, oldStart, newStart int, header string) string {
	logger := logging.GetCurrentLogger()

	var result strings.Builder

	// Add header if provided
	if header != "" {
		result.WriteString(header + "\n")
	}

	// Add table header
	result.WriteString("OLD | NEW | CONTENT\n")
	result.WriteString("----|-----|--------\n")

	currentOldLine := oldStart
	currentNewLine := newStart

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		prefix := line[0:1]
		content := ""
		if len(line) > 1 {
			content = line[1:]
		}

		var oldNum, newNum string

		switch prefix {
		case "+":
			// Added line - only appears in new version
			oldNum = "   "
			newNum = fmt.Sprintf("%3d", currentNewLine)
			currentNewLine++

		case "-":
			// Removed line - only appears in old version
			oldNum = fmt.Sprintf("%3d", currentOldLine)
			newNum = "   "
			currentOldLine++

		case " ":
			// Context line - appears in both versions
			oldNum = fmt.Sprintf("%3d", currentOldLine)
			newNum = fmt.Sprintf("%3d", currentNewLine)
			currentOldLine++
			currentNewLine++

		default:
			// Unknown prefix - treat as context
			if logger != nil {
				logger.Log("Unknown line prefix '%s' in hunk, treating as context", prefix)
			}
			oldNum = fmt.Sprintf("%3d", currentOldLine)
			newNum = fmt.Sprintf("%3d", currentNewLine)
			currentOldLine++
			currentNewLine++
		}

		// Format the line with proper table structure
		result.WriteString(fmt.Sprintf("%s | %s | %s%s\n", oldNum, newNum, prefix, content))
	}

	if logger != nil {
		logger.Log("Formatted hunk: old lines %d-%d, new lines %d-%d",
			oldStart, currentOldLine-1, newStart, currentNewLine-1)
	}

	return result.String()
}

// Config for the langchain provider
type Config struct {
	APIKey    string `json:"api_key"`
	ModelName string `json:"model_name"`
	MaxTokens int    `json:"max_tokens"`
}

// New creates a new langchain-based AI provider
func New(config Config) *LangchainProvider {
	return &LangchainProvider{
		apiKey:    config.APIKey,
		modelName: config.ModelName,
		maxTokens: config.MaxTokens,
	}
}

func (p *LangchainProvider) Name() string {
	return "langchain"
}

func (p *LangchainProvider) MaxTokensPerBatch() int {
	if p.maxTokens <= 0 {
		return 30000 // Default safe limit
	}
	return p.maxTokens
}

func (p *LangchainProvider) Configure(config map[string]interface{}) error {
	if apiKey, ok := config["api_key"].(string); ok {
		p.apiKey = apiKey
	}
	if modelName, ok := config["model_name"].(string); ok {
		p.modelName = modelName
	}
	if maxTokens, ok := config["max_tokens"].(float64); ok { // JSON numbers are float64
		p.maxTokens = int(maxTokens)
	}

	// Initialize the LLM
	return p.initializeLLM()
}

func (p *LangchainProvider) initializeLLM() error {
	if p.apiKey == "" {
		return fmt.Errorf("API key is required")
	}

	// Configure options for the LLM
	opts := []googleai.Option{
		googleai.WithAPIKey(p.apiKey),
		googleai.WithDefaultModel(p.getModelName()),
	}

	// Set max tokens if configured
	maxTokens := p.maxTokens
	if maxTokens <= 0 {
		maxTokens = 8192 // Default max output tokens for Gemini
	}
	opts = append(opts, googleai.WithDefaultMaxTokens(maxTokens))

	fmt.Printf("[LANGCHAIN INIT] Initializing LLM with model: %s, max tokens: %d\n", p.getModelName(), maxTokens)

	// For now, default to Google AI (Gemini) via langchain
	// In the future, this could be configurable to support other providers
	llm, err := googleai.New(context.Background(), opts...)
	if err != nil {
		return fmt.Errorf("failed to initialize LLM: %w", err)
	}

	p.llm = llm
	return nil
}

func (p *LangchainProvider) getModelName() string {
	if p.modelName != "" {
		return p.modelName
	}
	return "gemini-1.5-flash" // Default model
}

// ReviewCode is the legacy method for backwards compatibility
func (p *LangchainProvider) ReviewCode(ctx context.Context, diffs []*models.CodeDiff) (*models.ReviewResult, error) {
	// Convert to the newer batch-based approach
	batchProcessor := batch.DefaultBatchProcessor()
	return p.ReviewCodeWithBatching(ctx, diffs, batchProcessor)
}

// ReviewCodeBatch processes a single batch of code diffs
func (p *LangchainProvider) ReviewCodeBatch(ctx context.Context, diffs []models.CodeDiff) (*batch.BatchResult, error) {
	if p.llm == nil {
		return nil, fmt.Errorf("LLM not initialized")
	}

	// Generate unique timestamp for this batch
	timestamp := time.Now().Format("20060102_150405_000")
	batchId := fmt.Sprintf("batch_%s", timestamp)

	// Get global logger
	logger := logging.GetCurrentLogger()

	// CRITICAL: Add line numbers to hunks BEFORE creating prompt
	// This is essential for proper comment positioning
	if logger != nil {
		logger.LogSection(fmt.Sprintf("LINE NUMBERING - Batch %s", batchId))
		logger.Log("Adding line numbers to %d diffs before prompt creation", len(diffs))
	}

	// Process each diff to add line numbers (FIRST STEP - before batching/splitting)
	for i, diff := range diffs {
		if logger != nil {
			logger.Log("Processing diff %d: %s (%d hunks)", i+1, diff.FilePath, len(diff.Hunks))
		}

		var originalHunks []string
		var formattedHunks []string

		for j, hunk := range diff.Hunks {
			// Save original content for logging
			originalHunks = append(originalHunks, hunk.Content)

			// Format the hunk with line numbers (RESTORE CRITICAL FUNCTIONALITY)
			formattedContent := p.formatHunkWithLineNumbers(hunk)
			formattedHunks = append(formattedHunks, formattedContent)

			// Update the hunk content with the formatted version
			diff.Hunks[j].Content = formattedContent

			if logger != nil {
				logger.Log("  Hunk %d: Added line numbers (old: %d-%d, new: %d-%d)",
					j+1, hunk.OldStartLine, hunk.OldStartLine+hunk.OldLineCount-1,
					hunk.NewStartLine, hunk.NewStartLine+hunk.NewLineCount-1)
			}
		}

		// Log the transformation for debugging
		if logger != nil && len(originalHunks) > 0 {
			logger.Log("Line numbering transformation for %s:", diff.FilePath)
			logger.Log("--- ORIGINAL HUNK ---")
			logger.LogDiff(diff.FilePath, originalHunks[0][:minInt(200, len(originalHunks[0]))]+"...")
			logger.Log("--- FORMATTED HUNK ---")
			logger.LogDiff(diff.FilePath, formattedHunks[0][:minInt(200, len(formattedHunks[0]))]+"...")
		}
	}

	// Now process the batch with already-formatted diffs
	return p.reviewCodeBatchFormatted(ctx, diffs, batchId)
}

// reviewCodeBatchFormatted processes diffs that already have line numbers formatted
// This is used for recursive batch processing to avoid double line numbering
func (p *LangchainProvider) reviewCodeBatchFormatted(ctx context.Context, diffs []models.CodeDiff, batchId string) (*batch.BatchResult, error) {
	logger := logging.GetCurrentLogger()

	// Generate the review prompt (diffs already have line numbers)
	prompt := p.createReviewPrompt(diffs)

	// Log request to global logger
	if logger != nil {
		logger.LogRequest(batchId, p.modelName, prompt)
		logger.Log("Processing batch %s with %d diffs", batchId, len(diffs))
	}

	// Call the LLM
	fmt.Printf("[LANGCHAIN REQUEST] Calling LLM for batch %s...\n", batchId)
	response, err := llms.GenerateFromSinglePrompt(ctx, p.llm, prompt)
	if err != nil {
		if logger != nil {
			logger.LogError(fmt.Sprintf("LLM call batch %s", batchId), err)
		}
		fmt.Printf("[LANGCHAIN ERROR] LLM call failed for batch %s: %v\n", batchId, err)
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Log response to global logger
	if logger != nil {
		logger.LogResponse(batchId, response)
	}

	// Parse the response
	fmt.Printf("[LANGCHAIN PARSE] Starting to parse response for batch %s...\n", batchId)
	result, err := p.parseResponse(response, diffs)
	if err != nil {
		if logger != nil {
			logger.LogError(fmt.Sprintf("JSON parsing batch %s", batchId), err)
		}
		fmt.Printf("[LANGCHAIN PARSE ERROR] Failed to parse response for batch %s: %v\n", batchId, err)
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Log parsed comments to global logger
	if logger != nil {
		logger.LogComments(batchId, result.Comments)
		logger.Log("Batch %s completed successfully with %d comments", batchId, len(result.Comments))
	}

	fmt.Printf("[LANGCHAIN SUCCESS] Batch %s completed with %d comments\n", batchId, len(result.Comments))

	// Convert to BatchResult
	return &batch.BatchResult{
		Summary:     result.FileSummary, // Use FileSummary for batch-level summary
		FileSummary: result.FileSummary,
		Comments:    result.Comments,
		Error:       nil,
		BatchID:     batchId,
	}, nil
}

// ReviewCodeWithBatching processes code diffs using the batch processor
func (p *LangchainProvider) ReviewCodeWithBatching(ctx context.Context, diffs []*models.CodeDiff, batchProcessor *batch.BatchProcessor) (*models.ReviewResult, error) {
	if p.llm == nil {
		if err := p.initializeLLM(); err != nil {
			return nil, fmt.Errorf("failed to initialize LLM: %w", err)
		}
	}

	// Convert []*models.CodeDiff to []models.CodeDiff
	input := batchProcessor.PrepareFullInput(diffs)

	// CRITICAL: Add line numbers to ALL diffs BEFORE batching
	// This ensures line numbering happens only once, not per batch
	logger := logging.GetCurrentLogger()
	if logger != nil {
		logger.LogSection("LINE NUMBERING - Pre-Batch Processing")
		logger.Log("Adding line numbers to %d diffs before batching", len(input))
	}

	// Process each diff to add line numbers (BEFORE batching/splitting)
	for i, diff := range input {
		if logger != nil {
			logger.Log("Processing diff %d: %s (%d hunks)", i+1, diff.FilePath, len(diff.Hunks))
		}

		for j, hunk := range diff.Hunks {
			// Format the hunk with line numbers
			formattedContent := p.formatHunkWithLineNumbers(hunk)
			// Update the hunk content with the formatted version
			diff.Hunks[j].Content = formattedContent

			if logger != nil {
				logger.Log("  Hunk %d: Added line numbers (old: %d-%d, new: %d-%d)",
					j+1, hunk.OldStartLine, hunk.OldStartLine+hunk.OldLineCount-1,
					hunk.NewStartLine, hunk.NewStartLine+hunk.NewLineCount-1)
			}
		}
	}

	// Create batch input (AFTER line numbering)
	batchInput := batchProcessor.BatchInputs(input)

	// Process batches using task queue (following Gemini provider pattern)
	taskQueue := batch.NewTaskQueue(4) // Use 4 workers by default

	// Use the batch processor's configuration for the task queue if available
	if batchProcessor.TaskQueueConfig.MaxWorkers > 0 {
		taskQueue = batch.ConfigureTaskQueue(batchProcessor.TaskQueueConfig)
	}

	// Create tasks for each batch
	for i, batchDiffs := range batchInput.Batches {
		batchID := fmt.Sprintf("batch-%d", i+1)

		// Create a processor function for this batch
		// Use the formatted method to avoid double line numbering
		processor := func(ctx context.Context, batchDiffs []models.CodeDiff) (*batch.BatchResult, error) {
			return p.reviewCodeBatchFormatted(ctx, batchDiffs, batchID)
		}

		// Create and add the task
		task := batch.NewBatchTask(batchID, batchDiffs, processor)
		task.SetBatchNumber(i + 1)
		task.SetLogger(batchProcessor.Logger)
		taskQueue.AddTask(task)
	}

	// Execute all tasks
	taskResults := taskQueue.ProcessAll(ctx)

	// Collect results
	batchResults := make([]*batch.BatchResult, len(batchInput.Batches))
	for i := range batchInput.Batches {
		batchID := fmt.Sprintf("batch-%d", i+1)
		taskResult, ok := taskResults[batchID]

		if !ok || taskResult.Error != nil {
			if !ok {
				return nil, fmt.Errorf("batch %s not found in results", batchID)
			}
			return nil, fmt.Errorf("error processing batch %s: %v", batchID, taskResult.Error)
		}

		batchResult, ok := taskResult.Result.(*batch.BatchResult)
		if !ok {
			return nil, fmt.Errorf("invalid result type for batch %s", batchID)
		}

		batchResults[i] = batchResult
	}

	// Aggregate results using the batch processor's aggregation logic
	return batchProcessor.AggregateAndCombineOutputs(ctx, p.llm, batchResults)
}

// createReviewPrompt generates the prompt for code review
func (p *LangchainProvider) createReviewPrompt(diffs []models.CodeDiff) string {
	var prompt strings.Builder

	prompt.WriteString("You are an expert code reviewer. Analyze the following code changes and provide feedback.\n\n")
	prompt.WriteString("For each file, provide:\n")
	prompt.WriteString("- A brief file-level summary if the changes are complex (omit for simple changes)\n")
	prompt.WriteString("- Specific line comments with:\n")
	prompt.WriteString("  * Line number\n")
	prompt.WriteString("  * Issue description\n")
	prompt.WriteString("  * Severity (info, warning, critical)\n")
	prompt.WriteString("  * Clear suggestions for improvement\n")
	prompt.WriteString("  * Whether the comment is for internal analysis only or should be posted to the user\n\n")

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
	prompt.WriteString("      \"suggestions\": [\"Specific improvement suggestion 1\", \"Specific improvement suggestion 2\"],\n")
	prompt.WriteString("      \"isInternal\": false\n")
	prompt.WriteString("    }\n")
	prompt.WriteString("  ]\n")
	prompt.WriteString("}\n")
	prompt.WriteString("```\n\n")

	prompt.WriteString("COMMENT CLASSIFICATION:\n")
	prompt.WriteString("- Set \"isInternal\": true for comments that are:\n")
	prompt.WriteString("  * Obvious/trivial observations (\"variable renamed\", \"method added\")\n")
	prompt.WriteString("  * Purely informational with no actionable insight\n")
	prompt.WriteString("  * Low-value praise (\"good practice\", \"nice naming\")\n")
	prompt.WriteString("  * Detailed technical analysis better suited for synthesis\n")
	prompt.WriteString("- Set \"isInternal\": false for comments that are:\n")
	prompt.WriteString("  * Security vulnerabilities or bugs\n")
	prompt.WriteString("  * Performance issues\n")
	prompt.WriteString("  * Maintainability concerns with clear suggestions\n")
	prompt.WriteString("  * Important architectural decisions that need visibility\n")
	prompt.WriteString("Only post comments that add real value to the developer!\n\n")

	prompt.WriteString("CRITICAL: LINE NUMBER REFERENCES!\n")
	prompt.WriteString("- Each diff hunk is formatted as a table with columns: OLD | NEW | CONTENT\n")
	prompt.WriteString("- The OLD column shows line numbers in the original file\n")
	prompt.WriteString("- The NEW column shows line numbers in the modified file\n")
	prompt.WriteString("- For added lines (+ prefix), use the NEW line number for comments\n")
	prompt.WriteString("- For deleted lines (- prefix), use the OLD line number for comments\n")
	prompt.WriteString("- For modified lines, comment on the NEW version (+ line) with NEW line number\n")
	prompt.WriteString("- You can ONLY comment on lines with + or - prefixes (changed lines)\n")
	prompt.WriteString("- Do NOT comment on context lines (space prefix) or lines outside the diff\n\n")

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

// parseResponse parses the LLM response into a structured result
func (p *LangchainProvider) parseResponse(response string, diffs []models.CodeDiff) (*ParsedResult, error) {
	// Define JSON structures for response
	type Comment struct {
		FilePath    string   `json:"filePath"`
		LineNumber  int      `json:"lineNumber"`
		Content     string   `json:"content"`
		Severity    string   `json:"severity"`
		Suggestions []string `json:"suggestions"`
		IsInternal  bool     `json:"isInternal"`
	}

	type Response struct {
		FileSummary string    `json:"fileSummary"`
		Comments    []Comment `json:"comments"`
	}

	// Try to extract JSON from the response
	jsonStr := p.extractJSONFromResponse(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no valid JSON found in response")
	}

	// Check if the JSON appears to be truncated
	if !strings.HasSuffix(strings.TrimSpace(jsonStr), "}") {
		// Try to repair truncated JSON by attempting to close incomplete structures
		repairedJSON := p.attemptJSONRepair(jsonStr)
		if repairedJSON != jsonStr {
			fmt.Printf("[LANGCHAIN REPAIR] Detected truncated JSON, attempting repair\n")
			fmt.Printf("[LANGCHAIN REPAIR] Original length: %d, Repaired length: %d\n", len(jsonStr), len(repairedJSON))
			jsonStr = repairedJSON
		} else {
			return nil, fmt.Errorf("JSON response appears to be truncated (doesn't end with '}') - possible token limit exceeded. Response length: %d chars", len(response))
		}
	}

	var resp Response
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		// Try to provide more helpful error messages
		if strings.Contains(err.Error(), "unexpected end of JSON input") {
			return nil, fmt.Errorf("JSON response is incomplete/truncated - likely hit token limit. Original response length: %d chars, JSON length: %d chars. Error: %w", len(response), len(jsonStr), err)
		}
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Convert to our models
	var comments []*models.ReviewComment
	for _, comment := range resp.Comments {
		// Validate that this line is actually part of the diff
		if !p.isLineInDiff(comment.FilePath, comment.LineNumber, diffs) {
			fmt.Printf("[LANGCHAIN WARNING] Skipping comment for line %d in %s - line not in diff\n",
				comment.LineNumber, comment.FilePath)
			continue
		}

		// Convert severity
		severity := p.convertSeverity(comment.Severity)

		// Determine if this is a deleted line based on the diff content
		isDeletedLine := p.isDeletedLine(comment.FilePath, comment.LineNumber, diffs)

		reviewComment := &models.ReviewComment{
			FilePath:      comment.FilePath,
			Line:          comment.LineNumber,
			Content:       comment.Content,
			Severity:      severity,
			Suggestions:   comment.Suggestions,
			Category:      "review",
			IsInternal:    comment.IsInternal,
			IsDeletedLine: isDeletedLine,
		}

		comments = append(comments, reviewComment)
	}

	return &ParsedResult{
		FileSummary: resp.FileSummary,
		Comments:    comments,
	}, nil
}

type ParsedResult struct {
	FileSummary string
	Comments    []*models.ReviewComment
}

// extractJSONFromResponse extracts JSON from the LLM response
func (p *LangchainProvider) extractJSONFromResponse(response string) string {
	// Try to extract JSON from markdown code blocks
	start := strings.Index(response, "```json")
	if start == -1 {
		start = strings.Index(response, "```")
	}
	if start == -1 {
		// Check if the whole response is JSON
		trimmed := strings.TrimSpace(response)
		if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
			return trimmed
		}
		return ""
	}

	start = strings.Index(response[start:], "{")
	if start == -1 {
		return ""
	}
	start += strings.Index(response, "```")

	end := strings.LastIndex(response, "}")
	if end == -1 || end <= start {
		return ""
	}

	return response[start : end+1]
}

// convertSeverity converts string severity to models.CommentSeverity
func (p *LangchainProvider) convertSeverity(severity string) models.CommentSeverity {
	switch strings.ToLower(severity) {
	case "critical":
		return models.SeverityCritical
	case "warning":
		return models.SeverityWarning
	case "info":
		return models.SeverityInfo
	default:
		return models.SeverityInfo
	}
}

// isDeletedLine determines if a line comment is on a deleted line
func (p *LangchainProvider) isDeletedLine(filePath string, lineNumber int, diffs []models.CodeDiff) bool {
	for _, diff := range diffs {
		if diff.FilePath == filePath {
			for _, hunk := range diff.Hunks {
				if p.lineInHunk(lineNumber, hunk) {
					return p.lineIsDeleted(lineNumber, hunk)
				}
			}
		}
	}
	return false // Default to false if we can't determine
}

// isLineInDiff checks if a line number is actually part of any diff hunk
func (p *LangchainProvider) isLineInDiff(filePath string, lineNumber int, diffs []models.CodeDiff) bool {
	for _, diff := range diffs {
		if diff.FilePath == filePath {
			for _, hunk := range diff.Hunks {
				if p.lineInHunk(lineNumber, hunk) {
					return true
				}
			}
		}
	}
	return false
}

// lineInHunk checks if a line number is within the range of the given hunk
func (p *LangchainProvider) lineInHunk(lineNumber int, hunk models.DiffHunk) bool {
	return lineNumber >= hunk.OldStartLine && lineNumber <= hunk.OldStartLine+hunk.OldLineCount ||
		lineNumber >= hunk.NewStartLine && lineNumber <= hunk.NewStartLine+hunk.NewLineCount
}

// lineIsDeleted analyzes hunk content to determine if a line is deleted
func (p *LangchainProvider) lineIsDeleted(lineNumber int, hunk models.DiffHunk) bool {
	lines := strings.Split(hunk.Content, "\n")
	oldLine := hunk.OldStartLine
	newLine := hunk.NewStartLine

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			continue // Skip hunk header
		}

		if strings.HasPrefix(line, "-") {
			if oldLine == lineNumber {
				return true
			}
			oldLine++
		} else if strings.HasPrefix(line, "+") {
			newLine++
		} else {
			// Context line
			if oldLine == lineNumber || newLine == lineNumber {
				return false // Context lines are not deleted
			}
			oldLine++
			newLine++
		}
	}

	return false
}

// Helper functions for logging
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func getLastChars(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[len(s)-maxLen:]
}

// writeLogFile writes content to a log file, creating directories if needed
func (p *LangchainProvider) writeLogFile(filename, content string) error {
	// Ensure the directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Write the file
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write log file: %w", err)
	}

	return nil
}

// attemptJSONRepair tries to repair truncated JSON by closing incomplete structures
func (p *LangchainProvider) attemptJSONRepair(jsonStr string) string {
	// Remove any trailing incomplete content after the last complete comment
	lines := strings.Split(jsonStr, "\n")
	var repairedLines []string

	bracketCount := 0
	inString := false
	escapeNext := false
	lastCompleteIndex := -1

	for i, line := range lines {
		// Track brackets and strings to find the last complete structure
		for _, char := range line {
			if escapeNext {
				escapeNext = false
				continue
			}

			if char == '\\' {
				escapeNext = true
				continue
			}

			if char == '"' && !escapeNext {
				inString = !inString
				continue
			}

			if !inString {
				if char == '{' || char == '[' {
					bracketCount++
				} else if char == '}' || char == ']' {
					bracketCount--
				}
			}
		}

		repairedLines = append(repairedLines, line)

		// If we have balanced brackets and this line ends with } or ], this might be a good stopping point
		if bracketCount == 0 && (strings.HasSuffix(strings.TrimSpace(line), "}") || strings.HasSuffix(strings.TrimSpace(line), "]")) {
			lastCompleteIndex = i
		}
	}

	// If we found a good stopping point and there's more content after it, truncate there
	if lastCompleteIndex >= 0 && lastCompleteIndex < len(repairedLines)-1 {
		// Check if the content after the stopping point looks incomplete
		remainingContent := strings.Join(repairedLines[lastCompleteIndex+1:], "\n")
		if strings.TrimSpace(remainingContent) != "" {
			fmt.Printf("[LANGCHAIN REPAIR] Truncating at line %d, removing: %s\n", lastCompleteIndex+1, strings.TrimSpace(remainingContent))
			repairedLines = repairedLines[:lastCompleteIndex+1]
		}
	}

	return strings.Join(repairedLines, "\n")
}
