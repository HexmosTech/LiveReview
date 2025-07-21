package batch

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/livereview/pkg/models"
)

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

	for i, diff := range input {
		// Calculate tokens for this diff
		diffTokens := counter.CountTokens(diff.FilePath)
		for _, hunk := range diff.Hunks {
			diffTokens += counter.CountTokens(hunk.Content)
		}

		p.Logger.Debug("Diff %d (%s): %d tokens", i+1, diff.FilePath, diffTokens)

		// Check if adding this diff would exceed the batch token limit
		if currentBatchTokens+diffTokens > p.MaxBatchTokens && len(currentBatch) > 0 {
			// Current batch is full, start a new one
			p.Logger.Info("Batch %d full: %d files, %d tokens",
				currentBatchNum, len(currentBatch), currentBatchTokens)
			batches = append(batches, currentBatch)
			currentBatch = make([]models.CodeDiff, 0)
			currentBatchTokens = 0
			currentBatchNum++
		}

		// Add diff to current batch
		currentBatch = append(currentBatch, diff)
		currentBatchTokens += diffTokens
		p.Logger.Debug("Added to batch %d: %s (%d tokens, batch total: %d tokens)",
			currentBatchNum, diff.FilePath, diffTokens, currentBatchTokens)
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

// BatchResult represents the result of processing a single batch
type BatchResult struct {
	Summary  string
	Comments []*models.ReviewComment
	Error    error
	BatchID  string
}

// AggregateAndCombineOutputs combines the results of multiple batches
func (p *BatchProcessor) AggregateAndCombineOutputs(results []*BatchResult) (*models.ReviewResult, error) {
	p.Logger.Info("Aggregating outputs from %d batch results", len(results))

	if len(results) == 0 {
		p.Logger.Warn("No batch results to aggregate")
		return &models.ReviewResult{
			Summary:  "No results were produced.",
			Comments: []*models.ReviewComment{},
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

	// Combine summaries and comments
	combinedSummary := "# Multiple Files Review (LiveReview)\n\n"
	var combinedComments []*models.ReviewComment
	totalComments := 0

	for i, result := range results {
		p.Logger.Debug("Processing result %d: %d comments", i+1, len(result.Comments))

		if len(results) > 1 {
			batchIDSuffix := ""
			if result.BatchID != "" {
				batchIDSuffix = " (" + result.BatchID + ")"
			}
			combinedSummary += fmt.Sprintf("## Batch %d%s\n\n", i+1, batchIDSuffix)
		}
		combinedSummary += result.Summary + "\n\n"

		// Add comments from this batch
		combinedComments = append(combinedComments, result.Comments...)
		totalComments += len(result.Comments)
	}

	// Deduplicate comments based on file path and line number
	uniqueComments := deduplicateComments(combinedComments)

	p.Logger.Info("Combined %d total comments, %d unique comments after deduplication",
		totalComments, len(uniqueComments))

	return &models.ReviewResult{
		Summary:  combinedSummary,
		Comments: uniqueComments,
	}, nil
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
