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
