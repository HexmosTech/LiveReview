package vlrender

import (
	"errors"
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

	if !SpecIsTrivial([]byte(single)) {
		t.Errorf("single-value spec should be trivial")
	}
	if SpecIsTrivial([]byte(multi)) {
		t.Errorf("multi-value spec should NOT be trivial")
	}
}

func TestTrivialDescription(t *testing.T) {
	desc, query, ok := TrivialDescription(multi(values(`{"a":"x","b":5}`)))
	if !ok {
		t.Fatalf("expected trivial single-value wrapped report")
	}
	if desc != "D" {
		t.Errorf("expected description 'D', got %q", desc)
	}
	if query != "review completions in June" {
		t.Errorf("expected query preserved, got %q", query)
	}

	if _, _, ok := TrivialDescription(multi(values(`{"a":"x","b":5}`, `{"a":"y","b":9}`))); ok {
		t.Errorf("multi-value report should not be trivial")
	}
}

func TestRenderReportsTrivialSentinel(t *testing.T) {
	// All reports trivial => ErrTrivialSpec.
	_, err := renderReports(t.Context(), []VegaLiteReport{
		{Title: "a", Spec: []byte(values(`{"a":"x","b":5}`))},
		{Title: "b", Spec: []byte(values(`{"a":"y","b":7}`))},
	}, "1.0")
	if !errors.Is(err, ErrTrivialSpec) {
		t.Errorf("all-trivial reports should return ErrTrivialSpec, got %v", err)
	}
}
