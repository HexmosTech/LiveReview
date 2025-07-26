package batch

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/livereview/pkg/models"
	"github.com/tmc/langchaingo/llms"
)

// Add ParentHunkID to DiffHunk for tracking
// (If not present in models, add here for batching purposes)
type DiffHunkWithParent struct {
	models.DiffHunk
	ParentFilePath string
	ParentHunkIdx  int
}

// Logger defines a simple logging interface for batch processing
type Logger interface {
	Info(format string, args ...interface{})
	Debug(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
}

// DefaultLogger is a simple implementation of the Logger interface
// that prints to stdout
type DefaultLogger struct {
	Verbose bool
}

// Info logs informational messages
func (l *DefaultLogger) Info(format string, args ...interface{}) {
	fmt.Printf("[BATCH INFO] "+format+"\n", args...)
}

// Debug logs debug messages only when verbose is true
func (l *DefaultLogger) Debug(format string, args ...interface{}) {
	if l.Verbose {
		fmt.Printf("[BATCH DEBUG] "+format+"\n", args...)
	}
}

// Warn logs warning messages
func (l *DefaultLogger) Warn(format string, args ...interface{}) {
	fmt.Printf("[BATCH WARNING] "+format+"\n", args...)
}

// Error logs error messages
func (l *DefaultLogger) Error(format string, args ...interface{}) {
	fmt.Printf("[BATCH ERROR] "+format+"\n", args...)
}

// BatchProcessor handles processing of large code reviews in batches
type BatchProcessor struct {
	MaxBatchTokens  int
	TokenCounters   map[string]TokenCounter
	TaskQueueConfig Config // Configuration for task queue
	Logger          Logger
}

// TokenCounter is an interface for counting tokens in different content types
type TokenCounter interface {
	CountTokens(content string) int
}

// SimpleTokenCounter is a basic implementation of TokenCounter
// that estimates tokens based on word count and special characters
type SimpleTokenCounter struct{}

// CountTokens estimates the number of tokens in the given content
// This is a simple heuristic and not as accurate as model-specific tokenizers
func (c *SimpleTokenCounter) CountTokens(content string) int {
	// Split by whitespace
	words := strings.Fields(content)

	// Count special characters as additional tokens
	specialChars := regexp.MustCompile(`[.,!?;:(){}\[\]<>+\-*/=@#$%^&|~]`)
	specialCount := len(specialChars.FindAllString(content, -1))

	// Simple heuristic: words + special characters + some extra for combined tokens
	return len(words) + specialCount
}

// DefaultBatchProcessor creates a new BatchProcessor with default settings
func DefaultBatchProcessor() *BatchProcessor {
	return &BatchProcessor{
		MaxBatchTokens: 10000, // Default max tokens per batch
		TokenCounters: map[string]TokenCounter{
			"default": &SimpleTokenCounter{},
		},
		TaskQueueConfig: DefaultConfig(),
		Logger:          &DefaultLogger{Verbose: false},
	}
}

// BatchInput represents input that has been split into batches
type BatchInput struct {
	Batches     [][]models.CodeDiff
	TotalTokens int
}

// PrepareFullInput organizes the full input for token counting
// and filters out binary files that shouldn't be processed by the LLM
func (p *BatchProcessor) PrepareFullInput(diffs []*models.CodeDiff) []models.CodeDiff {
	p.Logger.Debug("Preparing full input for token counting: %d diffs", len(diffs))
	result := make([]models.CodeDiff, 0, len(diffs))

	for _, diff := range diffs {
		// Skip binary files
		if p.shouldSkipFile(diff) {
			p.Logger.Info("Skipping binary or non-textual file: %s", diff.FilePath)
			continue
		}

		result = append(result, *diff)
	}

	p.Logger.Info("Processed %d diffs, %d included after filtering binary files",
		len(diffs), len(result))
	return result
}

// shouldSkipFile determines if a file should be skipped in the review process
// It checks if the file is binary or otherwise not suitable for text-based review
func (p *BatchProcessor) shouldSkipFile(diff *models.CodeDiff) bool {
	// Check file extension for common binary formats
	ext := strings.ToLower(filepath.Ext(diff.FilePath))

	// Common binary file extensions to skip
	binaryExtensions := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true,
		".ico": true, ".tif": true, ".tiff": true, ".webp": true, ".svg": true,
		".exe": true, ".dll": true, ".so": true, ".dylib": true, ".a": true, ".lib": true,
		".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true, ".7z": true,
		".rar": true, ".jar": true, ".war": true, ".ear": true, ".class": true,
		".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
		".ppt": true, ".pptx": true, ".bin": true, ".dat": true, ".o": true,
		".mp3": true, ".mp4": true, ".avi": true, ".mov": true, ".wmv": true,
		".flv": true, ".webm": true, ".ttf": true, ".woff": true, ".woff2": true,
		".eot": true, ".pyc": true, ".pyd": true, ".pyo": true,
	}

	if binaryExtensions[ext] {
		return true
	}

	// For files without a recognized binary extension, check content
	if len(diff.Hunks) == 0 {
		// If the file is empty or has no hunks, let it pass through
		return false
	}

	// Check the content of the hunks for binary data
	// Combine a sample of content from hunks for binary detection
	sampleContent := ""

	// Take a sample from each hunk up to a maximum size
	for i, hunk := range diff.Hunks {
		if i < 3 { // Only sample first few hunks
			sampleContent += hunk.Content[:min(len(hunk.Content), 256)]
		}
	}

	return IsBinaryFile(sampleContent)
}

// AssessBatchRequirements determines if batching is needed and how many batches
func (p *BatchProcessor) AssessBatchRequirements(input []models.CodeDiff) (bool, int, int) {
	p.Logger.Debug("Assessing batch requirements for %d diffs", len(input))
	totalTokens := 0
	counter := p.TokenCounters["default"]

	for _, diff := range input {
		// Count tokens in file path
		fileTokens := counter.CountTokens(diff.FilePath)
		totalTokens += fileTokens

		// Count tokens in each hunk
		hunkTokens := 0
		for _, hunk := range diff.Hunks {
			hunkTokenCount := counter.CountTokens(hunk.Content)
			hunkTokens += hunkTokenCount
			totalTokens += hunkTokenCount
		}

		p.Logger.Debug("File %s: %d file path tokens, %d content tokens",
			diff.FilePath, fileTokens, hunkTokens)
	}

	// Determine if batching is needed
	needsBatching := totalTokens > p.MaxBatchTokens

	// Calculate number of batches needed
	batchCount := 1
	if needsBatching {
		batchCount = (totalTokens + p.MaxBatchTokens - 1) / p.MaxBatchTokens
	}

	p.Logger.Info("Batch assessment: Total tokens: %d, Max tokens per batch: %d",
		totalTokens, p.MaxBatchTokens)
	p.Logger.Info("Batching required: %v, Number of batches needed: %d", needsBatching, batchCount)

	return needsBatching, batchCount, totalTokens
}

// BatchInputs splits the input into batches based on token count
func (p *BatchProcessor) BatchInputs(input []models.CodeDiff) *BatchInput {
	needsBatching, batchCount, totalTokens := p.AssessBatchRequirements(input)

	if !needsBatching {
		// If batching is not needed, return a single batch
		p.Logger.Info("Batching not needed, using single batch")
		return &BatchInput{
			Batches:     [][]models.CodeDiff{input},
			TotalTokens: totalTokens,
		}
	}

	p.Logger.Info("Creating %d batches for %d total tokens", batchCount, totalTokens)

	// Initialize batches
	batches := make([][]models.CodeDiff, 0, batchCount)
	counter := p.TokenCounters["default"]

	currentBatch := make([]models.CodeDiff, 0)
	currentBatchTokens := 0
	currentBatchNum := 1

	for _, diff := range input {
		diffTokens := counter.CountTokens(diff.FilePath)
		for _, hunk := range diff.Hunks {
			hunkTokens := counter.CountTokens(hunk.Content)
			diffTokens += hunkTokens
			if hunkTokens > p.MaxBatchTokens {
				// Split hunk into sub-hunks
				subHunks := splitHunkByTokens(hunk, p.MaxBatchTokens, counter)
				for subIdx, subHunk := range subHunks {
					// Tag sub-hunk with parent info
					subDiff := models.CodeDiff{
						FilePath:    diff.FilePath,
						OldContent:  diff.OldContent,
						NewContent:  diff.NewContent,
						Hunks:       []models.DiffHunk{subHunk},
						CommitID:    diff.CommitID,
						FileType:    diff.FileType,
						IsDeleted:   diff.IsDeleted,
						IsNew:       diff.IsNew,
						IsRenamed:   diff.IsRenamed,
						OldFilePath: diff.OldFilePath,
					}
					// Attach parent info for merging (using Hunks[0] fields)
					// You may want to extend models.DiffHunk for real use
					subDiff.Hunks[0].Content = fmt.Sprintf("[PARENT:%s:%d]%s", diff.FilePath, subIdx, subHunk.Content)
					// Add sub-hunk diff to batch
					if currentBatchTokens+counter.CountTokens(subHunk.Content) > p.MaxBatchTokens && len(currentBatch) > 0 {
						batches = append(batches, currentBatch)
						currentBatch = make([]models.CodeDiff, 0)
						currentBatchTokens = 0
						currentBatchNum++
					}
					currentBatch = append(currentBatch, subDiff)
					currentBatchTokens += counter.CountTokens(subHunk.Content)
				}
			} else {
				// Add normal hunk as part of diff
				// ...existing code...
			}
		}
		// If all hunks are normal, add the diff as usual
		if diffTokens <= p.MaxBatchTokens {
			if currentBatchTokens+diffTokens > p.MaxBatchTokens && len(currentBatch) > 0 {
				batches = append(batches, currentBatch)
				currentBatch = make([]models.CodeDiff, 0)
				currentBatchTokens = 0
				currentBatchNum++
			}
			currentBatch = append(currentBatch, diff)
			currentBatchTokens += diffTokens
		}
	}
	// Add the last batch if it's not empty
	if len(currentBatch) > 0 {
		p.Logger.Info("Final batch %d: %d files, %d tokens",
			currentBatchNum, len(currentBatch), currentBatchTokens)
		batches = append(batches, currentBatch)
	}

	p.Logger.Info("Created %d batches with %d total tokens", len(batches), totalTokens)
	for i, batch := range batches {
		p.Logger.Debug("Batch %d: %d files", i+1, len(batch))
	}

	return &BatchInput{
		Batches:     batches,
		TotalTokens: totalTokens,
	}
}

// splitHunkByTokens splits a DiffHunk into sub-hunks each under maxTokens
func splitHunkByTokens(hunk models.DiffHunk, maxTokens int, counter TokenCounter) []models.DiffHunk {
	content := hunk.Content
	var subHunks []models.DiffHunk
	start := 0
	for start < len(content) {
		end := start
		lastTokenCount := -1
		for end < len(content) {
			tokenCount := counter.CountTokens(content[start:end])
			if tokenCount >= maxTokens {
				break
			}
			// If tokenCount doesn't increase, advance by a line or chunk
			if tokenCount == lastTokenCount {
				// Try to find next newline
				nextNewline := strings.Index(content[end:], "\n")
				if nextNewline == -1 {
					end = len(content)
				} else {
					end += nextNewline + 1
				}
			} else {
				end++
			}
			lastTokenCount = tokenCount
		}
		if end == start {
			// Prevent infinite loop: forcibly advance
			end = start + min(1000, len(content)-start)
		}
		// Create sub-hunk
		subHunk := models.DiffHunk{
			OldStartLine: hunk.OldStartLine,
			OldLineCount: hunk.OldLineCount,
			NewStartLine: hunk.NewStartLine,
			NewLineCount: hunk.NewLineCount,
			Content:      content[start:end],
		}
		subHunks = append(subHunks, subHunk)
		start = end
	}
	return subHunks
	// ...existing code...
}

// BatchResult represents the result of processing a single batch
type BatchResult struct {
	Summary     string
	FileSummary string // New: file-level summary if present
	Comments    []*models.ReviewComment
	Error       error
	BatchID     string
}

// AggregateAndCombineOutputs combines the results of multiple batches
func (p *BatchProcessor) AggregateAndCombineOutputs(ctx context.Context, llm llms.Model, results []*BatchResult) (*models.ReviewResult, error) {
	p.Logger.Info("Aggregating outputs from %d batch results", len(results))

	if len(results) == 0 {
		p.Logger.Warn("No batch results to aggregate")
		return &models.ReviewResult{
			Summary:          "No results were produced.",
			Comments:         []*models.ReviewComment{},
			InternalComments: []*models.ReviewComment{},
		}, nil
	}

	// Check for errors
	var errors []string
	for i, result := range results {
		if result.Error != nil {
			p.Logger.Error("Error in batch %d: %v", i+1, result.Error)
			errors = append(errors, fmt.Sprintf("Batch %d: %v", i+1, result.Error))
		}
	}

	if len(errors) > 0 {
		return nil, fmt.Errorf("errors in batch processing: %s", strings.Join(errors, "; "))
	}

	// Collect file-level summaries and separate internal/external comments
	var fileSummaries []string
	var externalComments []*models.ReviewComment
	var internalComments []*models.ReviewComment
	totalComments := 0
	for _, result := range results {
		if result.FileSummary != "" {
			fileSummaries = append(fileSummaries, result.FileSummary)
		}

		// Separate internal and external comments
		for _, comment := range result.Comments {
			totalComments++
			if comment.IsInternal {
				internalComments = append(internalComments, comment)
			} else {
				externalComments = append(externalComments, comment)
			}
		}
	}

	// Synthesize general summary from all file summaries and ALL comments (internal + external)
	// Use LLM abstraction for synthesis
	allCommentsForSynthesis := append(append([]*models.ReviewComment{}, internalComments...), externalComments...)
	generalSummary := synthesizeGeneralSummary(ctx, llm, fileSummaries, allCommentsForSynthesis)

	p.Logger.Info("Aggregation complete: %d total comments (%d external, %d internal), %d file summaries",
		totalComments, len(externalComments), len(internalComments), len(fileSummaries))

	// Output: one general summary, only EXTERNAL comments for posting
	return &models.ReviewResult{
		Summary:          generalSummary,
		Comments:         externalComments,
		InternalComments: internalComments,
	}, nil
}

// synthesizeGeneralSummary calls the LLM abstraction to generate a high-level summary from file summaries and comments

func synthesizeGeneralSummary(ctx context.Context, llm llms.Model, fileSummaries []string, comments []*models.ReviewComment) string {
	var prompt strings.Builder
	prompt.WriteString("You are an expert code reviewer. Given the following file-level summaries and line comments, synthesize a single, high-level summary of the overall change using proper markdown formatting.\n\n")
	prompt.WriteString("REQUIREMENTS:\n")
	prompt.WriteString("1. Use markdown formatting with clear structure: # headings, ## subheadings, **bold**, bullet points\n")
	prompt.WriteString("2. Focus on the big picture, impact, and intent - NOT individual file details\n")
	prompt.WriteString("3. Make it scannable and easy to understand quickly\n")
	prompt.WriteString("4. Start with a clear main title using # heading\n")
	prompt.WriteString("5. Use bullet points for key changes and impacts\n")
	prompt.WriteString("6. Keep it concise but informative\n\n")

	prompt.WriteString("File-level summaries:\n")
	for _, fs := range fileSummaries {
		prompt.WriteString("- " + fs + "\n")
	}
	prompt.WriteString("\nLine comments:\n")
	for _, c := range comments {
		prompt.WriteString(fmt.Sprintf("- [%s:%d] %s\n", c.FilePath, c.Line, c.Content))
	}

	prompt.WriteString("\nGenerate a well-formatted markdown summary following this structure:\n")
	prompt.WriteString("# [Clear main title of what changed]\n\n")
	prompt.WriteString("## Overview\n")
	prompt.WriteString("Brief description of the change intent and scope.\n\n")
	prompt.WriteString("## Key Changes\n")
	prompt.WriteString("- **Area 1**: Description\n")
	prompt.WriteString("- **Area 2**: Description\n\n")
	prompt.WriteString("## Impact\n")
	prompt.WriteString("- **Functionality**: How this affects functionality\n")
	prompt.WriteString("- **Risk**: Any notable risks or considerations\n")

	summary, err := llms.GenerateFromSinglePrompt(ctx, llm, prompt.String())
	if err != nil {
		return "Error generating summary: " + err.Error()
	}
	return summary
	// ...existing code...
}

// deduplicateComments removes duplicate comments based on file path and line number
func deduplicateComments(comments []*models.ReviewComment) []*models.ReviewComment {
	uniqueMap := make(map[string]*models.ReviewComment)

	for _, comment := range comments {
		key := fmt.Sprintf("%s:%d", comment.FilePath, comment.Line)

		if existing, found := uniqueMap[key]; found {
			// If we already have a comment for this location, keep the more severe one
			if getSeverityLevel(comment.Severity) > getSeverityLevel(existing.Severity) {
				uniqueMap[key] = comment
			}
		} else {
			uniqueMap[key] = comment
		}
	}

	// Convert map back to slice
	result := make([]*models.ReviewComment, 0, len(uniqueMap))
	for _, comment := range uniqueMap {
		result = append(result, comment)
	}

	return result
}

// getSeverityLevel converts severity to a numeric level for comparison
func getSeverityLevel(severity models.CommentSeverity) int {
	switch severity {
	case models.SeverityCritical:
		return 3
	case models.SeverityWarning:
		return 2
	default: // models.SeverityInfo
		return 1
	}
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SetLogger sets a custom logger for the batch processor
func (p *BatchProcessor) SetLogger(logger Logger) {
	p.Logger = logger
}

// SetVerboseLogging enables or disables verbose logging if using the default logger
func (p *BatchProcessor) SetVerboseLogging(verbose bool) {
	if defaultLogger, ok := p.Logger.(*DefaultLogger); ok {
		defaultLogger.Verbose = verbose
	}
}

// IsBinaryFile checks if a file is likely to be a binary (non-text) file
// This is a simple heuristic based on looking for null bytes and a high
// percentage of non-printable characters in a sample of the content
func IsBinaryFile(content string) bool {
	if len(content) == 0 {
		return false
	}

	// Check for null bytes, which are common in binary files
	if strings.Contains(content, "\x00") {
		return true
	}

	// Limit the sample size to avoid processing very large files completely
	sampleSize := 512
	if len(content) < sampleSize {
		sampleSize = len(content)
	}

	sample := content[:sampleSize]
	nonPrintable := 0

	for _, r := range sample {
		// Check for non-printable characters (control chars excluding common whitespace)
		if (r < 32 && r != 9 && r != 10 && r != 13) || r >= 127 {
			nonPrintable++
		}
	}

	// If more than 30% of characters are non-printable, consider it binary
	return float64(nonPrintable)/float64(sampleSize) > 0.3
}
