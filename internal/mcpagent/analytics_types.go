package mcpagent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/livereview/internal/vlrender"
)

// The analytics protocol rides on the same channel as everything else the model
// says: it replies with JSON and Go decides what it meant by looking at the
// shape. Three shapes are possible per turn, and they must not overlap:
//
//	{"tool": ...}            -> an action, handled by parseToolCalls
//	{"analytics_plan": [...]} -> this file
//	anything else             -> a plain text answer
//
// parseToolCalls already requires a "tool" field, so an analytics payload falls
// through it untouched - the same property that already lets chart JSON pass.

// PlanEntry is one sub-question the user's message implies. A single message
// like "show reviews per month and my top reviewers" produces two.
type PlanEntry struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	// CountSQL counts the rows the *answer* would contain, not the rows
	// scanned. That distinction is what makes the chart/csv decision meaningful
	// downstream, and the prompt teaches it with a worked subquery example.
	CountSQL string `json:"count_sql"`
}

type analyticsPlanEnvelope struct {
	AnalyticsPlan []PlanEntry `json:"analytics_plan"`
}

// Response types the model may choose for a report.
const (
	ResponseTypeChart  = "chart"
	ResponseTypeCSV    = "csv"
	ResponseTypeNoData = "no_data"
)

// FinalizePlan is the model's second-pass decision for one report: how to
// present it, and the SQL that produces the data. Note there is no field for
// the data itself - Go fetches it, so the model can never re-type a number.
type FinalizePlan struct {
	ResponseType string          `json:"response_type"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	Query        string          `json:"query"`
	DataSQL      string          `json:"data_sql"`
	Mark         string          `json:"mark"`
	Encoding     json.RawMessage `json:"encoding"`
	CSVFilename  string          `json:"csv_filename"`
	Text         string          `json:"text"`
}

// parseAnalyticsPlan reports whether the model asked for analytics, and if so
// what it wants counted. It is deliberately strict: returning true for a chart
// payload or a tool call would hijack a path that already works.
func parseAnalyticsPlan(text string) ([]PlanEntry, bool) {
	body := strings.TrimSpace(vlrender.ExtractJSONBlock(text))
	if body == "" {
		return nil, false
	}

	var env analyticsPlanEnvelope
	if err := json.Unmarshal([]byte(body), &env); err == nil && len(env.AnalyticsPlan) > 0 {
		return normalizePlan(env.AnalyticsPlan)
	}

	// Models routinely drop the wrapper and emit the bare array. Accept it, but
	// only when every element carries a count_sql - otherwise this would match
	// the tool-call array shape.
	if strings.HasPrefix(body, "[") {
		var entries []PlanEntry
		if err := json.Unmarshal([]byte(body), &entries); err == nil && len(entries) > 0 {
			for _, e := range entries {
				if strings.TrimSpace(e.CountSQL) == "" {
					return nil, false
				}
			}
			return normalizePlan(entries)
		}
	}

	return nil, false
}

func normalizePlan(entries []PlanEntry) ([]PlanEntry, bool) {
	out := make([]PlanEntry, 0, len(entries))
	for i, e := range entries {
		if strings.TrimSpace(e.CountSQL) == "" {
			continue
		}
		if strings.TrimSpace(e.ID) == "" {
			e.ID = fmt.Sprintf("r%d", i+1)
		}
		out = append(out, e)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// parseFinalizePlan decodes the second-pass response for one report.
func parseFinalizePlan(text string) (*FinalizePlan, error) {
	body := strings.TrimSpace(vlrender.ExtractJSONBlock(text))
	if body == "" {
		return nil, fmt.Errorf("empty finalize response")
	}
	var p FinalizePlan
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		return nil, fmt.Errorf("finalize response is not valid JSON: %w", err)
	}
	p.ResponseType = strings.ToLower(strings.TrimSpace(p.ResponseType))
	switch p.ResponseType {
	case ResponseTypeChart, ResponseTypeCSV, ResponseTypeNoData:
	case "":
		// A missing type with usable SQL is almost always a chart; defaulting
		// beats spending a retry on a formatting slip.
		if strings.TrimSpace(p.DataSQL) != "" {
			p.ResponseType = ResponseTypeChart
		} else {
			return nil, fmt.Errorf("finalize response has no response_type and no data_sql")
		}
	default:
		return nil, fmt.Errorf("unknown response_type %q", p.ResponseType)
	}
	if p.ResponseType != ResponseTypeNoData && strings.TrimSpace(p.DataSQL) == "" {
		return nil, fmt.Errorf("response_type %q requires data_sql", p.ResponseType)
	}
	return &p, nil
}

// encodingFields extracts every "field" referenced anywhere in the Vega-Lite
// encoding, at any depth - channels can be objects ({"x":{"field":...}}) or
// arrays ({"tooltip":[{"field":...}]}). These names are checked against the
// actual result columns before a chart is built, because a model that
// misremembers its own alias would otherwise produce a silently empty chart.
func (p *FinalizePlan) encodingFields() []string {
	if len(p.Encoding) == 0 {
		return nil
	}
	var doc any
	if err := json.Unmarshal(p.Encoding, &doc); err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	var walk func(any)
	walk = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			for key, child := range v {
				if key == "field" {
					if name, ok := child.(string); ok && name != "" && !seen[name] {
						seen[name] = true
						out = append(out, name)
					}
					continue
				}
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(doc)
	return out
}

var unsafeFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// safeCSVFilename turns a model-supplied name into something safe to put in a
// Content-Disposition header. Path separators and traversal are stripped rather
// than escaped, since no legitimate name needs them.
func safeCSVFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.TrimPrefix(name, ".")
	name = unsafeFilenameChars.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-.")
	if name == "" {
		name = "livereview-export"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".csv") {
		name += ".csv"
	}
	const maxLen = 80
	if len(name) > maxLen {
		name = name[:maxLen-4] + ".csv"
	}
	return name
}
