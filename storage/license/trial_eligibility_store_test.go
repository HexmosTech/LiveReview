package license

import "testing"

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
