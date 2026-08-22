package mcpagent

import (
	"encoding/json"
	"strings"
	"time"
)

// This file holds deterministic, Go-side fixups applied to a query result
// set before it reaches a chart spec or CSV export - the kind of thing that
// used to be asked of the model via a prompt rule and turned out unreliable
// in practice (a query that kept a raw column alias slipped the raw value
// straight through). Anything in this family - relabeling an enum,
// reformatting a value the model shouldn't have to get right by prompt
// alone - belongs here rather than as a new lawbook rule.

// triggerTypeLabels maps reviews.trigger_type's raw enum values (see
// livi.general.data law 20) to the label a reader should actually see on an
// axis, legend, or tooltip.
var triggerTypeLabels = map[string]string{
	"webhook":   "Pull Request",
	"cli_diff":  "Pre Commit",
	"mcp":       "Bots",
	"manual":    "Manual",
	"scheduled": "Scheduled",
}

// relabelTriggerTypeValues rewrites any trigger_type column's raw enum
// values to their human-readable labels, in place, independent of what the
// model's SQL actually aliased the column as.
func relabelTriggerTypeValues(rows []map[string]any) {
	for _, row := range rows {
		for col, val := range row {
			if !strings.Contains(strings.ToLower(col), "trigger") {
				continue
			}
			s, ok := val.(string)
			if !ok {
				continue
			}
			if label, known := triggerTypeLabels[s]; known {
				row[col] = label
			}
		}
	}
}

// githubGreens is GitHub's own dark-mode contribution-graph color ramp,
// darkest (no activity) to brightest (highest activity).
var githubGreens = []any{"#161b22", "#0e4429", "#006d32", "#26a641", "#39d353"}

// sanitizeChartSpec applies deterministic Go-side restyling to a fully-built
// Vega-Lite spec, independent of which pipeline or prompt produced it.
// Currently only restyles a calendar-heatmap-shaped spec; more spec-shape
// fixups belong here as they come up. Returns specJSON unchanged if nothing
// applies.
func sanitizeChartSpec(specJSON []byte) []byte {
	var spec map[string]any
	if err := json.Unmarshal(specJSON, &spec); err != nil {
		return specJSON
	}
	if !sanitizeCalendarHeatmap(spec) {
		return specJSON
	}
	out, err := json.Marshal(spec)
	if err != nil {
		return specJSON
	}
	return out
}

type heatmapChannel struct {
	Field    string
	Type     string
	TimeUnit string
}

func parseHeatmapChannel(v any) heatmapChannel {
	m, ok := v.(map[string]any)
	if !ok {
		return heatmapChannel{}
	}
	c := heatmapChannel{}
	if f, ok := m["field"].(string); ok {
		c.Field = f
	}
	if t, ok := m["type"].(string); ok {
		c.Type = t
	}
	if tu, ok := m["timeUnit"].(string); ok {
		c.TimeUnit = tu
	}
	return c
}

// sanitizeCalendarHeatmap detects a calendar-heatmap-shaped chart - one axis
// bucketed by week, the other by day-of-week, both off the same underlying
// date field, with a quantitative color - and restyles it in place to match
// GitHub's own contribution-graph look: a dark card, discrete green bands
// instead of a continuous gradient, cell gaps, month-only x labels, and
// Mon/Wed/Fri-only y labels (see scripts/adoption_chart/generate_heatmap.py,
// which renders this exact chart correctly with these exact settings). A
// model-authored spec following the chart_types.json "calendar_heatmap"
// template lands in this shape but only had Vega-Lite's plain default
// gradient - this is what actually gives it GitHub's look, deterministically,
// regardless of which pipeline or prompt produced the raw shape. Returns
// false (spec left untouched) if the shape doesn't match.
func sanitizeCalendarHeatmap(spec map[string]any) bool {
	if _, ok := spec["layer"]; ok {
		return false
	}
	if _, ok := spec["facet"]; ok {
		return false
	}
	enc, ok := spec["encoding"].(map[string]any)
	if !ok {
		return false
	}
	x := parseHeatmapChannel(enc["x"])
	y := parseHeatmapChannel(enc["y"])
	color := parseHeatmapChannel(enc["color"])
	if x.Field == "" || y.Field == "" || color.Field == "" {
		return false
	}
	if color.Type != "quantitative" || x.Field != y.Field {
		return false
	}
	isWeek := func(c heatmapChannel) bool { return strings.Contains(strings.ToLower(c.TimeUnit), "week") }
	isDay := func(c heatmapChannel) bool { return c.TimeUnit == "day" }
	if !((isWeek(x) && isDay(y)) || (isWeek(y) && isDay(x))) {
		return false
	}
	dateField := x.Field // == y.Field, already checked above
	colorField := color.Field

	// A grid whose columns only span whatever window the query's WHERE
	// clause happened to cover comes out non-square (few week-columns
	// stretched across a fixed width) and never shows an actually-empty
	// day as an empty cell, since a day the query never returned a row for
	// is simply missing from the grid rather than present at zero. GitHub's
	// own graph is always a full trailing year with every day accounted
	// for, so rebuild data.values the same way maybeCalendarHeatmap does
	// for the other pipeline: read whatever dated values the query
	// returned, keyed by calendar date, and re-emit exactly
	// calendarHeatmapDays consecutive days ending today, zero-filling
	// anything the query's rows didn't cover.
	byDate := map[string]float64{}
	if data, ok := spec["data"].(map[string]any); ok {
		if values, ok := data["values"].([]any); ok {
			for _, raw := range values {
				row, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				dateStr, val, ok := parseCalendarRow(row, dateField, colorField)
				if !ok {
					continue
				}
				byDate[dateStr] += val
			}
		}
	}
	today := time.Now()
	filled := make([]map[string]any, 0, calendarHeatmapDays+1)
	for i := calendarHeatmapDays; i >= 0; i-- {
		d := today.AddDate(0, 0, -i).Format("2006-01-02")
		filled = append(filled, map[string]any{dateField: d, colorField: byDate[d]})
	}
	spec["data"] = map[string]any{"values": filled}

	spec["background"] = "#0d1117"
	spec["config"] = map[string]any{
		"axis":   map[string]any{"labelColor": "#8b949e", "titleColor": "#e6ebf5", "labelFontSize": 10},
		"title":  map[string]any{"color": "#e6ebf5", "fontSize": 16, "anchor": "start"},
		"legend": map[string]any{"labelColor": "#8b949e", "titleColor": "#e6ebf5"},
		"view":   map[string]any{"stroke": "transparent"},
	}
	// Plain numbers, not {"step": N} - the chat frontend
	// (ui/src/pages/Chatbot/InteractiveChart.tsx) only derives an aspect
	// ratio to resize by when width/height are plain numbers.
	spec["width"] = 900
	spec["height"] = 130
	spec["mark"] = map[string]any{"type": "rect", "cornerRadius": 2}
	spec["encoding"] = map[string]any{
		"x": map[string]any{
			"field": dateField, "type": "ordinal", "timeUnit": "yearweek", "title": nil,
			"scale": map[string]any{"paddingInner": 0.15},
			"axis": map[string]any{
				"format":     "%b",
				"labelExpr":  "date(datum.value) <= 7 ? timeFormat(datum.value, '%b') : ''",
				"labelAngle": 0, "ticks": false, "domain": false, "grid": false,
			},
		},
		"y": map[string]any{
			"field": dateField, "type": "ordinal", "timeUnit": "day", "title": nil,
			"sort":  []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
			"scale": map[string]any{"paddingInner": 0.15},
			"axis":  map[string]any{"values": []string{"Mon", "Wed", "Fri"}, "ticks": false, "domain": false, "grid": false},
		},
		"color": map[string]any{
			"field": colorField, "type": "quantitative", "title": nil,
			"scale":  map[string]any{"type": "threshold", "domain": []int{1, 3, 6, 10}, "range": githubGreens},
			"legend": nil,
		},
		"tooltip": []any{
			map[string]any{"field": dateField, "type": "temporal", "title": "Date", "format": "%A, %b %d, %Y"},
			map[string]any{"field": colorField, "type": "quantitative"},
		},
	}
	return true
}
