package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/livereview/internal/retry"
)

// Mock LLM client for testing
type mockLLMClient struct {
	responses    []string
	errors       []error
	callCount    int
	shouldRepair bool
}

// Slow mock LLM client for timeout testing
type slowMockLLMClient struct {
	delay time.Duration
}

func (s *slowMockLLMClient) GenerateResponse(ctx context.Context, prompt string) (string, error) {
	// Simulate slow operation
	select {
	case <-time.After(s.delay):
		return `{"status": "success"}`, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *slowMockLLMClient) GenerateStructuredResponse(ctx context.Context, prompt string, target interface{}) error {
	_, err := s.GenerateResponse(ctx, prompt)
	return err
}

func (m *mockLLMClient) GenerateResponse(ctx context.Context, prompt string) (string, error) {
	if m.callCount < len(m.errors) && m.errors[m.callCount] != nil {
		err := m.errors[m.callCount]
		m.callCount++
		return "", err
	}

	if m.callCount < len(m.responses) {
		response := m.responses[m.callCount]
		m.callCount++

		// Simulate malformed JSON if requested
		if m.shouldRepair && response == "good_json" {
			return `{"comments": [{"file": "test.go", "line": 10,}]}`, nil
		}

		return response, nil
	}

	return "default response", nil
}

func (m *mockLLMClient) GenerateStructuredResponse(ctx context.Context, prompt string, target interface{}) error {
	response, err := m.GenerateResponse(ctx, prompt)
	if err != nil {
		return err
	}

	// Simulate successful parsing
	_ = response
	return nil
}

// Mock event sink for testing
type mockEventSink struct {
	retryEvents      []retryEvent
	jsonRepairs      []JsonRepairStats
	timeoutEvents    []timeoutEvent
	batchStatsEvents []BatchStats
}

type retryEvent struct {
	reviewID, orgID int64
	batchID         *string
	attempt         int
	reason          string
}

type timeoutEvent struct {
	reviewID, orgID int64
	batchID         *string
	operation       string
}

func (m *mockEventSink) LogRetryEvent(reviewID, orgID int64, batchID *string, attempt int, reason, delay, nextAttempt string) {
	m.retryEvents = append(m.retryEvents, retryEvent{
		reviewID: reviewID,
		orgID:    orgID,
		batchID:  batchID,
		attempt:  attempt,
		reason:   reason,
	})
}

func (m *mockEventSink) LogJSONRepairEvent(reviewID, orgID int64, batchID *string, stats JsonRepairStats) {
	m.jsonRepairs = append(m.jsonRepairs, stats)
}

func (m *mockEventSink) LogTimeoutEvent(reviewID, orgID int64, batchID *string, operation, configuredTimeout, actualDuration string) {
	m.timeoutEvents = append(m.timeoutEvents, timeoutEvent{
		reviewID:  reviewID,
		orgID:     orgID,
		batchID:   batchID,
		operation: operation,
	})
}

func (m *mockEventSink) LogBatchStatsEvent(reviewID, orgID int64, batchID string, stats BatchStats) {
	m.batchStatsEvents = append(m.batchStatsEvents, stats)
}

func TestResilientClient_SuccessFirstAttempt(t *testing.T) {
	mockClient := &mockLLMClient{
		responses: []string{`{"status": "success"}`},
	}
	eventSink := &mockEventSink{}

	config := retry.RetryConfig{
		MaxRetries: 2,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   100 * time.Millisecond,
		Multiplier: 2.0,
		Jitter:     false,
		LogRetries: false,
	}

	client := NewResilientClient(mockClient, config, eventSink)

	req := ResilientRequest{
		ReviewID: 123,
		OrgID:    456,
		Prompt:   "test prompt",
		Timeout:  5 * time.Second,
	}

	response := client.GenerateResilientResponse(context.Background(), req)

	if !response.Success {
		t.Error("Expected success=true")
	}

	if response.AttemptsMade != 1 {
		t.Errorf("Expected 1 attempt, got %d", response.AttemptsMade)
	}

	if len(response.RetryReasons) != 0 {
		t.Errorf("Expected no retry reasons, got %d", len(response.RetryReasons))
	}

	if len(eventSink.retryEvents) != 0 {
		t.Errorf("Expected no retry events, got %d", len(eventSink.retryEvents))
	}
}

func TestResilientClient_RetryWithEventualSuccess(t *testing.T) {
	mockClient := &mockLLMClient{
		errors: []error{
			errors.New("connection timeout"), // First attempt fails
			errors.New("rate limit"),         // Second attempt fails
			nil,                              // Third attempt succeeds
		},
		responses: []string{"", "", `{"status": "success"}`},
	}
	eventSink := &mockEventSink{}

	config := retry.RetryConfig{
		MaxRetries: 3,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   100 * time.Millisecond,
		Multiplier: 2.0,
		Jitter:     false,
		LogRetries: false,
	}

	client := NewResilientClient(mockClient, config, eventSink)

	req := ResilientRequest{
		ReviewID: 123,
		OrgID:    456,
		Prompt:   "test prompt",
	}

	response := client.GenerateResilientResponse(context.Background(), req)

	if !response.Success {
		t.Error("Expected success=true")
	}

	if response.AttemptsMade != 3 {
		t.Errorf("Expected 3 attempts, got %d", response.AttemptsMade)
	}

	if len(response.RetryReasons) != 2 {
		t.Errorf("Expected 2 retry reasons, got %d", len(response.RetryReasons))
	}

	if len(eventSink.retryEvents) != 2 {
		t.Errorf("Expected 2 retry events, got %d", len(eventSink.retryEvents))
	}
}

func TestResilientClient_JSONRepair(t *testing.T) {
	mockClient := &mockLLMClient{
		responses:    []string{"good_json"},
		shouldRepair: true, // This will cause malformed JSON to be returned
	}
	eventSink := &mockEventSink{}

	config := retry.DefaultRetryConfig()
	config.LogRetries = false

	client := NewResilientClient(mockClient, config, eventSink)

	req := ResilientRequest{
		ReviewID: 123,
		OrgID:    456,
		Prompt:   "test prompt",
	}

	response := client.GenerateResilientResponse(context.Background(), req)

	if !response.Success {
		t.Error("Expected success=true")
	}

	if !response.JsonRepaired {
		t.Error("Expected JsonRepaired=true")
	}

	if response.RepairStats == nil {
		t.Error("Expected RepairStats to be populated")
	}

	if len(eventSink.jsonRepairs) != 1 {
		t.Errorf("Expected 1 JSON repair event, got %d", len(eventSink.jsonRepairs))
	}
}

func TestBatchProcessor(t *testing.T) {
	mockClient := &mockLLMClient{
		responses: []string{
			`{"status": "success"}`,
			`{"status": "success"}`,
			`{"status": "success"}`,
		},
	}
	eventSink := &mockEventSink{}

	config := retry.DefaultRetryConfig()
	config.LogRetries = false

	resilientClient := NewResilientClient(mockClient, config, eventSink)
	processor := NewBatchProcessor(resilientClient)

	requests := []ResilientRequest{
		{ReviewID: 123, OrgID: 456, Prompt: "prompt 1"},
		{ReviewID: 123, OrgID: 456, Prompt: "prompt 2"},
		{ReviewID: 123, OrgID: 456, Prompt: "prompt 3"},
	}

	responses := processor.ProcessBatch(context.Background(), requests, "batch-123")

	if len(responses) != 3 {
		t.Errorf("Expected 3 responses, got %d", len(responses))
	}

	for i, response := range responses {
		if !response.Success {
			t.Errorf("Expected response %d to be successful", i)
		}
	}

	if len(eventSink.batchStatsEvents) != 1 {
		t.Errorf("Expected 1 batch stats event, got %d", len(eventSink.batchStatsEvents))
	}

	stats := eventSink.batchStatsEvents[0]
	if stats.TotalRequests != 3 {
		t.Errorf("Expected 3 total requests, got %d", stats.TotalRequests)
	}

	if stats.Successful != 3 {
		t.Errorf("Expected 3 successful requests, got %d", stats.Successful)
	}
}

func TestResilientClient_Timeout(t *testing.T) {
	// Create a mock client that simulates a slow operation by sleeping
	slowClient := &slowMockLLMClient{
		delay: 100 * time.Millisecond,
	}

	eventSink := &mockEventSink{}

	config := retry.RetryConfig{
		MaxRetries: 1,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   100 * time.Millisecond,
		Multiplier: 2.0,
		Jitter:     false,
		LogRetries: false,
	}

	client := NewResilientClient(slowClient, config, eventSink)

	req := ResilientRequest{
		ReviewID: 123,
		OrgID:    456,
		Prompt:   "test prompt",
		Timeout:  10 * time.Millisecond, // Timeout before the slow operation completes
	}

	response := client.GenerateResilientResponse(context.Background(), req)

	if response.Success {
		t.Error("Expected success=false due to timeout")
	}

	// The response should indicate it was cancelled due to timeout
	if response.TotalDuration < 10*time.Millisecond {
		t.Errorf("Expected timeout duration >= 10ms, got %v", response.TotalDuration)
	}
}

func TestResilientClient_ConfigurationUpdate(t *testing.T) {
	mockClient := &mockLLMClient{}
	eventSink := &mockEventSink{}

	initialConfig := retry.DefaultRetryConfig()
	client := NewResilientClient(mockClient, initialConfig, eventSink)

	// Verify initial config
	currentConfig := client.GetRetryConfig()
	if currentConfig.MaxRetries != initialConfig.MaxRetries {
		t.Errorf("Expected MaxRetries=%d, got %d", initialConfig.MaxRetries, currentConfig.MaxRetries)
	}

	// Update config
	newConfig := retry.RetryConfig{
		MaxRetries: 5,
		BaseDelay:  2 * time.Second,
		MaxDelay:   60 * time.Second,
		Multiplier: 3.0,
		Jitter:     false,
		LogRetries: true,
	}

	client.UpdateRetryConfig(newConfig)

	// Verify updated config
	updatedConfig := client.GetRetryConfig()
	if updatedConfig.MaxRetries != 5 {
		t.Errorf("Expected updated MaxRetries=5, got %d", updatedConfig.MaxRetries)
	}

	if updatedConfig.BaseDelay != 2*time.Second {
		t.Errorf("Expected updated BaseDelay=2s, got %v", updatedConfig.BaseDelay)
	}
}
