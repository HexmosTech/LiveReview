package payment

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSubscriptionCancelRequestJSONUsesBoolean(t *testing.T) {
	payload, err := json.Marshal(SubscriptionCancelRequest{CancelAtCycleEnd: true})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(payload), ":1") || strings.Contains(string(payload), ":0") {
		t.Fatalf("expected boolean cancel_at_cycle_end payload, got %s", string(payload))
	}
	if !strings.Contains(string(payload), `"cancel_at_cycle_end":true`) {
		t.Fatalf("expected boolean true payload, got %s", string(payload))
	}
}

func TestCancellationVerifiedImmediate(t *testing.T) {
	pre := &RazorpaySubscription{Status: "active", ChargeAt: 1000}
	cancelResp := &RazorpaySubscription{Status: "cancelled", EndedAt: 1234}
	post := &RazorpaySubscription{Status: "cancelled", EndedAt: 1234}

	ok, reason := cancellationVerified(pre, cancelResp, post, true)
	if !ok {
		t.Fatalf("expected immediate cancellation to verify, got reason: %s", reason)
	}
}

func TestCancellationVerifiedCycleEndWithSignal(t *testing.T) {
	pre := &RazorpaySubscription{Status: "active", EndAt: 2000, ChargeAt: 1500, RemainingCount: 5}
	cancelResp := &RazorpaySubscription{Status: "active"}
	post := &RazorpaySubscription{Status: "active", EndAt: 1700, ChargeAt: 1500, RemainingCount: 5}

	ok, reason := cancellationVerified(pre, cancelResp, post, false)
	if !ok {
		t.Fatalf("expected cycle-end cancellation to verify, got reason: %s", reason)
	}
}

func TestCancellationVerifiedCycleEndWithExplicitProviderMarker(t *testing.T) {
	pre := &RazorpaySubscription{Status: "active", EndAt: 2000, ChargeAt: 1500, RemainingCount: 5}
	cancelResp := &RazorpaySubscription{Status: "active", CancelAtCycleEnd: true}
	post := &RazorpaySubscription{Status: "active", EndAt: 2000, ChargeAt: 1500, RemainingCount: 5}

	ok, reason := cancellationVerified(pre, cancelResp, post, false)
	if !ok {
		t.Fatalf("expected cycle-end cancellation marker to verify, got reason: %s", reason)
	}
}

func TestCancellationVerifiedCycleEndWithCancelAPIAcknowledgementOnlyIsVerified(t *testing.T) {
	pre := &RazorpaySubscription{ID: "sub_123", Status: "active", EndAt: 2000, ChargeAt: 1500, RemainingCount: 5}
	cancelResp := &RazorpaySubscription{ID: "sub_123", Status: "active"}
	post := &RazorpaySubscription{ID: "sub_123", Status: "active", EndAt: 2000, ChargeAt: 1500, RemainingCount: 5}

	ok, reason := cancellationVerified(pre, cancelResp, post, false)
	if !ok {
		t.Fatalf("expected acknowledgement-only cycle-end cancellation to verify, got reason: %s", reason)
	}
}

func TestCancellationVerifiedCycleEndUnverified(t *testing.T) {
	pre := &RazorpaySubscription{Status: "active", EndAt: 2000, ChargeAt: 1500, RemainingCount: 5}
	cancelResp := &RazorpaySubscription{Status: "active", EndAt: 2000, ChargeAt: 1500, RemainingCount: 5}
	post := &RazorpaySubscription{Status: "active", EndAt: 2000, ChargeAt: 1500, RemainingCount: 5}

	ok, reason := cancellationVerified(pre, cancelResp, post, false)
	if ok {
		t.Fatalf("expected unverified cycle-end cancellation to fail verification")
	}
	if reason == "" {
		t.Fatalf("expected verification failure reason")
	}
}

func TestVerifyCancellationWithRetrySucceedsAfterDelayedSignal(t *testing.T) {
	pre := &RazorpaySubscription{Status: "active", EndAt: 2000, ChargeAt: 1500, RemainingCount: 5}
	cancelResp := &RazorpaySubscription{Status: "active"}

	attempt := 0
	post, reason, err := verifyCancellationWithRetry(context.Background(), pre, cancelResp, false, 4, func() (*RazorpaySubscription, error) {
		attempt++
		if attempt < 3 {
			return &RazorpaySubscription{Status: "active", EndAt: 2000, ChargeAt: 1500, RemainingCount: 5}, nil
		}
		return &RazorpaySubscription{Status: "active", HasScheduledChanges: true}, nil
	}, func(time.Duration) {})

	if err != nil {
		t.Fatalf("expected no fetch error, got: %v", err)
	}
	if reason != "" {
		t.Fatalf("expected empty failure reason, got: %s", reason)
	}
	if post == nil || !post.HasScheduledChanges {
		t.Fatalf("expected verified post-cancel subscription with scheduled changes")
	}
	if attempt != 3 {
		t.Fatalf("expected 3 attempts before verification, got %d", attempt)
	}
}

func TestVerifyCancellationWithRetryExhaustsAttempts(t *testing.T) {
	pre := &RazorpaySubscription{Status: "active", EndAt: 2000, ChargeAt: 1500, RemainingCount: 5}
	cancelResp := &RazorpaySubscription{Status: "active", EndAt: 2000, ChargeAt: 1500, RemainingCount: 5}

	attempt := 0
	post, reason, err := verifyCancellationWithRetry(context.Background(), pre, cancelResp, false, 3, func() (*RazorpaySubscription, error) {
		attempt++
		return &RazorpaySubscription{Status: "active", EndAt: 2000, ChargeAt: 1500, RemainingCount: 5}, nil
	}, func(time.Duration) {})

	if err != nil {
		t.Fatalf("expected no fetch error, got: %v", err)
	}
	if post != nil {
		t.Fatalf("expected nil post-cancel subscription on verification exhaustion")
	}
	if reason == "" {
		t.Fatalf("expected non-empty failure reason")
	}
	if attempt != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempt)
	}
}

func TestVerifyCancellationWithRetryReturnsFetchErrorAfterExhaustion(t *testing.T) {
	pre := &RazorpaySubscription{Status: "active"}
	cancelResp := &RazorpaySubscription{Status: "active"}
	wantErr := errors.New("temporary razorpay read failure")

	post, reason, err := verifyCancellationWithRetry(context.Background(), pre, cancelResp, false, 2, func() (*RazorpaySubscription, error) {
		return nil, wantErr
	}, func(time.Duration) {})

	if post != nil {
		t.Fatalf("expected nil post-cancel subscription when fetch fails")
	}
	if reason != "" {
		t.Fatalf("expected empty reason when fetch error is returned, got: %s", reason)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected fetch error %v, got %v", wantErr, err)
	}
}

func TestIsNoPendingScheduledChangeError(t *testing.T) {
	body := []byte(`{"error":{"code":"BAD_REQUEST_ERROR","description":"No Pending update for this subscription"}}`)
	if !isNoPendingScheduledChangeError(400, body) {
		t.Fatalf("expected no-pending-update response to be recognized")
	}

	otherBody := []byte(`{"error":{"code":"BAD_REQUEST_ERROR","description":"Subscription is not cancellable in expired status."}}`)
	if isNoPendingScheduledChangeError(400, otherBody) {
		t.Fatalf("expected non-pending-update error to be rejected")
	}

	if isNoPendingScheduledChangeError(200, body) {
		t.Fatalf("expected non-error status to be rejected")
	}
}

func TestKeepPlanVerifiedWhenNoPendingAndMarkersCleared(t *testing.T) {
	post := &RazorpaySubscription{Status: "active", CancelAtCycleEnd: false, CancelAt: 0}
	ok, reason := keepPlanVerified(post, nil, ErrNoPendingScheduledChange)
	if !ok {
		t.Fatalf("expected keep plan to verify, got reason: %s", reason)
	}
}

func TestKeepPlanVerifiedFailsWhenProviderStillHasScheduledChanges(t *testing.T) {
	post := &RazorpaySubscription{Status: "active"}
	scheduled := &RazorpaySubscription{Status: "active", HasScheduledChanges: true}
	ok, reason := keepPlanVerified(post, scheduled, nil)
	if ok {
		t.Fatalf("expected keep plan verification to fail when scheduled changes are present")
	}
	if reason == "" {
		t.Fatalf("expected failure reason when scheduled changes remain")
	}
}

func TestKeepPlanVerifiedFailsWithTerminalState(t *testing.T) {
	post := &RazorpaySubscription{Status: "cancelled", EndedAt: 1234}
	ok, reason := keepPlanVerified(post, nil, ErrNoPendingScheduledChange)
	if ok {
		t.Fatalf("expected keep plan verification to fail for terminal state")
	}
	if reason == "" {
		t.Fatalf("expected failure reason for terminal state")
	}
}

func TestVerifyKeepPlanWithRetrySucceedsAfterNoPendingSignal(t *testing.T) {
	attempt := 0
	post, reason, err := verifyKeepPlanWithRetry(context.Background(), 4, func() (*RazorpaySubscription, error) {
		attempt++
		return &RazorpaySubscription{ID: "sub_keep", Status: "active"}, nil
	}, func() (*RazorpaySubscription, error) {
		if attempt < 3 {
			return &RazorpaySubscription{ID: "sub_keep", HasScheduledChanges: true}, nil
		}
		return nil, ErrNoPendingScheduledChange
	}, func(time.Duration) {})

	if err != nil {
		t.Fatalf("expected no fetch error, got: %v", err)
	}
	if reason != "" {
		t.Fatalf("expected empty failure reason, got: %s", reason)
	}
	if post == nil || post.ID != "sub_keep" {
		t.Fatalf("expected verified post keep-plan subscription")
	}
	if attempt != 3 {
		t.Fatalf("expected 3 attempts before verification, got %d", attempt)
	}
}

func TestVerifyKeepPlanWithRetryReturnsReasonWhenStillScheduled(t *testing.T) {
	post, reason, err := verifyKeepPlanWithRetry(context.Background(), 2, func() (*RazorpaySubscription, error) {
		return &RazorpaySubscription{ID: "sub_keep", Status: "active"}, nil
	}, func() (*RazorpaySubscription, error) {
		return &RazorpaySubscription{ID: "sub_keep", HasScheduledChanges: true}, nil
	}, func(time.Duration) {})

	if err != nil {
		t.Fatalf("expected no fetch error, got: %v", err)
	}
	if post != nil {
		t.Fatalf("expected nil post keep-plan subscription when verification exhausts")
	}
	if reason == "" {
		t.Fatalf("expected non-empty verification failure reason")
	}
}

func TestVerifyKeepPlanWithRetryReturnsFetchErrorAfterExhaustion(t *testing.T) {
	wantErr := errors.New("temporary retrieve_scheduled_changes error")
	post, reason, err := verifyKeepPlanWithRetry(context.Background(), 2, func() (*RazorpaySubscription, error) {
		return &RazorpaySubscription{ID: "sub_keep", Status: "active"}, nil
	}, func() (*RazorpaySubscription, error) {
		return nil, wantErr
	}, func(time.Duration) {})

	if post != nil {
		t.Fatalf("expected nil post keep-plan subscription when fetch fails")
	}
	if reason != "" {
		t.Fatalf("expected empty reason when fetch error is returned, got: %s", reason)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected fetch error %v, got %v", wantErr, err)
	}
}
