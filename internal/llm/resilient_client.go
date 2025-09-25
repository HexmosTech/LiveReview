package llm

import (
	"context"
	"time"

	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/retry"
)

// ResilientClient wraps an LLM client with retry logic, timeout handling, and comprehensive logging
type ResilientClient struct {
	client      LLMClient             // The underlying LLM client
	retryConfig retry.RetryConfig     // Retry configuration
	eventSink   EventSink             // Event sink for logging resiliency events
	logger      *logging.ReviewLogger // Logger instance
}

// LLMClient defines the interface for LLM clients
type LLMClient interface {
	GenerateResponse(ctx context.Context, prompt string) (string, error)
	GenerateStructuredResponse(ctx context.Context, prompt string, target interface{}) error
}

// EventSink defines the interface for logging resiliency events
type EventSink interface {
	LogRetryEvent(reviewID, orgID int64, batchID *string, attempt int, reason, delay, nextAttempt string)
	LogJSONRepairEvent(reviewID, orgID int64, batchID *string, stats JsonRepairStats)
	LogTimeoutEvent(reviewID, orgID int64, batchID *string, operation, configuredTimeout, actualDuration string)
	LogBatchStatsEvent(reviewID, orgID int64, batchID string, stats BatchStats)
}

// BatchStats contains statistics about a batch of requests
type BatchStats struct {
	TotalRequests   int
	Successful      int
	Retries         int
	JsonRepairs     int
	AvgResponseTime time.Duration
}

// NewResilientClient creates a new resilient LLM client wrapper
func NewResilientClient(client LLMClient, config retry.RetryConfig, eventSink EventSink) *ResilientClient {
	return &ResilientClient{
		client:      client,
		retryConfig: config,
		eventSink:   eventSink,
		logger:      logging.GetCurrentLogger(),
	}
}

// NewResilientClientWithDefaults creates a resilient client with default retry configuration
func NewResilientClientWithDefaults(client LLMClient, eventSink EventSink) *ResilientClient {
	return NewResilientClient(client, retry.LLMRetryConfig(), eventSink)
}

// ResilientRequest represents a request with resiliency context
type ResilientRequest struct {
	ReviewID int64
	OrgID    int64
	BatchID  *string
	Prompt   string
	Timeout  time.Duration
}

// ResilientResponse represents a response with resiliency information
type ResilientResponse struct {
	Response      string
	Success       bool
	AttemptsMade  int
	TotalDuration time.Duration
	JsonRepaired  bool
	RepairStats   *JsonRepairStats
	RetryReasons  []string
}

// GenerateResilientResponse generates a response with full resiliency features
func (rc *ResilientClient) GenerateResilientResponse(ctx context.Context, req ResilientRequest) ResilientResponse {
	startTime := time.Now()

	// Apply timeout if specified
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	response := ResilientResponse{
		Success: false,
	}

	// Execute with retry logic
	result := retry.RetryWithBackoffAndReason(ctx, rc.retryConfig, func() (error, string) {
		attemptStart := time.Now()

		// Generate response from underlying client
		rawResponse, err := rc.client.GenerateResponse(ctx, req.Prompt)
		_ = time.Since(attemptStart) // Track duration but don't use it yet

		if err != nil {
			// Log retry event if this isn't the last attempt
			reason := err.Error()
			if retry.IsRetryableError(err) {
				return err, reason
			}
			// Non-retryable error - fail immediately
			return err, reason
		}

		// Process response with JSON repair if needed
		processResult, processErr := ProcessLLMResponse(rawResponse, &map[string]interface{}{})
		if processErr != nil {
			if rc.logger != nil {
				rc.logger.Log("LLM response processing failed: %v", processErr)
			}
			return processErr, "json_processing_failed"
		}

		// Log JSON repair event if repair was performed
		if processResult.RepairStats.WasRepaired && rc.eventSink != nil {
			rc.eventSink.LogJSONRepairEvent(req.ReviewID, req.OrgID, req.BatchID, processResult.RepairStats)
			response.JsonRepaired = true
			response.RepairStats = &processResult.RepairStats
		}

		response.Response = processResult.RepairedJSON
		return nil, "success"
	})

	// Populate response details
	response.Success = result.Success
	response.AttemptsMade = result.Attempts
	response.TotalDuration = result.TotalDuration
	response.RetryReasons = result.RetryReasons

	// Log retry events for failed attempts
	if rc.eventSink != nil && len(result.RetryReasons) > 0 {
		for i, reason := range result.RetryReasons {
			attempt := i + 1
			delay := retry.DefaultRetryConfig().BaseDelay * time.Duration(attempt)
			nextAttempt := startTime.Add(delay)

			rc.eventSink.LogRetryEvent(
				req.ReviewID, req.OrgID, req.BatchID,
				attempt, reason,
				delay.String(), nextAttempt.Format(time.RFC3339),
			)
		}
	}

	// Log timeout event if context was cancelled due to timeout
	if ctx.Err() == context.DeadlineExceeded && rc.eventSink != nil {
		configuredTimeout := req.Timeout.String()
		if req.Timeout == 0 {
			configuredTimeout = "none"
		}
		rc.eventSink.LogTimeoutEvent(
			req.ReviewID, req.OrgID, req.BatchID,
			"llm_generate_response", configuredTimeout, response.TotalDuration.String(),
		)
	}

	return response
}

// GenerateStructuredResilientResponse generates a structured response with resiliency
func (rc *ResilientClient) GenerateStructuredResilientResponse(ctx context.Context, req ResilientRequest, target interface{}) ResilientResponse {
	startTime := time.Now()

	// Apply timeout if specified
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	response := ResilientResponse{
		Success: false,
	}

	// Execute with retry logic
	result := retry.RetryWithBackoffAndReason(ctx, rc.retryConfig, func() (error, string) {
		attemptStart := time.Now()

		// Generate structured response from underlying client
		err := rc.client.GenerateStructuredResponse(ctx, req.Prompt, target)
		attemptDuration := time.Since(attemptStart)

		if err != nil {
			reason := err.Error()
			if retry.IsRetryableError(err) {
				return err, reason
			}
			return err, reason
		}

		// For structured responses, the client should handle JSON parsing internally
		// We just log the successful generation
		if rc.logger != nil {
			rc.logger.Log("Structured response generated successfully in %v", attemptDuration)
		}

		return nil, "success"
	})

	// Populate response details
	response.Success = result.Success
	response.AttemptsMade = result.Attempts
	response.TotalDuration = result.TotalDuration
	response.RetryReasons = result.RetryReasons

	// Log retry events for failed attempts
	if rc.eventSink != nil && len(result.RetryReasons) > 0 {
		for i, reason := range result.RetryReasons {
			attempt := i + 1
			delay := retry.DefaultRetryConfig().BaseDelay * time.Duration(attempt)
			nextAttempt := startTime.Add(delay)

			rc.eventSink.LogRetryEvent(
				req.ReviewID, req.OrgID, req.BatchID,
				attempt, reason,
				delay.String(), nextAttempt.Format(time.RFC3339),
			)
		}
	}

	// Log timeout event if context was cancelled due to timeout
	if ctx.Err() == context.DeadlineExceeded && rc.eventSink != nil {
		configuredTimeout := req.Timeout.String()
		if req.Timeout == 0 {
			configuredTimeout = "none"
		}
		rc.eventSink.LogTimeoutEvent(
			req.ReviewID, req.OrgID, req.BatchID,
			"llm_generate_structured_response", configuredTimeout, response.TotalDuration.String(),
		)
	}

	return response
}

// BatchProcessor handles batch processing with comprehensive statistics
type BatchProcessor struct {
	client *ResilientClient
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(client *ResilientClient) *BatchProcessor {
	return &BatchProcessor{client: client}
}

// ProcessBatch processes multiple requests and logs batch statistics
func (bp *BatchProcessor) ProcessBatch(ctx context.Context, requests []ResilientRequest, batchID string) []ResilientResponse {
	startTime := time.Now()
	responses := make([]ResilientResponse, len(requests))

	stats := BatchStats{
		TotalRequests: len(requests),
	}

	// Process each request
	for i, req := range requests {
		req.BatchID = &batchID // Ensure batch ID is set

		response := bp.client.GenerateResilientResponse(ctx, req)
		responses[i] = response

		// Update statistics
		if response.Success {
			stats.Successful++
		}
		stats.Retries += (response.AttemptsMade - 1) // Don't count the first attempt as a retry
		if response.JsonRepaired {
			stats.JsonRepairs++
		}
	}

	// Calculate average response time
	totalDuration := time.Since(startTime)
	if stats.TotalRequests > 0 {
		stats.AvgResponseTime = totalDuration / time.Duration(stats.TotalRequests)
	}

	// Log batch statistics
	if bp.client.eventSink != nil && len(requests) > 0 {
		bp.client.eventSink.LogBatchStatsEvent(
			requests[0].ReviewID, requests[0].OrgID, batchID, stats,
		)
	}

	return responses
}

// UpdateRetryConfig updates the retry configuration
func (rc *ResilientClient) UpdateRetryConfig(config retry.RetryConfig) {
	rc.retryConfig = config
	if rc.logger != nil {
		rc.logger.Log("Updated retry configuration: MaxRetries=%d, BaseDelay=%v, MaxDelay=%v",
			config.MaxRetries, config.BaseDelay, config.MaxDelay)
	}
}

// GetRetryConfig returns the current retry configuration
func (rc *ResilientClient) GetRetryConfig() retry.RetryConfig {
	return rc.retryConfig
}
