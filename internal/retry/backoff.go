package retry

import (
	"context"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/livereview/internal/logging"
)

// RetryConfig configures retry behavior with exponential backoff
type RetryConfig struct {
	MaxRetries int           `json:"max_retries"` // Maximum number of retry attempts (default: 3)
	BaseDelay  time.Duration `json:"base_delay"`  // Base delay between retries (default: 1s)
	MaxDelay   time.Duration `json:"max_delay"`   // Maximum delay between retries (default: 30s)
	Multiplier float64       `json:"multiplier"`  // Exponential backoff multiplier (default: 2.0)
	Jitter     bool          `json:"jitter"`      // Add random jitter to prevent thundering herd (default: true)
	LogRetries bool          `json:"log_retries"` // Whether to log retry attempts (default: true)
}

// RetryResult contains information about the retry operation
type RetryResult struct {
	Attempts      int           `json:"attempts"`       // Total number of attempts made
	TotalDuration time.Duration `json:"total_duration"` // Total time spent on all attempts
	LastError     error         `json:"-"`              // Last error encountered
	Success       bool          `json:"success"`        // Whether the operation eventually succeeded
	RetryReasons  []string      `json:"retry_reasons"`  // Reasons for each retry attempt
}

// DefaultRetryConfig returns a retry configuration with sensible defaults
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
		Multiplier: 2.0,
		Jitter:     true,
		LogRetries: true,
	}
}

// LLMRetryConfig returns a retry configuration optimized for LLM requests
func LLMRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  2 * time.Second,  // LLM requests can be slower
		MaxDelay:   60 * time.Second, // Allow longer max delay for LLM
		Multiplier: 2.5,              // Slightly more aggressive backoff
		Jitter:     true,
		LogRetries: true,
	}
}

// RetryWithBackoff executes an operation with exponential backoff retry logic
func RetryWithBackoff(ctx context.Context, config RetryConfig, operation func() error, logger *logging.ReviewLogger) RetryResult {
	return RetryWithBackoffAndReason(ctx, config, func() (error, string) {
		err := operation()
		reason := "unknown_error"
		if err != nil {
			reason = err.Error()
		}
		return err, reason
	}, logger)
}

// RetryWithBackoffAndReason executes an operation with exponential backoff retry logic and custom reason tracking
func RetryWithBackoffAndReason(ctx context.Context, config RetryConfig, operation func() (error, string), logger *logging.ReviewLogger) RetryResult {
	startTime := time.Now()

	result := RetryResult{
		Attempts:     0,
		Success:      false,
		RetryReasons: make([]string, 0),
	}

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		result.Attempts = attempt + 1

		// Log attempt start
		if config.LogRetries && logger != nil {
			if attempt == 0 {
				logger.Log("Starting operation (attempt %d/%d)", attempt+1, config.MaxRetries+1)
			} else {
				logger.Log("Retrying operation (attempt %d/%d)", attempt+1, config.MaxRetries+1)
			}
		}

		// Execute the operation
		err, reason := operation()
		if err == nil {
			// Success!
			result.Success = true
			result.TotalDuration = time.Since(startTime)
			if config.LogRetries && logger != nil {
				if attempt == 0 {
					logger.Log("Operation succeeded on first attempt")
				} else {
					logger.Log("Operation succeeded after %d retries (total duration: %v)", attempt, result.TotalDuration)
				}
			}
			return result
		}

		// Operation failed
		result.LastError = err
		result.RetryReasons = append(result.RetryReasons, reason)

		// Check if we should retry
		if attempt >= config.MaxRetries {
			// No more retries left
			result.TotalDuration = time.Since(startTime)
			if config.LogRetries && logger != nil {
				logger.Log("Operation failed after %d attempts (total duration: %v): %v",
					result.Attempts, result.TotalDuration, err)
			}
			return result
		}

		// Check context cancellation
		if ctx.Err() != nil {
			result.LastError = ctx.Err()
			result.TotalDuration = time.Since(startTime)
			if config.LogRetries && logger != nil {
				logger.Log("Operation cancelled during retry %d: %v", attempt+1, ctx.Err())
			}
			return result
		}

		// Calculate delay for next attempt
		delay := calculateDelay(config, attempt)
		nextAttemptTime := time.Now().Add(delay)

		if config.LogRetries && logger != nil {
			logger.Log("Operation failed (attempt %d/%d): %v", attempt+1, config.MaxRetries+1, err)
			logger.Log("Waiting %v before retry (next attempt at %v)", delay, nextAttemptTime.Format("15:04:05"))
		}

		// Wait before retrying
		select {
		case <-ctx.Done():
			result.LastError = ctx.Err()
			result.TotalDuration = time.Since(startTime)
			if config.LogRetries && logger != nil {
				logger.Log("Operation cancelled during backoff delay: %v", ctx.Err())
			}
			return result
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	// This should never be reached due to the loop logic above
	result.TotalDuration = time.Since(startTime)
	return result
}

// calculateDelay calculates the delay for the next retry attempt using exponential backoff
func calculateDelay(config RetryConfig, attempt int) time.Duration {
	// Calculate exponential backoff: baseDelay * multiplier^attempt
	delay := float64(config.BaseDelay) * math.Pow(config.Multiplier, float64(attempt))

	// Apply maximum delay limit
	if delay > float64(config.MaxDelay) {
		delay = float64(config.MaxDelay)
	}

	// Add jitter to prevent thundering herd problem
	if config.Jitter {
		// Add up to 10% random jitter
		jitterRange := delay * 0.1
		jitter := (rand.Float64() - 0.5) * 2 * jitterRange // Random value between -jitterRange and +jitterRange
		delay += jitter

		// Ensure delay is not negative
		if delay < 0 {
			delay = float64(config.BaseDelay)
		}
	}

	return time.Duration(delay)
}

// IsRetryableError determines if an error is retryable
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Network-related errors that are typically retryable
	retryableErrors := []string{
		"connection refused",
		"connection reset",
		"connection timeout",
		"timeout",
		"temporary failure",
		"service unavailable",
		"too many requests",
		"rate limit",
		"429", // HTTP 429 Too Many Requests
		"502", // HTTP 502 Bad Gateway
		"503", // HTTP 503 Service Unavailable
		"504", // HTTP 504 Gateway Timeout
		"dns lookup failed",
		"no such host",
		"network unreachable",
		"broken pipe",
		"context deadline exceeded",
	}

	for _, retryable := range retryableErrors {
		if contains(errStr, retryable) {
			return true
		}
	}

	return false
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	// Simple case-insensitive substring check
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)

	return strings.Contains(s, substr)
}
