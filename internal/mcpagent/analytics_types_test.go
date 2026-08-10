package mcpagent

import (
	"encoding/json"
	"testing"
)

// The discriminator's most important property is what it does NOT claim. If it
// matched a tool call or a chart payload it would hijack a working path.
func TestParseAnalyticsPlanIgnoresOtherShapes(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"single tool call", "```json\n{\"tool\": \"GET_api_v1_reviews\", \"arguments\": {\"per_page\": 20}}\n```"},
		{"tool call array", "```json\n[{\"tool\": \"POST_api_v1_learnings\", \"arguments\": {}}]\n```"},
		{"bare tool call", `{"tool":"GET_api_v1_auth_me","arguments":{}}`},
		{"chart payload", `{"title":"x","description":"y","spec":{"$schema":"https://vega.github.io/schema/vega-lite/v5.json","mark":"bar"}}`},
		{"multi report chart payload", `{"reports":[{"title":"a","spec":{"mark":"bar"}}]}`},
		{"plain prose", "You have 12 connectors configured."},
		{"empty", ""},
		{"array without count_sql", `[{"id":"r1","question":"hi"}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if entries, ok := parseAnalyticsPlan(tc.text); ok {
				t.Fatalf("hijacked a non-analytics response, returned %d entries", len(entries))
			}
		})
	}
}

func TestParseAnalyticsPlanAcceptsRealShapes(t *testing.T) {
	t.Run("wrapped object", func(t *testing.T) {
		entries, ok := parseAnalyticsPlan(`{"analytics_plan":[
			{"id":"r1","question":"reviews per month","count_sql":"SELECT count(*) FROM (SELECT 1 FROM reviews GROUP BY 1) t"},
			{"id":"r2","question":"top reviewers","count_sql":"SELECT count(*) FROM (SELECT author_username FROM reviews GROUP BY 1) t"}
		]}`)
		if !ok {
			t.Fatal("expected a plan")
		}
		if len(entries) != 2 || entries[0].ID != "r1" || entries[1].Question != "top reviewers" {
			t.Fatalf("unexpected entries: %+v", entries)
		}
	})

	t.Run("fenced", func(t *testing.T) {
		if _, ok := parseAnalyticsPlan("Here you go:\n```json\n{\"analytics_plan\":[{\"count_sql\":\"SELECT count(*) FROM reviews\"}]}\n```"); !ok {
			t.Fatal("expected a plan from a fenced payload")
		}
	})

	t.Run("bare array with count_sql", func(t *testing.T) {
		entries, ok := parseAnalyticsPlan(`[{"question":"q","count_sql":"SELECT count(*) FROM reviews"}]`)
		if !ok {
			t.Fatal("expected a plan")
		}
		if entries[0].ID != "r1" {
			t.Fatalf("missing id was not backfilled: %+v", entries[0])
		}
	})

	t.Run("entries missing sql are dropped", func(t *testing.T) {
		entries, ok := parseAnalyticsPlan(`{"analytics_plan":[
			{"id":"a","count_sql":"SELECT count(*) FROM reviews"},
			{"id":"b","question":"no sql here"}
		]}`)
		if !ok || len(entries) != 1 || entries[0].ID != "a" {
			t.Fatalf("unexpected entries: %+v", entries)
		}
	})
}

func TestParseFinalizePlan(t *testing.T) {
	t.Run("chart", func(t *testing.T) {
		p, err := parseFinalizePlan(`{"response_type":"chart","title":"Reviews per month",
			"description":"d","query":"q","data_sql":"SELECT 1 AS month, 2 AS n",
			"mark":"bar","encoding":{"x":{"field":"month","type":"temporal"},"y":{"field":"n","type":"quantitative"}}}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.ResponseType != ResponseTypeChart || p.Mark != "bar" {
			t.Fatalf("unexpected plan: %+v", p)
		}
		fields := p.encodingFields()
		if len(fields) != 2 {
			t.Fatalf("expected 2 encoding fields, got %v", fields)
		}
	})

	t.Run("nested and array channels are found", func(t *testing.T) {
		p := &FinalizePlan{Encoding: json.RawMessage(
			`{"x":{"field":"month"},"tooltip":[{"field":"a"},{"field":"b"}],"color":{"condition":{"field":"c"}}}`)}
		got := map[string]bool{}
		for _, f := range p.encodingFields() {
			got[f] = true
		}
		for _, want := range []string{"month", "a", "b", "c"} {
			if !got[want] {
				t.Fatalf("field %q not found in %v", want, p.encodingFields())
			}
		}
	})

	t.Run("missing type defaults to chart when sql is present", func(t *testing.T) {
		p, err := parseFinalizePlan(`{"title":"t","data_sql":"SELECT 1 AS a"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.ResponseType != ResponseTypeChart {
			t.Fatalf("expected chart default, got %q", p.ResponseType)
		}
	})

	t.Run("rejects unusable responses", func(t *testing.T) {
		for _, bad := range []string{
			``,
			`not json`,
			`{"response_type":"interpretive_dance","data_sql":"SELECT 1"}`,
			`{"response_type":"chart"}`,
			`{"title":"no sql and no type"}`,
		} {
			if _, err := parseFinalizePlan(bad); err == nil {
				t.Fatalf("accepted an unusable finalize response: %q", bad)
			}
		}
	})

	t.Run("no_data needs no sql", func(t *testing.T) {
		p, err := parseFinalizePlan(`{"response_type":"no_data","text":"No reviews were completed today."}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Text == "" {
			t.Fatal("no_data text was dropped")
		}
	})
}

func TestSafeCSVFilename(t *testing.T) {
	cases := map[string]string{
		"reviews-by-month.csv": "reviews-by-month.csv",
		"reviews by month":     "reviews-by-month.csv",
		"":                     "livereview-export.csv",
		"../../etc/passwd":     "passwd.csv",
		"/absolute/path.csv":   "path.csv",
		".hidden":              "hidden.csv",
		"weird\"name*?.csv":    "weird-name-.csv",
	}
	for in, want := range cases {
		if got := safeCSVFilename(in); got != want {
			t.Errorf("safeCSVFilename(%q) = %q, want %q", in, got, want)
		}
	}
	if got := safeCSVFilename(string(make([]byte, 200))); len(got) > 80 {
		t.Errorf("long name not truncated: %d chars", len(got))
	}
}
