package api

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
)

// ReviewEventSink provides high-level methods for emitting different types of review events
// It implements the EventSink interface and adds convenience methods
type ReviewEventSink interface {
	EventSink // Embed the basic EventSink interface
	EmitStatusEvent(ctx context.Context, reviewID, orgID int64, status string) error
	EmitLogEvent(ctx context.Context, reviewID, orgID int64, level, message, batchID string) error
	EmitBatchEvent(ctx context.Context, reviewID, orgID int64, batchID, status string, tokenEstimate, fileCount int, comments interface{}) error
	EmitArtifactEvent(ctx context.Context, reviewID, orgID int64, kind, url, batchID string, sizeBytes int64, previewHead, previewTail string) error
	EmitCompletionEvent(ctx context.Context, reviewID, orgID int64, resultSummary string, commentCount int, errorSummary string) error
}

// DatabaseEventSink implements ReviewEventSink using our PollingEventService
type DatabaseEventSink struct {
	service *PollingEventService
}

// NewDatabaseEventSink creates a new database event sink
func NewDatabaseEventSink(db *sql.DB) *DatabaseEventSink {
	return &DatabaseEventSink{
		service: NewPollingEventService(db),
	}
}

// EmitEvent implements the basic EventSink interface
func (s *DatabaseEventSink) EmitEvent(ctx context.Context, event *ReviewEvent) error {
	return s.service.EmitEvent(ctx, event)
}

// EmitStatusEvent emits a status change event
func (s *DatabaseEventSink) EmitStatusEvent(ctx context.Context, reviewID, orgID int64, status string) error {
	return s.service.CreateStatusEvent(ctx, reviewID, orgID, status, nil, nil)
}

// EmitLogEvent emits a log message event
func (s *DatabaseEventSink) EmitLogEvent(ctx context.Context, reviewID, orgID int64, level, message, batchID string) error {
	var batchIDPtr *string
	if batchID != "" {
		batchIDPtr = &batchID
	}
	return s.service.CreateLogEvent(ctx, reviewID, orgID, level, message, batchIDPtr)
}

// EmitBatchEvent emits a batch progress event
func (s *DatabaseEventSink) EmitBatchEvent(ctx context.Context, reviewID, orgID int64, batchID, status string, tokenEstimate, fileCount int, comments interface{}) error {
	var tokenPtr, filePtr *int
	if tokenEstimate > 0 {
		tokenPtr = &tokenEstimate
	}
	// For completed batches, fileCount actually contains commentCount (API design quirk)
	// This will be fixed to use a proper commentCount parameter
	if fileCount > 0 {
		filePtr = &fileCount
	}
	return s.service.CreateBatchEvent(ctx, reviewID, orgID, batchID, status, tokenPtr, filePtr, nil, nil, comments)
}

// EmitArtifactEvent emits an artifact reference event
func (s *DatabaseEventSink) EmitArtifactEvent(ctx context.Context, reviewID, orgID int64, kind, url, batchID string, sizeBytes int64, previewHead, previewTail string) error {
	var batchIDPtr *string
	var sizeBytesPtr *int64
	var previewHeadPtr, previewTailPtr *string

	if batchID != "" {
		batchIDPtr = &batchID
	}
	if sizeBytes > 0 {
		sizeBytesPtr = &sizeBytes
	}
	if previewHead != "" {
		previewHeadPtr = &previewHead
	}
	if previewTail != "" {
		previewTailPtr = &previewTail
	}

	return s.service.CreateArtifactEvent(ctx, reviewID, orgID, kind, url, batchIDPtr, sizeBytesPtr, previewHeadPtr, previewTailPtr)
}

// EmitCompletionEvent emits a review completion event
func (s *DatabaseEventSink) EmitCompletionEvent(ctx context.Context, reviewID, orgID int64, resultSummary string, commentCount int, errorSummary string) error {
	var commentCountPtr *int
	var errorSummaryPtr *string

	if commentCount > 0 {
		commentCountPtr = &commentCount
	}
	if errorSummary != "" {
		errorSummaryPtr = &errorSummary
	}

	return s.service.CreateCompletionEvent(ctx, reviewID, orgID, resultSummary, commentCountPtr, errorSummaryPtr)
}

// Helper functions to extract useful information from log messages and contexts

// ExtractBatchIDFromContext tries to extract batch ID from various contexts
func ExtractBatchIDFromContext(context, message string) string {
	// Look for patterns like "batch-1", "Batch 1", etc.
	contexts := []string{context, message}
	for _, text := range contexts {
		text = strings.ToLower(text)

		// Pattern: "batch xyz" or "batch-xyz"
		if strings.Contains(text, "batch") {
			parts := strings.Fields(text)
			for i, part := range parts {
				if strings.Contains(part, "batch") && i+1 < len(parts) {
					return strings.TrimSpace(parts[i+1])
				}
				if strings.HasPrefix(part, "batch-") {
					return strings.TrimPrefix(part, "batch-")
				}
			}
		}
	}
	return ""
}

// ExtractTokenEstimateFromMessage tries to extract token estimates from log messages
func ExtractTokenEstimateFromMessage(message string) int {
	message = strings.ToLower(message)

	// Look for patterns like "1200 tokens", "token count: 500", etc.
	if strings.Contains(message, "token") {
		parts := strings.Fields(message)
		for i, part := range parts {
			// Look for number before "token"
			if strings.Contains(part, "token") && i > 0 {
				if num, err := strconv.Atoi(strings.TrimSpace(parts[i-1])); err == nil {
					return num
				}
			}
			// Look for number after "tokens:"
			if (part == "tokens:" || part == "token:") && i+1 < len(parts) {
				if num, err := strconv.Atoi(strings.TrimSpace(parts[i+1])); err == nil {
					return num
				}
			}
		}
	}

	return 0
}

// DetermineLogLevel determines appropriate log level from context and message
func DetermineLogLevel(context, message string) string {
	contextLower := strings.ToLower(context)
	messageLower := strings.ToLower(message)

	// Check for error indicators
	errorKeywords := []string{"error", "failed", "fail", "exception", "panic"}
	for _, keyword := range errorKeywords {
		if strings.Contains(contextLower, keyword) || strings.Contains(messageLower, keyword) {
			return "error"
		}
	}

	// Check for warning indicators
	warningKeywords := []string{"warning", "warn", "timeout", "retry", "fallback"}
	for _, keyword := range warningKeywords {
		if strings.Contains(contextLower, keyword) || strings.Contains(messageLower, keyword) {
			return "warn"
		}
	}

	// Check for debug indicators
	debugKeywords := []string{"debug", "trace", "dump", "raw", "chunk"}
	for _, keyword := range debugKeywords {
		if strings.Contains(contextLower, keyword) || strings.Contains(messageLower, keyword) {
			return "debug"
		}
	}

	// Default to info
	return "info"
}
