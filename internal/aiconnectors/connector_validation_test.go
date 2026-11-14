package aiconnectors

import (
	"strings"
	"testing"
)

func TestSanitizeProviderErrorMessage(t *testing.T) {
	apiKey := "AQ.Ab8SECRETKEYVALUE1234"
	raw := "googleapi: Error 402: Your prepayment credits are depleted. key=" + apiKey + " "

	got := sanitizeProviderErrorMessage(raw, apiKey)

	if strings.Contains(got, apiKey) {
		t.Fatalf("sanitized message still contains the raw API key: %q", got)
	}
	if !strings.Contains(got, maskAPIKey(apiKey)) {
		t.Fatalf("sanitized message does not contain the masked key %q: %q", maskAPIKey(apiKey), got)
	}
	if !strings.Contains(got, "Your prepayment credits are depleted") {
		t.Fatalf("sanitized message dropped the provider detail: %q", got)
	}
	if strings.HasSuffix(got, " ") {
		t.Fatalf("sanitized message was not trimmed: %q", got)
	}
}

func TestSanitizeProviderErrorMessageTruncates(t *testing.T) {
	got := sanitizeProviderErrorMessage(strings.Repeat("x", 5000), "irrelevant-key")
	if len(got) != 1000 {
		t.Fatalf("expected sanitized message to be truncated to 1000 chars, got %d", len(got))
	}
}

func TestAPIKeyValidationErrorMessage(t *testing.T) {
	err := &APIKeyValidationError{Message: "provider rejected the key"}
	if err.Error() != "provider rejected the key" {
		t.Fatalf("unexpected error message: %q", err.Error())
	}
}
