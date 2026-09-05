package api

import "testing"

func TestRunJQBool(t *testing.T) {
	doc := &canonicalReviewDoc{
		ReviewID:   1,
		OrgID:      1,
		Repository: "acme/webapp",
		Provider:   "github",
		Status:     "completed",
		Findings: []canonicalFinding{
			{Severity: "critical", Confidence: "high", Category: "security"},
			{Severity: "low", Confidence: "medium", Category: "style"},
		},
		Counts: canonicalReviewDocCounts{
			BySeverity: map[string]int{"critical": 1, "high": 0, "medium": 0, "low": 1, "info": 0},
			ByCategory: map[string]int{"security": 1, "style": 1},
			Total:      2,
		},
	}

	cases := []struct {
		name    string
		expr    string
		block   bool
		wantErr bool
	}{
		{name: "critical count blocks", expr: ".counts.by_severity.critical > 0", block: true},
		{name: "high count allows", expr: ".counts.by_severity.high > 0", block: false},
		{name: "security category blocks", expr: ".counts.by_category.security > 0", block: true},
		{name: "or expression", expr: "(.counts.by_severity.critical > 0) or (.counts.by_category.security > 0)", block: true},
		{name: "row-level select", expr: `[.findings[] | select(.severity == "critical" and .confidence == "high")] | length > 0`, block: true},
		{name: "literal false never blocks", expr: "false", block: false},
		{name: "invalid expr errors", expr: "this is not jq(", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocked, _, err := runJQBool(tc.expr, doc)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if blocked != tc.block {
				t.Fatalf("expected block=%v, got %v", tc.block, blocked)
			}
		})
	}
}
