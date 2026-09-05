package vlrender

import (
	"testing"
)

func multi(specs ...string) string {
	s := `{"reports":[`
	for i, sp := range specs {
		if i > 0 {
			s += ","
		}
		s += `{"title":"r","description":"D","query":"review completions in June","spec":` + sp + `}`
	}
	return s + `]}`
}

func values(rows ...string) string {
	s := `{"$schema":"https://vega.github.io/schema/vega-lite/v5.json","mark":"bar","data":{"values":[`
	s += joinRows(rows)
	return s + `]},"encoding":{"x":{"field":"a","type":"ordinal"},"y":{"field":"b","type":"quantitative"}}}`
}

func joinRows(rows []string) string {
	if len(rows) == 0 {
		return ""
	}
	out := rows[0]
	for _, r := range rows[1:] {
		out += "," + r
	}
	return out
}

func TestSpecIsTrivial(t *testing.T) {
	single := values(`{"a":"x","b":5}`)
	multi := values(`{"a":"x","b":5}`, `{"a":"y","b":9}`)

	if SpecIsTrivial([]byte(single)) {
		t.Errorf("single-value spec should NOT be trivial")
	}
	if SpecIsTrivial([]byte(multi)) {
		t.Errorf("multi-value spec should NOT be trivial")
	}
}

func TestSpecIsTrivialUnknownSource(t *testing.T) {
	// Data sourced from a URL/source (not inline values) — the row count is
	// unknown, so the spec must never be treated as trivial.
	urlData := `{"$schema":"https://vega.github.io/schema/vega-lite/v5.json","mark":"bar","data":{"url":"/data.json"},"encoding":{"x":{"field":"a","type":"ordinal"},"y":{"field":"b","type":"quantitative"}}}`
	sourceData := `{"$schema":"https://vega.github.io/schema/vega-lite/v5.json","mark":"bar","data":{"source":"raw"},"encoding":{"x":{"field":"a","type":"ordinal"},"y":{"field":"b","type":"quantitative"}}}`

	if SpecIsTrivial([]byte(urlData)) {
		t.Errorf("spec with data.url must NOT be trivial (count unknown)")
	}
	if SpecIsTrivial([]byte(sourceData)) {
		t.Errorf("spec with data.source must NOT be trivial (count unknown)")
	}
}

func TestTrivialDescription(t *testing.T) {
	// Single-value spec is no longer trivial — only empty data (0 rows) is.
	if _, _, _, _, _, ok := TrivialDescription(multi(values(`{"a":"x","b":5}`))); ok {
		t.Errorf("single-value report should NOT be trivial")
	}

	if _, _, _, _, _, ok := TrivialDescription(multi(values(`{"a":"x","b":5}`, `{"a":"y","b":9}`))); ok {
		t.Errorf("multi-value report should not be trivial")
	}
}

func TestRenderReportsTrivialSentinel(t *testing.T) {
	// Empty-data specs (0 rows) should be skipped and not rendered.
	empty := values()
	_, err := renderReports(t.Context(), []VegaLiteReport{
		{Title: "a", Spec: []byte(empty)},
		{Title: "b", Spec: []byte(empty)},
	}, "1.0")
	if err == nil {
		t.Errorf("all-empty-data reports should return an error")
	}
}
