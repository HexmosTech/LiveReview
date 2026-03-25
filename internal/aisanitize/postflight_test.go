package aisanitize

import (
	"context"
	"strings"
	"testing"
)

func TestSanitizationPostflight_RedactsSecretAndPII(t *testing.T) {
	input := "Contact alice@example.com and use token sk-12345678901234567890"
	out, report := SanitizationPostflight(context.Background(), input)

	if strings.Contains(out, "alice@example.com") {
		t.Fatalf("expected email to be redacted, got: %s", out)
	}
	if strings.Contains(out, "sk-12345678901234567890") {
		t.Fatalf("expected secret to be redacted, got: %s", out)
	}
	if !strings.Contains(out, "REDACTED_SECRET") {
		t.Fatalf("expected redacted secret marker, got: %s", out)
	}
	if !report.Sanitized {
		t.Fatalf("expected report.Sanitized=true")
	}
}

func TestSanitizationPostflight_LeavesSafeTextUnchanged(t *testing.T) {
	input := "Looks good overall. Please consider renaming this variable for clarity."
	out, report := SanitizationPostflight(context.Background(), input)

	if out != input {
		t.Fatalf("expected safe text to remain unchanged, got: %s", out)
	}
	if report.Sanitized {
		t.Fatalf("expected report.Sanitized=false for safe text")
	}
}
