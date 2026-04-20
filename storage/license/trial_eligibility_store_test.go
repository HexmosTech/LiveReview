package license

import (
	"context"
	"strings"
	"testing"
)

func TestNormalizeTrialEligibilityEmail(t *testing.T) {
	normalized, err := NormalizeTrialEligibilityEmail("  User+Alias@Example.COM  ")
	if err != nil {
		t.Fatalf("NormalizeTrialEligibilityEmail returned error: %v", err)
	}
	if normalized != "user+alias@example.com" {
		t.Fatalf("normalized email = %q, want %q", normalized, "user+alias@example.com")
	}
}

func TestNormalizeTrialEligibilityEmailRejectsBlank(t *testing.T) {
	if _, err := NormalizeTrialEligibilityEmail("   "); err == nil {
		t.Fatalf("expected error for blank email")
	}
}

func TestGetTrialEligibilityByEmailRejectsMissingDB(t *testing.T) {
	store := &TrialEligibilityStore{}
	_, found, err := store.GetTrialEligibilityByEmail(context.Background(), "user@example.com")
	if err == nil {
		t.Fatalf("expected error for missing db handle")
	}
	if !strings.Contains(err.Error(), "missing db handle") {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatalf("found = true, want false")
	}
}

func TestGetTrialEligibilityByEmailRejectsBlankEmail(t *testing.T) {
	store := &TrialEligibilityStore{}
	_, _, err := store.GetTrialEligibilityByEmail(context.Background(), "   ")
	if err == nil {
		t.Fatalf("expected error for blank email")
	}
}
