package logging

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// fakeEventSink records every call so the test can assert on exact shape,
// instead of a real DB-backed sink.
type fakeEventSink struct {
	batchEvents []fakeBatchEvent
	logEvents   []fakeLogEvent
}

type fakeBatchEvent struct {
	batchID       string
	status        string
	tokenEstimate int
	fileCount     int
}

type fakeLogEvent struct {
	level   string
	message string
	batchID string
}

func (f *fakeEventSink) EmitStatusEvent(ctx context.Context, reviewID, orgID int64, status string) error {
	return nil
}

func (f *fakeEventSink) EmitLogEvent(ctx context.Context, reviewID, orgID int64, level, message, batchID string) error {
	f.logEvents = append(f.logEvents, fakeLogEvent{level: level, message: message, batchID: batchID})
	return nil
}

func (f *fakeEventSink) EmitBatchEvent(ctx context.Context, reviewID, orgID int64, batchID, status string, tokenEstimate, fileCount int, comments interface{}) error {
	f.batchEvents = append(f.batchEvents, fakeBatchEvent{
		batchID: batchID, status: status, tokenEstimate: tokenEstimate, fileCount: fileCount,
	})
	return nil
}

func (f *fakeEventSink) EmitArtifactEvent(ctx context.Context, reviewID, orgID int64, kind, url, batchID string, sizeBytes int64, previewHead, previewTail string) error {
	return nil
}

func (f *fakeEventSink) EmitCompletionEvent(ctx context.Context, reviewID, orgID int64, resultSummary string, commentCount int, errorSummary string) error {
	return nil
}

func newTestLogger(t *testing.T, sink EventSink) *ReviewLogger {
	t.Helper()
	tmpFile, err := os.CreateTemp(t.TempDir(), "review-log-*.log")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	t.Cleanup(func() { tmpFile.Close() })

	return &ReviewLogger{
		reviewID:    "13536",
		reviewIDInt: 13536,
		orgID:       3,
		logFile:     tmpFile,
		startTime:   time.Now(),
		eventSink:   sink,
	}
}

// TestEmitBatchFailed_EmitsDistinctTerminalFailureEvent is the regression
// test for the Events-tab fake-success bug: a batch that exhausts retries
// must emit a batch event whose status is explicitly "failed" - not fold
// into the same free-text "log" event type a transient per-attempt retry
// error uses, which the frontend had no reliable way to distinguish from a
// genuine terminal failure (see ReviewProgressView.tsx's markFailed/
// markCompleted handling).
func TestEmitBatchFailed_EmitsDistinctTerminalFailureEvent(t *testing.T) {
	sink := &fakeEventSink{}
	logger := newTestLogger(t, sink)

	logger.EmitBatchFailed("batch-6", errors.New("request timeout: API call exceeded deadline"))

	if len(sink.batchEvents) != 1 {
		t.Fatalf("got %d batch events, want exactly 1", len(sink.batchEvents))
	}
	got := sink.batchEvents[0]
	if got.batchID != "batch-6" {
		t.Errorf("batchID = %q, want batch-6", got.batchID)
	}
	if got.status != "failed" {
		t.Errorf("status = %q, want %q - this is the field the frontend must key off instead of fileCount fallback", got.status, "failed")
	}

	if len(sink.logEvents) != 1 {
		t.Fatalf("got %d log events, want exactly 1 (r.Log's own eventSink branch, not a duplicate)", len(sink.logEvents))
	}
	if sink.logEvents[0].level != "error" {
		t.Errorf("log event level = %q, want error", sink.logEvents[0].level)
	}
}

func TestEmitBatchFailed_NilSafe(t *testing.T) {
	var logger *ReviewLogger
	logger.EmitBatchFailed("batch-1", errors.New("x")) // must not panic

	logger2 := newTestLogger(t, nil)
	logger2.EmitBatchFailed("batch-1", errors.New("x")) // nil eventSink must not panic
}

func TestEmitBatchFailed_NilErrorUsesFallbackMessage(t *testing.T) {
	sink := &fakeEventSink{}
	logger := newTestLogger(t, sink)

	logger.EmitBatchFailed("batch-2", nil)

	if len(sink.logEvents) != 1 {
		t.Fatalf("got %d log events, want 1", len(sink.logEvents))
	}
	if sink.logEvents[0].message == "" {
		t.Error("expected a non-empty fallback message when err is nil")
	}
}
