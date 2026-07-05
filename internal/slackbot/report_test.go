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

	pngData, title, err := renderVegaLiteReport(context.Background(), wrapped)
	if err != nil {
		t.Fatalf("render failed: %s", err)
	}
	if len(pngData) < 1000 {
		t.Fatalf("png too small: %d bytes", len(pngData))
	}
	if title == "" {
		t.Fatalf("expected non-empty title")
	}
	_ = os.WriteFile("/tmp/test-vega-lite.png", pngData, 0644)
}
