package mcpagent

import (
	"strings"
	"testing"
)

// TestLawbookCompiles asserts the embedded lawbook has no error-severity
// diagnostics, so a structural break is caught at `go test` time rather
// than at the first chat turn (PLAN.md step 7).
func TestLawbookCompiles(t *testing.T) {
	book, err := ensureLawbook()
	if err != nil {
		t.Fatalf("lawbook failed to compile: %v", err)
	}
	for _, d := range book.Diagnostics() {
		if d.Severity == "error" {
			t.Errorf("lawbook diagnostic [%s] %s: %s", d.Severity, d.Code, d.Message)
		}
	}
}

// TestLawbookCarriesMachineContracts guards a failure that shipped once:
// the lawbook described the shapes in prose ("a data question") while the
// parser expects literal tokens ({"shape":"count_query"}), so every turn
// degraded to chat and no chart was ever produced. Commentary is never
// sent to the model, so any contract a parser depends on must live in the
// laws themselves.
func TestLawbookCarriesMachineContracts(t *testing.T) {
	p, err := buildLawbookPrompts("test-org", "tester", 42)
	if err != nil {
		t.Fatalf("build lawbook prompts: %v", err)
	}
	for _, c := range []struct {
		name string
		text string
		want []string
	}{
		{"classify", p.classify, []string{`{"shape"`, "count_query", "`action`", "`chat`"}},
		{"plan", p.planTail, []string{"analytics_plan", "count_sql", "`id`"}},
		{"finalize", p.finalizeTail, []string{"response_type", "data_sql", "encoding", "csv"}},
	} {
		for _, w := range c.want {
			if !strings.Contains(c.text, w) {
				t.Errorf("%s branch is missing the contract token %q", c.name, w)
			}
		}
	}
}
