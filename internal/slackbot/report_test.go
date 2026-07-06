package slackbot

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

func TestRenderVegaLiteReport(t *testing.T) {
	if _, err := exec.LookPath("vl-convert"); err != nil {
		t.Skip("vl-convert not installed")
	}
	wrapped := `{
  "title": "Monthly Review Volume",
  "spec": {
    "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
    "width": 600,
    "height": 300,
    "data": {
      "values": [
        {"month": "Jan", "reviews": 12},
        {"month": "Feb", "reviews": 19},
        {"month": "Mar", "reviews": 27}
      ]
    },
    "mark": "bar",
    "encoding": {
      "x": {"field": "month", "type": "ordinal"},
      "y": {"field": "reviews", "type": "quantitative"},
      "color": {"value": "#2563EB"}
    }
  }
}`

	reports, err := renderVegaLiteReports(context.Background(), wrapped)
	if err != nil {
		t.Fatalf("render failed: %s", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if len(reports[0].PNGData) < 1000 {
		t.Fatalf("png too small: %d bytes", len(reports[0].PNGData))
	}
	if reports[0].Title == "" {
		t.Fatalf("expected non-empty title")
	}
	_ = os.WriteFile("/tmp/test-vega-lite.png", reports[0].PNGData, 0644)
}

func TestRenderMultiVegaLiteReports(t *testing.T) {
	if _, err := exec.LookPath("vl-convert"); err != nil {
		t.Skip("vl-convert not installed")
	}
	multi := `{
  "reports": [
    {
      "title": "Reviews by User",
      "description": "*Top reviewers* by count.",
      "spec": {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "width": 600, "height": 300,
        "data": { "values": [{"user": "Alice", "count": 12}, {"user": "Bob", "count": 8}] },
        "mark": "bar",
        "encoding": {
          "x": {"field": "user", "type": "ordinal"},
          "y": {"field": "count", "type": "quantitative"}
        }
      }
    },
    {
      "title": "Reviews by Month",
      "description": "*Monthly trend* of reviews.",
      "spec": {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "width": 600, "height": 300,
        "data": { "values": [{"month": "Jan", "count": 5}, {"month": "Feb", "count": 9}] },
        "mark": "line",
        "encoding": {
          "x": {"field": "month", "type": "ordinal"},
          "y": {"field": "count", "type": "quantitative"}
        }
      }
    }
  ]
}`

	reports, err := renderVegaLiteReports(context.Background(), multi)
	if err != nil {
		t.Fatalf("render failed: %s", err)
	}
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}
	for i, r := range reports {
		if len(r.PNGData) < 1000 {
			t.Fatalf("report %d: png too small: %d bytes", i, len(r.PNGData))
		}
		if r.Title == "" {
			t.Fatalf("report %d: expected non-empty title", i)
		}
	}
}
