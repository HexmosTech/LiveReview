package payment

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestShouldRetryConfirmProviderRead(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "status 400", err: fmt.Errorf("razorpay API error (status 400): bad request"), want: false},
		{name: "status 401", err: fmt.Errorf("razorpay API error (status 401): unauthorized"), want: false},
		{name: "status 403", err: fmt.Errorf("razorpay API error (status 403): forbidden"), want: false},
		{name: "status 404", err: fmt.Errorf("razorpay API error (status 404): not found"), want: true},
		{name: "status 500", err: fmt.Errorf("razorpay API error (status 500): internal"), want: true},
		{name: "network error", err: fmt.Errorf("error making request: connection reset"), want: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := shouldRetryConfirmProviderRead(tc.err)
			if got != tc.want {
				t.Fatalf("shouldRetryConfirmProviderRead(%v) = %t, want %t", tc.err, got, tc.want)
			}
		})
	}
}

func TestFetchRazorpayWithRetryRetriesAndSucceeds(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	sleeps := 0

	type sample struct {
		ID string
	}

	result, err := fetchRazorpayWithRetry(ctx, "payment", 4, 10*time.Millisecond, func() (*sample, error) {
		attempts++
		if attempts < 3 {
			return nil, fmt.Errorf("error making request: temporary timeout")
		}
		return &sample{ID: "ok"}, nil
	}, func(_ time.Duration) {
		sleeps++
	})
	if err != nil {
		t.Fatalf("fetchRazorpayWithRetry returned error: %v", err)
	}
	if result == nil || result.ID != "ok" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if sleeps != 2 {
		t.Fatalf("sleeps = %d, want 2", sleeps)
	}
}

func TestFetchRazorpayWithRetryStopsOnNonRetryableError(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	type sample struct{}

	_, err := fetchRazorpayWithRetry(ctx, "subscription", 4, 10*time.Millisecond, func() (*sample, error) {
		attempts++
		return nil, fmt.Errorf("razorpay API error (status 401): unauthorized")
	}, func(_ time.Duration) {
		t.Fatalf("sleep should not be called for non-retryable errors")
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestFetchRazorpayWithRetryRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	type sample struct{}

	_, err := fetchRazorpayWithRetry(ctx, "payment", 4, 10*time.Millisecond, func() (*sample, error) {
		return nil, errors.New("should not run")
	}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}
