package payment

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/livereview/internal/license"
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

func TestPaidPlansRemainTrialEnabled(t *testing.T) {
	paidPlans := []license.PlanType{
		license.PlanTeam32USD,
		license.PlanLOC200K,
		license.PlanLOC400K,
		license.PlanLOC800K,
		license.PlanLOC1600K,
		license.PlanLOC3200K,
	}

	for _, plan := range paidPlans {
		plan := plan
		t.Run(plan.String(), func(t *testing.T) {
			limits := plan.GetLimits()
			if limits.MonthlyPriceUSD <= 0 {
				t.Fatalf("plan %s monthly price must be > 0", plan)
			}
			if limits.TrialDays != firstPurchaseTrialDays {
				t.Fatalf("plan %s trial days = %d, want %d", plan, limits.TrialDays, firstPurchaseTrialDays)
			}
		})
	}
}

func TestExtractTrialConfirmationDetailsFromNotes(t *testing.T) {
	now := time.Date(2026, time.April, 20, 12, 0, 0, 0, time.UTC)

	notes, err := json.Marshal(map[string]string{
		"trial_applied":           "true",
		"trial_email":             "trial@example.com",
		"trial_reservation_token": "abc123",
		"trial_window_start_unix": "1713614400",
		"trial_window_end_unix":   "1714219200",
	})
	if err != nil {
		t.Fatalf("marshal notes: %v", err)
	}

	sub := &RazorpaySubscription{Notes: notes}
	applied, email, token, trialStart, trialEnd := extractTrialConfirmationDetails(sub, now)

	if !applied {
		t.Fatalf("expected trial to be applied")
	}
	if email != "trial@example.com" {
		t.Fatalf("email = %q, want trial@example.com", email)
	}
	if token != "abc123" {
		t.Fatalf("token = %q, want abc123", token)
	}
	if trialStart.Unix() != 1713614400 {
		t.Fatalf("trialStart = %d, want %d", trialStart.Unix(), 1713614400)
	}
	if trialEnd.Unix() != 1714219200 {
		t.Fatalf("trialEnd = %d, want %d", trialEnd.Unix(), 1714219200)
	}
}

func TestExtractTrialConfirmationDetailsFallsBackToStartAt(t *testing.T) {
	now := time.Date(2026, time.April, 20, 12, 0, 0, 0, time.UTC)
	futureStart := now.AddDate(0, 0, 7)

	notes, err := json.Marshal(map[string]string{
		"trial_applied": "true",
	})
	if err != nil {
		t.Fatalf("marshal notes: %v", err)
	}

	sub := &RazorpaySubscription{Notes: notes, StartAt: futureStart.Unix()}
	applied, _, _, trialStart, trialEnd := extractTrialConfirmationDetails(sub, now)

	if !applied {
		t.Fatalf("expected trial to be applied")
	}
	if trialStart.Unix() != now.Unix() {
		t.Fatalf("trialStart = %d, want %d", trialStart.Unix(), now.Unix())
	}
	if trialEnd.Unix() != futureStart.Unix() {
		t.Fatalf("trialEnd = %d, want %d", trialEnd.Unix(), futureStart.Unix())
	}
}

func TestExtractTrialConfirmationDetailsWithoutTrial(t *testing.T) {
	now := time.Date(2026, time.April, 20, 12, 0, 0, 0, time.UTC)
	notes, err := json.Marshal(map[string]string{"trial_applied": "false"})
	if err != nil {
		t.Fatalf("marshal notes: %v", err)
	}

	sub := &RazorpaySubscription{Notes: notes}
	applied, email, token, trialStart, trialEnd := extractTrialConfirmationDetails(sub, now)

	if applied {
		t.Fatalf("expected trial not to be applied")
	}
	if email != "" || token != "" {
		t.Fatalf("expected empty email/token, got %q/%q", email, token)
	}
	if !trialStart.IsZero() || !trialEnd.IsZero() {
		t.Fatalf("expected zero trial window, got %v - %v", trialStart, trialEnd)
	}
}
