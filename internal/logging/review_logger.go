package logging

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// EventSink defines the interface for emitting review events (imported from api package)
type EventSink interface {
	EmitStatusEvent(ctx context.Context, reviewID, orgID int64, status string) error
	EmitLogEvent(ctx context.Context, reviewID, orgID int64, level, message, batchID string) error
	EmitBatchEvent(ctx context.Context, reviewID, orgID int64, batchID, status string, tokenEstimate, fileCount int) error
	EmitArtifactEvent(ctx context.Context, reviewID, orgID int64, kind, url, batchID string, sizeBytes int64, previewHead, previewTail string) error
	EmitCompletionEvent(ctx context.Context, reviewID, orgID int64, resultSummary string, commentCount int, errorSummary string) error
}

// ReviewLogger manages logging for a single trigger-review invocation
type ReviewLogger struct {
	reviewID    string
	reviewIDInt int64 // numeric review ID for events
	orgID       int64 // organization ID for events
	logFile     *os.File
	mutex       sync.Mutex
	startTime   time.Time
	eventSink   EventSink // optional event sink for emitting structured events
}

var (
	currentLogger *ReviewLogger
	loggerMutex   sync.Mutex
)

// StartReviewLogging initializes logging for a new trigger-review invocation
func StartReviewLogging(reviewID string) (*ReviewLogger, error) {
	// Try to parse reviewID as int64, default to 0 if it fails
	reviewIDInt, _ := strconv.ParseInt(reviewID, 10, 64)
	return StartReviewLoggingWithIDs(reviewID, reviewIDInt, 1) // default orgID to 1
}

// StartReviewLoggingWithIDs initializes logging with explicit numeric IDs for event emission
func StartReviewLoggingWithIDs(reviewID string, reviewIDInt, orgID int64) (*ReviewLogger, error) {
	loggerMutex.Lock()
	defer loggerMutex.Unlock()

	// Close previous logger if exists
	if currentLogger != nil {
		currentLogger.Close()
	}

	timestamp := time.Now().Format("20060102_150405")
	logFileName := fmt.Sprintf("review_%s_%s.log", reviewID, timestamp)
	logPath := filepath.Join("review_logs", logFileName)

	// Ensure directory exists
	if err := os.MkdirAll("review_logs", 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create log file
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	logger := &ReviewLogger{
		reviewID:    reviewID,
		reviewIDInt: reviewIDInt,
		orgID:       orgID,
		logFile:     logFile,
		startTime:   time.Now(),
	}

	currentLogger = logger

	// Write header
	logger.writeHeader()

	return logger, nil
}

// GetCurrentLogger returns the current active logger
func GetCurrentLogger() *ReviewLogger {
	loggerMutex.Lock()
	defer loggerMutex.Unlock()
	return currentLogger
}

// SetEventSink sets the event sink for emitting structured events
func (r *ReviewLogger) SetEventSink(sink EventSink) {
	if r == nil {
		return
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.eventSink = sink

	// Emit initial status event
	if sink != nil {
		ctx := context.Background()
		_ = sink.EmitStatusEvent(ctx, r.reviewIDInt, r.orgID, "started")
	}
}

// Log writes a message to the review log
func (r *ReviewLogger) Log(format string, args ...interface{}) {
	if r == nil {
		return
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	timestamp := time.Now().Format("15:04:05.000")
	elapsed := time.Since(r.startTime)
	logMessage := fmt.Sprintf(format, args...)

	message := fmt.Sprintf("[%s] [+%v] %s\n", timestamp, elapsed.Round(time.Millisecond), logMessage)
	r.logFile.WriteString(message)
	r.logFile.Sync() // Ensure immediate write

	// Also log to console for immediate feedback
	fmt.Printf("[REVIEW LOG] %s", message)

	// Emit log event if event sink is available
	if r.eventSink != nil {
		ctx := context.Background()
		level := r.determineLogLevel("", logMessage)
		batchID := r.extractBatchID(logMessage)
		_ = r.eventSink.EmitLogEvent(ctx, r.reviewIDInt, r.orgID, level, logMessage, batchID)
	}
}

// LogSection writes a section header to the log
func (r *ReviewLogger) LogSection(title string) {
	if r == nil {
		return
	}

	separator := repeatString("=", 80)
	r.Log(separator)
	r.Log("= %s", title)
	r.Log(separator)
}

// LogDiff logs diff information
func (r *ReviewLogger) LogDiff(filePath string, diffContent string) {
	if r == nil {
		return
	}

	r.Log("DIFF for %s:", filePath)
	r.Log("--- DIFF START ---")
	r.logFile.WriteString(diffContent + "\n")
	r.Log("--- DIFF END ---")
}

// LogRequest logs an LLM request
func (r *ReviewLogger) LogRequest(batchID, model string, prompt string) {
	if r == nil {
		return
	}

	r.LogSection(fmt.Sprintf("LLM REQUEST - Batch %s", batchID))
	r.Log("Model: %s", model)
	r.Log("Prompt length: %d characters", len(prompt))
	r.Log("--- PROMPT START ---")
	r.logFile.WriteString(prompt + "\n")
	r.Log("--- PROMPT END ---")

	// Emit artifact event for the prompt
	if r.eventSink != nil {
		ctx := context.Background()
		previewHead := truncateString(prompt, 200)
		previewTail := getLastChars(prompt, 200)
		url := fmt.Sprintf("/review_logs/review_%s_batch_%s_prompt.txt", r.reviewID, batchID)
		_ = r.eventSink.EmitArtifactEvent(ctx, r.reviewIDInt, r.orgID, "prompt", url, batchID, int64(len(prompt)), previewHead, previewTail)
	}
}

// LogResponse logs an LLM response
func (r *ReviewLogger) LogResponse(batchID string, response string) {
	if r == nil {
		return
	}

	r.LogSection(fmt.Sprintf("LLM RESPONSE - Batch %s", batchID))
	r.Log("Response length: %d characters", len(response))
	r.Log("--- RESPONSE START ---")
	r.logFile.WriteString(response + "\n")
	r.Log("--- RESPONSE END ---")

	// Emit artifact event for the response
	if r.eventSink != nil {
		ctx := context.Background()
		previewHead := truncateString(response, 200)
		previewTail := getLastChars(response, 200)
		url := fmt.Sprintf("/review_logs/review_%s_batch_%s_response.txt", r.reviewID, batchID)
		_ = r.eventSink.EmitArtifactEvent(ctx, r.reviewIDInt, r.orgID, "response", url, batchID, int64(len(response)), previewHead, previewTail)
	}
}

// LogError logs an error
func (r *ReviewLogger) LogError(context string, err error) {
	if r == nil {
		return
	}

	r.Log("ERROR in %s: %v", context, err)
}

// LogComments logs the parsed comments
func (r *ReviewLogger) LogComments(batchID string, comments interface{}) {
	if r == nil {
		return
	}

	r.LogSection(fmt.Sprintf("PARSED COMMENTS - Batch %s", batchID))
	r.Log("Comments: %+v", comments)

	// Emit completion event for this batch
	if r.eventSink != nil {
		ctx := context.Background()
		commentCount := r.countComments(comments)
		resultSummary := fmt.Sprintf("Batch %s completed with %d comments", batchID, commentCount)
		_ = r.eventSink.EmitCompletionEvent(ctx, r.reviewIDInt, r.orgID, resultSummary, commentCount, "")
	}
}

// Close finalizes the log file
func (r *ReviewLogger) Close() {
	if r == nil {
		return
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.logFile != nil {
		// Write final message directly without using r.Log() to avoid deadlock
		timestamp := time.Now().Format("15:04:05.000")
		elapsed := time.Since(r.startTime)
		finalMessage := fmt.Sprintf("[%s] [+%v] Review logging completed. Total duration: %v\n",
			timestamp, elapsed.Round(time.Millisecond), time.Since(r.startTime))
		r.logFile.WriteString(finalMessage)
		r.logFile.Sync()

		// Emit final status event
		if r.eventSink != nil {
			ctx := context.Background()
			_ = r.eventSink.EmitStatusEvent(ctx, r.reviewIDInt, r.orgID, "completed")
		}

		r.logFile.Close()
		r.logFile = nil

		// Also log to console
		fmt.Printf("[REVIEW LOG] %s", finalMessage)
	}
}

func (r *ReviewLogger) writeHeader() {
	header := fmt.Sprintf(`LIVEREVIEW TRIGGER-REVIEW LOG
Review ID: %s
Start Time: %s
Log Format: [HH:MM:SS.mmm] [+duration] message

`, r.reviewID, r.startTime.Format("2006-01-02 15:04:05"))

	r.logFile.WriteString(header)
	r.logFile.Sync()
}

// Helper function to repeat strings (Go doesn't have built-in string repetition)
func repeatString(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}

// Helper methods for event emission

// determineLogLevel determines appropriate log level from context and message
func (r *ReviewLogger) determineLogLevel(context, message string) string {
	messageLower := fmt.Sprintf("%s %s", context, message)
	messageLower = fmt.Sprintf("%s", messageLower) // convert to lowercase safely

	// Check for error indicators
	if containsAny(messageLower, []string{"error", "failed", "fail", "exception", "panic"}) {
		return "error"
	}

	// Check for warning indicators
	if containsAny(messageLower, []string{"warning", "warn", "timeout", "retry", "fallback"}) {
		return "warn"
	}

	// Check for debug indicators
	if containsAny(messageLower, []string{"debug", "trace", "dump", "raw", "chunk"}) {
		return "debug"
	}

	// Default to info
	return "info"
}

// extractBatchID tries to extract batch ID from log messages
func (r *ReviewLogger) extractBatchID(message string) string {
	// Look for patterns like "batch-1", "Batch 1", etc.
	if pos := findBatchID(message); pos != "" {
		return pos
	}
	return ""
}

// containsAny checks if text contains any of the given keywords
func containsAny(text string, keywords []string) bool {
	textLower := fmt.Sprintf("%s", text) // ensure lowercase
	for _, keyword := range keywords {
		if pos := findSubstring(textLower, keyword); pos >= 0 {
			return true
		}
	}
	return false
}

// findSubstring finds a substring in a string (case insensitive)
func findSubstring(text, sub string) int {
	// Simple case-insensitive search
	textLen := len(text)
	subLen := len(sub)

	if subLen == 0 {
		return 0
	}
	if subLen > textLen {
		return -1
	}

	for i := 0; i <= textLen-subLen; i++ {
		match := true
		for j := 0; j < subLen; j++ {
			c1 := text[i+j]
			c2 := sub[j]
			// Simple lowercase comparison
			if c1 >= 'A' && c1 <= 'Z' {
				c1 = c1 + 32
			}
			if c2 >= 'A' && c2 <= 'Z' {
				c2 = c2 + 32
			}
			if c1 != c2 {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// findBatchID extracts batch ID from text
func findBatchID(text string) string {
	// Look for "batch" followed by number or dash-number
	words := splitWords(text)
	for i, word := range words {
		wordLower := toLower(word)
		if wordLower == "batch" && i+1 < len(words) {
			return words[i+1]
		}
		if startsWithBatch(wordLower) && len(word) > 5 {
			return word[6:] // remove "batch-"
		}
	}
	return ""
}

// Helper functions for string processing
func splitWords(text string) []string {
	var words []string
	var current string

	for _, char := range text {
		if char == ' ' || char == '\t' || char == '\n' {
			if current != "" {
				words = append(words, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}
	if current != "" {
		words = append(words, current)
	}
	return words
}

func toLower(s string) string {
	result := ""
	for _, char := range s {
		if char >= 'A' && char <= 'Z' {
			result += string(char + 32)
		} else {
			result += string(char)
		}
	}
	return result
}

func startsWithBatch(s string) bool {
	return len(s) >= 6 && s[:6] == "batch-"
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// getLastChars gets the last maxLen characters of a string
func getLastChars(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return "..." + s[len(s)-maxLen:]
}

// countComments attempts to count comments from the interface
func (r *ReviewLogger) countComments(comments interface{}) int {
	if comments == nil {
		return 0
	}

	// Try to handle different types of comment structures
	switch v := comments.(type) {
	case []interface{}:
		return len(v)
	case map[string]interface{}:
		if commentsArray, ok := v["comments"].([]interface{}); ok {
			return len(commentsArray)
		}
		return 1 // assume it's a single comment object
	default:
		return 1 // assume it's a single comment
	}
}
