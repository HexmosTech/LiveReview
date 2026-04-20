package payment

import (
	"testing"
	"time"
)

func TestComputeTrialWindowUsesProvidedDays(t *testing.T) {
	now := time.Date(2026, time.April, 20, 12, 0, 0, 0, time.UTC)
	startAtUnix, trialStartUnix, trialEndUnix := computeTrialWindow(now, 10)

	if startAtUnix != trialEndUnix {
		t.Fatalf("expected startAtUnix (%d) to equal trialEndUnix (%d)", startAtUnix, trialEndUnix)
	}
	if trialStartUnix != now.Unix() {
		t.Fatalf("trialStartUnix = %d, want %d", trialStartUnix, now.Unix())
	}
	wantEnd := now.AddDate(0, 0, 10).Unix()
	if trialEndUnix != wantEnd {
		t.Fatalf("trialEndUnix = %d, want %d", trialEndUnix, wantEnd)
	}
}

func TestComputeTrialWindowDefaultsDays(t *testing.T) {
	now := time.Date(2026, time.April, 20, 12, 0, 0, 0, time.UTC)
	_, _, trialEndUnix := computeTrialWindow(now, 0)
	wantEnd := now.AddDate(0, 0, firstPurchaseTrialDays).Unix()
	if trialEndUnix != wantEnd {
		t.Fatalf("trialEndUnix = %d, want %d", trialEndUnix, wantEnd)
	}
}
