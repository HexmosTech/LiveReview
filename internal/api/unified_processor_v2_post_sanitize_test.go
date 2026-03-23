package api

import (
	"context"
	"strings"
	"testing"
)

func TestApplyPostOutputSanitization_RedactsSensitiveOutput(t *testing.T) {
	processor := &UnifiedProcessorV2Impl{}
	input := "Please email alice@example.com and use sk-12345678901234567890"

	out := processor.applyPostOutputSanitization(context.Background(), input, "openai")

	if strings.Contains(out, "alice@example.com") {
		t.Fatalf("expected email redaction, got: %s", out)
	}
	if strings.Contains(out, "sk-12345678901234567890") {
		t.Fatalf("expected secret redaction, got: %s", out)
	}
	if !strings.Contains(out, "REDACTED_SECRET") {
		t.Fatalf("expected redacted marker in output, got: %s", out)
	}
}
