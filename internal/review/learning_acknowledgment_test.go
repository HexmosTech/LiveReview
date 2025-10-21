package review

import (
	"strings"
	"testing"
)

func TestAppendLearningAcknowledgment_NoBlock(t *testing.T) {
	input := "Plain response without metadata."
	result := appendLearningAcknowledgment(input)

	if result != input {
		t.Fatalf("expected original content to be unchanged, got %q", result)
	}
}

func TestAppendLearningAcknowledgment_WithLearning(t *testing.T) {
	input := "Thanks for the clarification!\n```learning\n{\n  \"type\": \"team_policy\",\n  \"title\": \"Error Handling\",\n  \"content\": \"We always wrap errors with context.\",\n  \"tags\": [\"errors\", \"policy\"],\n  \"scope\": \"repo\",\n  \"confidence\": 4\n}\n```"

	result := appendLearningAcknowledgment(input)

	if strings.Contains(result, "```learning") {
		t.Fatalf("expected learning block to be removed, got %q", result)
	}

	if !strings.Contains(result, "💡 *Learning captured:") {
		t.Fatalf("expected acknowledgment marker in result, got %q", result)
	}

	if !strings.Contains(result, "Error Handling") {
		t.Fatalf("expected learning title to be surfaced, got %q", result)
	}
}

func TestAppendLearningAcknowledgment_InvalidJSON(t *testing.T) {
	input := "Here is the response.\n```learning\n{not-json}\n```"
	result := appendLearningAcknowledgment(input)

	if strings.Contains(result, "```learning") {
		t.Fatalf("expected invalid block to be removed, got %q", result)
	}

	if strings.Contains(result, "💡 *Learning captured:") {
		t.Fatalf("did not expect acknowledgment when JSON invalid, got %q", result)
	}
}
