package payment

import "testing"

func TestBuildUpdateSubscriptionRequestNow(t *testing.T) {
	req := buildUpdateSubscriptionRequest(4, scheduleChangeAtNowSentinel)

	if got, ok := req["schedule_change_at"].(string); !ok || got != "now" {
		t.Fatalf("expected schedule_change_at='now', got %#v", req["schedule_change_at"])
	}
}

func TestBuildUpdateSubscriptionRequestCycleEnd(t *testing.T) {
	const cycleEnd = int64(1777573800)
	req := buildUpdateSubscriptionRequest(2, cycleEnd)

	if got, ok := req["schedule_change_at"].(string); !ok || got != "cycle_end" {
		t.Fatalf("expected schedule_change_at='cycle_end', got %#v", req["schedule_change_at"])
	}
}

func TestBuildUpdateSubscriptionRequestOmitScheduleAt(t *testing.T) {
	req := buildUpdateSubscriptionRequest(1, 0)

	if got, ok := req["schedule_change_at"].(string); !ok || got != "cycle_end" {
		t.Fatalf("expected schedule_change_at='cycle_end', got %#v", req["schedule_change_at"])
	}
}

func TestBuildCreateSubscriptionRequestWithoutStartAt(t *testing.T) {
	req := buildCreateSubscriptionRequest("plan_test", 2, map[string]string{"k": "v"}, 0)

	if _, ok := req["start_at"]; ok {
		t.Fatalf("did not expect start_at in request when startAt is zero")
	}
}

func TestBuildCreateSubscriptionRequestWithStartAt(t *testing.T) {
	const startAt = int64(1777573800)
	req := buildCreateSubscriptionRequest("plan_test", 2, map[string]string{"k": "v"}, startAt)

	if got, ok := req["start_at"].(int64); !ok || got != startAt {
		t.Fatalf("expected start_at=%d, got %#v", startAt, req["start_at"])
	}
}
