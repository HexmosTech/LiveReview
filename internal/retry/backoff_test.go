package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries=3, got %d", config.MaxRetries)
	}

	if config.BaseDelay != time.Second {
		t.Errorf("Expected BaseDelay=1s, got %v", config.BaseDelay)
	}

	if config.MaxDelay != 30*time.Second {
		t.Errorf("Expected MaxDelay=30s, got %v", config.MaxDelay)
	}

	if config.Multiplier != 2.0 {
		t.Errorf("Expected Multiplier=2.0, got %f", config.Multiplier)
	}

	if !config.Jitter {
		t.Error("Expected Jitter=true")
	}

	if !config.LogRetries {
		t.Error("Expected LogRetries=true")
	}
}

func TestLLMRetryConfig(t *testing.T) {
	config := LLMRetryConfig()

	if config.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries=3, got %d", config.MaxRetries)
	}

	if config.BaseDelay != 2*time.Second {
		t.Errorf("Expected BaseDelay=2s, got %v", config.BaseDelay)
	}

	if config.MaxDelay != 60*time.Second {
		t.Errorf("Expected MaxDelay=60s, got %v", config.MaxDelay)
	}

	if config.Multiplier != 2.5 {
		t.Errorf("Expected Multiplier=2.5, got %f", config.Multiplier)
	}
}

func TestRetryWithBackoff_Success(t *testing.T) {
	config := RetryConfig{
		MaxRetries: 2,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   100 * time.Millisecond,
		Multiplier: 2.0,
		Jitter:     false, // Disable jitter for predictable testing
		LogRetries: false, // Disable logging for cleaner test output
	}

	result := RetryWithBackoff(context.Background(), config, func() error {
		return nil // Success on first attempt
	}, nil)

	if !result.Success {
		t.Error("Expected success=true")
	}

	if result.Attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", result.Attempts)
	}

	if result.LastError != nil {
		t.Errorf("Expected no error, got %v", result.LastError)
	}

	if len(result.RetryReasons) != 0 {
		t.Errorf("Expected no retry reasons, got %d", len(result.RetryReasons))
	}
}

func TestRetryWithBackoff_EventualSuccess(t *testing.T) {
	config := RetryConfig{
		MaxRetries: 3,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   100 * time.Millisecond,
		Multiplier: 2.0,
		Jitter:     false,
		LogRetries: false,
	}

	attempts := 0
	result := RetryWithBackoff(context.Background(), config, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary failure")
		}
		return nil // Success on third attempt
	}, nil)

	if !result.Success {
		t.Error("Expected success=true")
	}

	if result.Attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", result.Attempts)
	}

	if len(result.RetryReasons) != 2 {
		t.Errorf("Expected 2 retry reasons, got %d", len(result.RetryReasons))
	}

	if result.TotalDuration == 0 {
		t.Error("Expected non-zero total duration")
	}
}

func TestRetryWithBackoff_AllAttemptsFailure(t *testing.T) {
	config := RetryConfig{
		MaxRetries: 2,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   100 * time.Millisecond,
		Multiplier: 2.0,
		Jitter:     false,
		LogRetries: false,
	}

	expectedError := errors.New("persistent failure")
	result := RetryWithBackoff(context.Background(), config, func() error {
		return expectedError
	}, nil)

	if result.Success {
		t.Error("Expected success=false")
	}

	if result.Attempts != 3 { // MaxRetries + 1
		t.Errorf("Expected 3 attempts, got %d", result.Attempts)
	}

	if result.LastError != expectedError {
		t.Errorf("Expected last error to be %v, got %v", expectedError, result.LastError)
	}

	if len(result.RetryReasons) != 3 {
		t.Errorf("Expected 3 retry reasons, got %d", len(result.RetryReasons))
	}
}

func TestRetryWithBackoff_ContextCancellation(t *testing.T) {
	config := RetryConfig{
		MaxRetries: 5,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   1 * time.Second,
		Multiplier: 2.0,
		Jitter:     false,
		LogRetries: false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := RetryWithBackoff(ctx, config, func() error {
		return errors.New("always fails")
	}, nil)

	if result.Success {
		t.Error("Expected success=false due to context cancellation")
	}

	if result.LastError != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", result.LastError)
	}

	// Should fail quickly due to context timeout
	if result.Attempts > 2 {
		t.Errorf("Expected few attempts due to quick timeout, got %d", result.Attempts)
	}
}

func TestCalculateDelay(t *testing.T) {
	config := RetryConfig{
		BaseDelay:  1 * time.Second,
		MaxDelay:   10 * time.Second,
		Multiplier: 2.0,
		Jitter:     false,
	}

	// Test exponential growth
	delay0 := calculateDelay(config, 0)
	delay1 := calculateDelay(config, 1)
	delay2 := calculateDelay(config, 2)

	if delay0 != 1*time.Second {
		t.Errorf("Expected delay0=1s, got %v", delay0)
	}

	if delay1 != 2*time.Second {
		t.Errorf("Expected delay1=2s, got %v", delay1)
	}

	if delay2 != 4*time.Second {
		t.Errorf("Expected delay2=4s, got %v", delay2)
	}

	// Test max delay cap
	delay10 := calculateDelay(config, 10) // Should be capped at MaxDelay
	if delay10 != 10*time.Second {
		t.Errorf("Expected delay10=10s (capped), got %v", delay10)
	}
}

func TestCalculateDelay_WithJitter(t *testing.T) {
	config := RetryConfig{
		BaseDelay:  1 * time.Second,
		MaxDelay:   10 * time.Second,
		Multiplier: 2.0,
		Jitter:     true,
	}

	// Test that jitter produces different values
	delay1a := calculateDelay(config, 1)
	delay1b := calculateDelay(config, 1)
	delay1c := calculateDelay(config, 1)

	// Should be close to 2s but with some variation
	expectedBase := 2 * time.Second
	tolerance := 200 * time.Millisecond // 10% of 2s

	if abs(delay1a-expectedBase) > tolerance {
		t.Errorf("delay1a %v too far from expected %v", delay1a, expectedBase)
	}

	// At least one should be different (very high probability with jitter)
	if delay1a == delay1b && delay1b == delay1c {
		t.Error("Expected some variation with jitter enabled")
	}
}

func TestIsRetryableError(t *testing.T) {
	retryableErrors := []error{
		errors.New("connection refused"),
		errors.New("connection timeout"),
		errors.New("temporary failure"),
		errors.New("HTTP 429 Too Many Requests"),
		errors.New("HTTP 502 Bad Gateway"),
		errors.New("HTTP 503 Service Unavailable"),
		errors.New("DNS lookup failed"),
		errors.New("context deadline exceeded"),
	}

	for _, err := range retryableErrors {
		if !IsRetryableError(err) {
			t.Errorf("Expected %v to be retryable", err)
		}
	}

	nonRetryableErrors := []error{
		errors.New("invalid input"),
		errors.New("permission denied"),
		errors.New("HTTP 400 Bad Request"),
		errors.New("HTTP 401 Unauthorized"),
		errors.New("HTTP 404 Not Found"),
	}

	for _, err := range nonRetryableErrors {
		if IsRetryableError(err) {
			t.Errorf("Expected %v to NOT be retryable", err)
		}
	}

	// Test nil error
	if IsRetryableError(nil) {
		t.Error("Expected nil error to NOT be retryable")
	}
}

func TestRetryWithBackoffAndReason(t *testing.T) {
	config := RetryConfig{
		MaxRetries: 2,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   100 * time.Millisecond,
		Multiplier: 2.0,
		Jitter:     false,
		LogRetries: false,
	}

	attempts := 0
	result := RetryWithBackoffAndReason(context.Background(), config, func() (error, string) {
		attempts++
		switch attempts {
		case 1:
			return errors.New("network timeout"), "network_timeout"
		case 2:
			return errors.New("rate limited"), "rate_limit"
		default:
			return nil, "success"
		}
	}, nil)

	if !result.Success {
		t.Error("Expected success=true")
	}

	if result.Attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", result.Attempts)
	}

	expectedReasons := []string{"network_timeout", "rate_limit"}
	if len(result.RetryReasons) != len(expectedReasons) {
		t.Errorf("Expected %d retry reasons, got %d", len(expectedReasons), len(result.RetryReasons))
	}

	for i, expected := range expectedReasons {
		if result.RetryReasons[i] != expected {
			t.Errorf("Expected retry reason %d to be %s, got %s", i, expected, result.RetryReasons[i])
		}
	}
}

// Helper function to calculate absolute difference between durations
func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
