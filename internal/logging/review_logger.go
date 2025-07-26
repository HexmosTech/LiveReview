package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ReviewLogger manages logging for a single trigger-review invocation
type ReviewLogger struct {
	reviewID  string
	logFile   *os.File
	mutex     sync.Mutex
	startTime time.Time
}

var (
	currentLogger *ReviewLogger
	loggerMutex   sync.Mutex
)

// StartReviewLogging initializes logging for a new trigger-review invocation
func StartReviewLogging(reviewID string) (*ReviewLogger, error) {
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
		reviewID:  reviewID,
		logFile:   logFile,
		startTime: time.Now(),
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

// Log writes a message to the review log
func (r *ReviewLogger) Log(format string, args ...interface{}) {
	if r == nil {
		return
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	timestamp := time.Now().Format("15:04:05.000")
	elapsed := time.Since(r.startTime)

	message := fmt.Sprintf("[%s] [+%v] %s\n", timestamp, elapsed.Round(time.Millisecond), fmt.Sprintf(format, args...))
	r.logFile.WriteString(message)
	r.logFile.Sync() // Ensure immediate write

	// Also log to console for immediate feedback
	fmt.Printf("[REVIEW LOG] %s", message)
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
