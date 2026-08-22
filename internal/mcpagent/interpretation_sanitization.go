package mcpagent

import "strings"

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
