// Package onboardingreport provides the embedded template catalog for the
// Onboarding Report — a 1-click, LLM-free report that any org can generate
// to see their LiveReview adoption metrics after using the product.
package onboardingreport

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed templates.json
var templatesJSON []byte

// ChartTemplate is one chart definition from the template catalog.
// SQL contains {{.OrgID}} which must be replaced before execution.
// VegaSpec contains "DATA_PLACEHOLDER" which must be replaced with
// the actual query result rows.
type ChartTemplate struct {
	ID          string          `json:"id"`
	Section     string          `json:"section"`
	SectionLabel string         `json:"section_label"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	QuerySummary string         `json:"query_summary"`
	SQL         string          `json:"sql"`
	VegaSpec    json.RawMessage `json:"vega_spec"`
	ChartType   string          `json:"chart_type"`
	Granularity string          `json:"granularity"`
	TimeRange   string          `json:"time_range"`
	SortOrder   int             `json:"sort_order"`
}

// Section is a report section heading.
type Section struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// TemplateCatalog is the full embedded template catalog.
type TemplateCatalog struct {
	Version      string          `json:"version"`
	TotalCharts  int             `json:"total_charts"`
	Sections     []Section       `json:"sections"`
	Charts       []ChartTemplate `json:"charts"`
}

var catalog TemplateCatalog

func init() {
	if err := json.Unmarshal(templatesJSON, &catalog); err != nil {
		panic(fmt.Sprintf("onboardingreport: failed to parse templates.json: %v", err))
	}
}

// Catalog returns the parsed template catalog.
func Catalog() TemplateCatalog {
	return catalog
}

// Sections returns the ordered list of report sections.
func Sections() []Section {
	return catalog.Sections
}

// Charts returns all chart templates.
func Charts() []ChartTemplate {
	return catalog.Charts
}

// ChartsBySection returns chart templates grouped by section, in order.
func ChartsBySection() map[string][]ChartTemplate {
	out := make(map[string][]ChartTemplate, len(catalog.Sections))
	for _, ch := range catalog.Charts {
		out[ch.Section] = append(out[ch.Section], ch)
	}
	return out
}

// PrepareSQL replaces {{.OrgID}} in the template SQL with the actual org ID.
func (t ChartTemplate) PrepareSQL(orgID int64) string {
	return strings.ReplaceAll(t.SQL, "{{.OrgID}}", fmt.Sprintf("%d", orgID))
}

// vegaLiteConfig is a consistent theme injected into every chart spec at
// render time. It controls font sizes, line widths, point sizes, legend
// layout, and background so charts render identically regardless of the
// original spec's defaults. The values are tuned for static PNG export at
// 2x scale embedded in A4 PDF.
//
// Key design decisions:
//   - view.continuousWidth/Height set the DATA area size (excluding axes/legend).
//     At 2x scale these become the PNG pixel dimensions. We use 540×300 (1.8:1)
//     which fits well on A4 portrait (180mm usable width) and keeps charts from
//     being too tall for bar charts with many categories.
//   - axis.domain=true draws the X/Y axis lines. axis.grid draws reference lines.
//   - legend.labelColor is explicitly dark so legend text is always visible.
var vegaLiteConfig = map[string]interface{}{
	"background": "white",
	"font":       "sans-serif",
	"title": map[string]interface{}{
		"fontSize":   12,
		"fontWeight": "bold",
		"anchor":     "start",
		"offset":     6,
		"color":      "#1e293b",
	},
	"axis": map[string]interface{}{
		"labelFontSize":  9,
		"titleFontSize":  10,
		"domain":         true,
		"domainColor":    "#64748b",
		"domainWidth":    1,
		"grid":           true,
		"gridColor":      "#e2e8f0",
		"gridOpacity":    0.7,
		"gridWidth":      0.5,
		"tickColor":      "#64748b",
		"tickSize":       4,
		"tickWidth":      0.8,
		"labelColor":     "#475569",
		"titleColor":     "#334155",
		"labelPadding":   4,
		"titlePadding":   8,
	},
	"legend": map[string]interface{}{
		"labelFontSize":  10,
		"titleFontSize":  10,
		"labelColor":     "#1e293b",
		"titleColor":     "#334155",
		"symbolSize":     80,
		"symbolStrokeWidth": 1.5,
		"padding":        8,
		"rowPadding":     3,
		"columnPadding":  10,
		"labelLimit":     200,
		"titleLimit":     200,
	},
	"line": map[string]interface{}{
		"strokeWidth": 1.8,
	},
	"point": map[string]interface{}{
		"size":        25,
		"strokeWidth": 1.2,
		"filled":      true,
	},
	"bar": map[string]interface{}{
		"cornerRadiusTopLeft":  2,
		"cornerRadiusTopRight": 2,
		"continuousBandSize":   16,
	},
	"range": map[string]interface{}{
		"category": []string{
			"#2563eb", "#dc2626", "#16a34a", "#ea580c",
			"#7c3aed", "#0891b2", "#ca8a04", "#be185d",
			"#4f46e5", "#059669",
		},
	},
	// view.continuousWidth/Height only govern a chart's size when its scale
	// is continuous. A bar chart with an ordinal/nominal x-axis (the most
	// common shape in this catalog) uses a band scale, so Vega-Lite instead
	// sizes it from discreteWidth/discreteHeight, which default to a small
	// step-based size (~20px per category). Left unset, a chart with only a
	// few categories rendered far narrower than 540px regardless of this
	// config, coming out narrow-and-tall — exactly the charts that most
	// needed to fit the page. Setting them to match keeps every chart at a
	// consistent, page-appropriate size regardless of its scale type.
	"view": map[string]interface{}{
		"stroke":           "transparent",
		"strokeWidth":      0,
		"continuousWidth":  540,
		"continuousHeight": 300,
		"discreteWidth":    540,
		"discreteHeight":   300,
	},
}

// PrepareVegaSpec replaces DATA_PLACEHOLDER in the Vega-Lite spec with
// the actual query result JSON array, and injects the standard config
// theme for consistent styling.
func (t ChartTemplate) PrepareVegaSpec(dataJSON []byte) json.RawMessage {
	placeholder := `"DATA_PLACEHOLDER"`
	replacement := string(dataJSON)
	result := strings.Replace(string(t.VegaSpec), placeholder, replacement, 1)

	// Inject config theme into the spec.
	var spec map[string]interface{}
	if err := json.Unmarshal([]byte(result), &spec); err != nil {
		// If unmarshal fails, return as-is (shouldn't happen with valid specs).
		return json.RawMessage(result)
	}
	spec["config"] = vegaLiteConfig
	out, err := json.Marshal(spec)
	if err != nil {
		return json.RawMessage(result)
	}
	return json.RawMessage(out)
}
