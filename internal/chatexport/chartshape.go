package chatexport

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DescribeChartShape reports what a Vega-Lite spec actually renders as -
// its mark(s) - independent of whatever chart_type label the model may
// have picked (that label is a hint the finalize pipeline doesn't even
// receive, and can diverge from the encoding). Ported from the identical
// TypeScript helper that once lived in ChatDebugPage.tsx (removed only for
// display parity between /chat and /chat-debug during the Phase 1 merge,
// not because the detection was wrong) - kept as the single source of
// truth for "what kind of chart is this" across both the compile-picker
// summary and any future caption use.
//
// Returns a bare mark ("bar", "line", "point", ...), "layered (bar + line)"
// for spec.layer arrays, "faceted (bar)" for spec.facet + spec.spec.mark,
// or "unknown" if the spec doesn't parse or has no mark at all.
func DescribeChartShape(vegaSpec []byte) string {
	var spec map[string]any
	if err := json.Unmarshal(vegaSpec, &spec); err != nil {
		return "unknown"
	}

	if layer, ok := spec["layer"].([]any); ok {
		marks := make([]string, 0, len(layer))
		for _, l := range layer {
			lm, _ := l.(map[string]any)
			marks = append(marks, markTypeLabel(lm["mark"]))
		}
		return fmt.Sprintf("layered (%s)", strings.Join(marks, " + "))
	}

	if _, ok := spec["facet"]; ok {
		var innerMark any
		if inner, ok := spec["spec"].(map[string]any); ok {
			innerMark = inner["mark"]
		}
		return fmt.Sprintf("faceted (%s)", markTypeLabel(innerMark))
	}

	return markTypeLabel(spec["mark"])
}

// markTypeLabel reads a mark's "type" whether it's the bare-string form
// ("bar") or the object form ({"type": "bar", ...}) Vega-Lite also allows.
func markTypeLabel(mark any) string {
	if s, ok := mark.(string); ok {
		return s
	}
	if m, ok := mark.(map[string]any); ok {
		if s, ok := m["type"].(string); ok {
			return s
		}
	}
	return "unknown"
}
