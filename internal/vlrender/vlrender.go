// Package vlrender centralizes the Vega-Lite chart parsing and rendering logic
// shared by the Discord, Teams, Slack bots and the web chat handler. Keeping it
// in one place ensures a single source of truth for chart extraction and
// vl-convert invocation across all integration surfaces.
package vlrender

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	vlConvertDefault = "vl-convert"
	vlVersion        = "5.21"
	vlThemeDefault   = "powerbi"
)

// VegaLiteReport is a single titled chart spec as emitted by the review prompt.
type VegaLiteReport struct {
	Title       string          `json:"title"`
	Subtitle    string          `json:"subtitle,omitempty"`
	Description string          `json:"description,omitempty"`
	Query       string          `json:"query,omitempty"`
	TimeRange   string          `json:"time_range,omitempty"`
	Granularity string          `json:"granularity,omitempty"`
	Context     ChartContext    `json:"context"`
	Spec        json.RawMessage `json:"spec"`
}

// ChartContext names who or what a chart/export is scoped to. Organization
// is always the real org name - every query is org-scoped regardless of
// whether it's also narrowed to specific repositories or people.
type ChartContext struct {
	Organization string   `json:"organization"`
	Repository   []string `json:"repository,omitempty"`
	Person       []string `json:"person,omitempty"`
}

// Format renders a ChartContext as one plain-text line, for surfaces (Slack,
// Discord, Teams plain-text fallback) that show a single string rather than
// the structured object the web chat UI renders as separate bullet points.
func (c ChartContext) Format() string {
	var parts []string
	if c.Organization != "" {
		parts = append(parts, "Organization: "+c.Organization)
	}
	if len(c.Repository) > 0 {
		parts = append(parts, "Repository: "+strings.Join(c.Repository, ", "))
	}
	if len(c.Person) > 0 {
		parts = append(parts, "Person: "+strings.Join(c.Person, ", "))
	}
	return strings.Join(parts, "; ")
}

// Report is a rendered chart. PNGData holds the in-memory image; PNGPath is the
// temp directory containing report.png. Callers that only need PNGData must call
// CleanupReports to avoid leaking temp files, while callers that serve the file
// (e.g. Teams, web chat) retain PNGPath for later serving and are responsible
// for cleanup at their own lifetime boundary.
type Report struct {
	PNGData     []byte
	Title       string
	Description string
	Query       string
	TimeRange   string
	Granularity string
	PNGPath     string
}

// ErrTrivialSpec is returned when a Vega-Lite payload resolves to only a single
// value/bar, which is not worth charting. Callers should present the
// human-readable description as text instead of rendering a chart or reporting
// an error.
var ErrTrivialSpec = errors.New("chart is trivial: only a single value to show")

// SpecIsTrivial reports whether a Vega-Lite spec encodes only a single data
// point (one value, e.g. a lone bar). Rendering such a spec adds no insight
// over stating the number in text, so callers skip chart generation for it.
func SpecIsTrivial(spec []byte) bool {
	var m map[string]any
	if err := json.Unmarshal(spec, &m); err != nil {
		return false
	}
	count, known := countDataValues(m)
	// Only treat a spec as trivial when the row count is actually known and
	// there is at most one value. If the data source isn't inline `values`
	// (e.g. data.url / data.source / transforms), the count is unknown, so we
	// must NOT guess — render it as usual.
	return known && count <= 1
}

// countDataValues recursively tallies the number of rows across all `data.values`
// arrays embedded in a spec (including layered/concat compositions). known is
// false when any data source is not an inline `values` array, in which case the
// total cannot be trusted and callers should not treat the spec as trivial.
func countDataValues(m map[string]any) (total int, known bool) {
	known = true
	for _, key := range []string{"layer", "concat", "hconcat", "vconcat"} {
		if arr, ok := m[key].([]any); ok {
			for _, item := range arr {
				if child, ok := item.(map[string]any); ok {
					t, k := countDataValues(child)
					total += t
					if !k {
						known = false
					}
				}
			}
		}
	}
	if child, ok := m["spec"].(map[string]any); ok {
		t, k := countDataValues(child)
		total += t
		if !k {
			known = false
		}
	}
	if data, ok := m["data"].(map[string]any); ok {
		if values, ok := data["values"].([]any); ok {
			total += len(values)
		} else {
			// data.url, data.source, data.sequence, datasets, etc. — we cannot
			// count these, so the result is not known to be trivial.
			known = false
		}
	}
	return total, known
}

// TrivialDescription returns the human-readable description, query, time range,
// and granularity to show in place of a chart when the payload's spec(s) resolve
// to only a single value/bar, plus true if that is the case. Callers render the
// returned text instead of a chart and may append the query with their own phrasing.
func TrivialDescription(raw string) (desc string, query string, timeRange string, granularity string, context string, ok bool) {
	body := ExtractJSONBlock(raw)

	var multi struct {
		Reports []VegaLiteReport `json:"reports"`
	}
	if err := json.Unmarshal([]byte(body), &multi); err == nil && len(multi.Reports) > 0 {
		trivial := true
		var parts []string
		var queries []string
		var timeRanges []string
		var granularities []string
		var contexts []string
		for _, r := range multi.Reports {
			spec, err := NormalizeVegaLiteSpec(r.Spec)
			if err != nil {
				trivial = false
				continue
			}
			if !SpecIsTrivial(spec) {
				trivial = false
			}
			if strings.TrimSpace(r.Description) != "" {
				parts = append(parts, r.Description)
			}
			if strings.TrimSpace(r.Query) != "" {
				queries = append(queries, r.Query)
			}
			if strings.TrimSpace(r.TimeRange) != "" {
				timeRanges = append(timeRanges, r.TimeRange)
			}
			if strings.TrimSpace(r.Granularity) != "" {
				granularities = append(granularities, r.Granularity)
			}
			if formatted := r.Context.Format(); formatted != "" {
				contexts = append(contexts, formatted)
			}
		}
		if !trivial {
			return "", "", "", "", "", false
		}
		return strings.Join(parts, "\n\n"), strings.Join(queries, "\n\n"),
			strings.Join(timeRanges, "\n\n"), strings.Join(granularities, "\n\n"),
			strings.Join(contexts, "\n\n"), true
	}

	var wrapped VegaLiteReport
	if err := json.Unmarshal([]byte(body), &wrapped); err == nil && len(wrapped.Spec) > 0 {
		spec, err := NormalizeVegaLiteSpec(wrapped.Spec)
		if err != nil {
			return "", "", "", "", "", false
		}
		if !SpecIsTrivial(spec) {
			return "", "", "", "", "", false
		}
		return wrapped.Description, wrapped.Query, wrapped.TimeRange, wrapped.Granularity, wrapped.Context.Format(), true
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return "", "", "", "", "", false
	}
	spec, err := NormalizeVegaLiteSpec([]byte(body))
	if err != nil {
		return "", "", "", "", "", false
	}
	if !SpecIsTrivial(spec) {
		return "", "", "", "", "", false
	}
	q, _ := m["query"].(string)
	tr, _ := m["time_range"].(string)
	g, _ := m["granularity"].(string)
	var chartCtx ChartContext
	if raw, err := json.Marshal(m["context"]); err == nil {
		_ = json.Unmarshal(raw, &chartCtx)
	}
	ctx := chartCtx.Format()
	if d, ok := m["description"].(string); ok {
		return d, q, tr, g, ctx, true
	}
	return "", q, tr, g, ctx, true
}

// CleanupReports removes the temp directories backing the given reports.
func CleanupReports(reports []Report) {
	for i := range reports {
		if reports[i].PNGPath != "" {
			os.RemoveAll(reports[i].PNGPath)
			reports[i].PNGPath = ""
		}
	}
}

// RenderVegaLiteReports parses `raw`, extracts any Vega-Lite JSON payload it
// contains and renders each chart spec to a PNG via vl-convert at 1x scale.
func RenderVegaLiteReports(ctx context.Context, raw string) ([]Report, error) {
	return renderVegaLiteReports(ctx, raw, "1.0")
}

// RenderVegaLiteReportsScaled is RenderVegaLiteReports with an explicit
// vl-convert scale factor (e.g. "1.0" for bots, "2.0" for web/slack).
func RenderVegaLiteReportsScaled(ctx context.Context, raw string, scale string) ([]Report, error) {
	return renderVegaLiteReports(ctx, raw, scale)
}

// RenderVegaLiteReportsWithRetry attempts to render `raw` up to attempts times,
// returning the first successful set of reports. Transient vl-convert failures
// are retried silently so callers never have to fall back to raw text. Any
// partial reports from a failed attempt are cleaned up before the next try. If
// every attempt fails, the last error is returned.
func RenderVegaLiteReportsWithRetry(ctx context.Context, raw string, scale string, attempts int) ([]Report, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		reports, err := renderVegaLiteReports(ctx, raw, scale)
		if err == nil && len(reports) > 0 {
			return reports, nil
		}
		if errors.Is(err, ErrTrivialSpec) {
			// Trivial specs are not transient failures — don't retry them.
			return nil, ErrTrivialSpec
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("no charts could be rendered")
		}
		if len(reports) > 0 {
			CleanupReports(reports)
		}
	}
	return nil, lastErr
}

// renderVegaLiteReports is the shared render pipeline. scale controls the
// vl-convert output resolution; bots render at 1.0 while the web chat renders
// at 2.0 for sharper display. Callers that only need PNGData must clean up the
// returned temp dirs via CleanupReports.
func renderVegaLiteReports(ctx context.Context, raw string, scale string) ([]Report, error) {
	body := ExtractJSONBlock(raw)

	var multi struct {
		Reports []VegaLiteReport `json:"reports"`
	}
	if err := json.Unmarshal([]byte(body), &multi); err == nil && len(multi.Reports) > 0 {
		return renderReports(ctx, multi.Reports, scale)
	}

	var wrapped VegaLiteReport
	if err := json.Unmarshal([]byte(body), &wrapped); err == nil && len(wrapped.Spec) > 0 {
		spec, err := NormalizeVegaLiteSpec(wrapped.Spec)
		if err != nil {
			return nil, err
		}
		if SpecIsTrivial(spec) {
			return nil, ErrTrivialSpec
		}
		png, pngPath, err := ConvertVegaLiteToPNG(ctx, spec, scale)
		if err != nil {
			return nil, err
		}
		return []Report{{
			PNGData:     png,
			PNGPath:     pngPath,
			Title:       FriendlyTitle(wrapped.Title, wrapped.Subtitle),
			Description: wrapped.Description,
			Query:       wrapped.Query,
		}}, nil
	}

	var rawMap map[string]any
	if err := json.Unmarshal([]byte(body), &rawMap); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if _, ok := rawMap["$schema"]; !ok && rawMap["mark"] == nil && rawMap["layer"] == nil && rawMap["vconcat"] == nil && rawMap["hconcat"] == nil {
		return nil, fmt.Errorf("not a Vega-Lite specification")
	}
	spec, err := NormalizeVegaLiteSpec([]byte(body))
	if err != nil {
		return nil, err
	}
	if SpecIsTrivial(spec) {
		return nil, ErrTrivialSpec
	}
	png, pngPath, err := ConvertVegaLiteToPNG(ctx, spec, scale)
	if err != nil {
		return nil, err
	}
	return []Report{{PNGData: png, PNGPath: pngPath, Title: "LiveReview Chart"}}, nil
}

func renderReports(ctx context.Context, reports []VegaLiteReport, scale string) ([]Report, error) {
	var out []Report
	trivialOnly := true
	for _, r := range reports {
		spec, err := NormalizeVegaLiteSpec(r.Spec)
		if err != nil {
			trivialOnly = false
			log.Printf("[VegaRender] Skipping report %q: failed to normalize spec: %s", r.Title, err)
			continue
		}
		if SpecIsTrivial(spec) {
			log.Printf("[VegaRender] Skipping trivial chart %q (single value, showing text instead)", r.Title)
			continue
		}
		trivialOnly = false
		png, pngPath, err := ConvertVegaLiteToPNG(ctx, spec, scale)
		if err != nil {
			log.Printf("[VegaRender] Skipping report %q: failed to render chart: %s", r.Title, err)
			continue
		}
		out = append(out, Report{
			PNGData:     png,
			PNGPath:     pngPath,
			Title:       FriendlyTitle(r.Title, r.Subtitle),
			Description: r.Description,
			Query:       r.Query,
		})
	}
	if len(out) == 0 {
		if trivialOnly {
			return nil, ErrTrivialSpec
		}
		return nil, fmt.Errorf("no reports could be rendered")
	}
	return out, nil
}

// NormalizeVegaLiteSpec re-marshals a spec after injecting axis label angles.
func NormalizeVegaLiteSpec(spec []byte) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(spec, &m); err != nil {
		return nil, err
	}
	sanitizeAxisFormats(m)
	injectAxisAngle(m)
	injectSafeFonts(m)
	injectFriendlyTitles(m)
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// safeFontStack is a font family every environment has, with fallbacks.
const safeFontStack = "Helvetica, Arial, sans-serif"

// injectSafeFonts works around a rendering bug in the vl-convert theme (see
// vlThemeDefault): the "powerbi" theme's config.font is a bare "Segoe UI",
// with no fallback family. Axis TITLES still render, because their font
// setting is a full CSS-style stack ("wf_standard-font, helvetica, arial,
// sans-serif") that resolves to an installed font - but axis tick labels,
// legend labels, and header labels inherit the bare top-level "font" and have
// no fallback, so vl-convert's SVG renderer draws them with zero-width glyphs
// on any server without Segoe UI installed (i.e. every non-Windows server).
// The chart LOOKS complete - correct colors, correct bar heights, axis
// titles all present - but every number and category name is silently
// missing. This has affected every chart rendered by this package, not only
// the SQL analytics path.
//
// Fix: force a font family with real fallbacks. This is layered on top of
// whatever theme is active (spec-level config merges with the theme rather
// than replacing it), so bar/line colors and other theme styling are
// untouched - only the font resolves differently. config is spec-level
// (top-level), so unlike sanitizeAxisFormats/injectAxisAngle this needs no
// recursion into layer/concat/facet children.
func injectSafeFonts(m map[string]any) {
	if m == nil {
		return
	}
	config, _ := m["config"].(map[string]any)
	if config == nil {
		config = map[string]any{}
		m["config"] = config
	}
	setIfAbsent(config, "font", safeFontStack)
	ensureNestedFont(config, "axis", "labelFont")
	ensureNestedFont(config, "legend", "labelFont")
	ensureNestedFont(config, "header", "labelFont")
	ensureNestedFont(config, "text", "font")
}

func ensureNestedFont(config map[string]any, section, key string) {
	sub, _ := config[section].(map[string]any)
	if sub == nil {
		sub = map[string]any{}
		config[section] = sub
	}
	setIfAbsent(sub, key, safeFontStack)
}

func setIfAbsent(m map[string]any, key string, value any) {
	if _, exists := m[key]; !exists {
		m[key] = value
	}
}

// temporalTypes are Vega-Lite field types that use time-based axis formatting.
var temporalTypes = map[string]bool{
	"temporal": true,
	"time":     true,
	"utc":      true,
}

// sanitizeAxisFormats recursively walks a Vega-Lite spec and removes time-format
// (`%` d3-time-format) axis formats from channels that are not temporal.
// vl-convert applies d3-format to non-time scales, which errors on `%`
// specifiers (e.g. "%Y-%m-%d") and aborts the whole render. Stripping such
// formats lets the chart render with default labels instead of failing.
func sanitizeAxisFormats(m map[string]any) {
	for _, key := range []string{"layer", "concat", "hconcat", "vconcat"} {
		if arr, ok := m[key].([]any); ok {
			for _, item := range arr {
				if child, ok := item.(map[string]any); ok {
					sanitizeAxisFormats(child)
				}
			}
		}
	}
	if child, ok := m["spec"].(map[string]any); ok {
		sanitizeAxisFormats(child)
	}
	if faceted, ok := m["facet"].(map[string]any); ok {
		sanitizeAxisFormats(faceted)
	}
	encoding, ok := m["encoding"].(map[string]any)
	if !ok {
		return
	}
	for _, v := range encoding {
		if channelMap, ok := v.(map[string]any); ok {
			stripNonTemporalAxisFormat(channelMap)
		}
	}
}

func stripNonTemporalAxisFormat(channelMap map[string]any) {
	fieldType, _ := channelMap["type"].(string)
	if temporalTypes[fieldType] {
		return
	}
	if ft, _ := channelMap["formatType"].(string); ft == "time" || ft == "utc" {
		return
	}
	if f, ok := channelMap["format"].(string); ok && strings.Contains(f, "%") {
		delete(channelMap, "format")
	}
	axis, ok := channelMap["axis"].(map[string]any)
	if !ok {
		return
	}
	if ft, _ := axis["formatType"].(string); ft == "time" || ft == "utc" {
		return
	}
	if f, ok := axis["format"].(string); ok && strings.Contains(f, "%") {
		delete(axis, "format")
	}
}

func injectAxisAngle(m map[string]any) {
	if m == nil {
		return
	}
	for _, key := range []string{"layer", "concat", "hconcat", "vconcat"} {
		if arr, ok := m[key].([]any); ok {
			for _, item := range arr {
				if child, ok := item.(map[string]any); ok {
					injectAxisAngle(child)
				}
			}
		}
	}
	if child, ok := m["spec"].(map[string]any); ok {
		injectAxisAngle(child)
	}
	encoding, ok := m["encoding"].(map[string]any)
	if !ok {
		return
	}
	for channel, v := range encoding {
		if channel != "x" && channel != "xOffset" && channel != "x2" {
			continue
		}
		channelMap, ok := v.(map[string]any)
		if !ok {
			continue
		}
		t := ""
		if typ, ok := channelMap["type"].(string); ok {
			t = typ
		}
		if t == "quantitative" {
			continue
		}
		axis, ok := channelMap["axis"].(map[string]any)
		if !ok {
			axis = map[string]any{}
			channelMap["axis"] = axis
		}
		if _, exists := axis["labelAngle"]; !exists {
			axis["labelAngle"] = float64(45)
		}
	}
}

// titleableChannels are the Vega-Lite encoding channels that render a visible
// title (axis title, legend title, or header title) and therefore benefit
// from a human-readable fallback. Channels like "tooltip", "order", and
// "detail" never render a title on their own, so they are left alone.
var titleableChannels = map[string]bool{
	"x": true, "y": true, "x2": true, "y2": true,
	"xOffset": true, "yOffset": true,
	"theta": true, "radius": true,
	"color": true, "fill": true, "stroke": true,
	"size": true, "opacity": true, "shape": true,
	"column": true, "row": true, "facet": true,
	"text": true,
}

// axisTitleAcronyms are words humanizeFieldName should upper-case wholesale
// instead of simply capitalizing the first letter.
var axisTitleAcronyms = map[string]string{
	"id": "ID", "sql": "SQL", "url": "URL", "ai": "AI",
	"pr": "PR", "mr": "MR", "loc": "LOC", "api": "API",
}

// injectFriendlyTitles walks a Vega-Lite spec and gives every titleable
// encoding channel that lacks an explicit "title" a human-readable one
// derived from its "field" (e.g. "total_billing_usage" -> "Total Billing
// Usage"). This runs regardless of what the model produced, so it is a
// backstop rather than the primary fix - the finalize prompt is expected to
// set good titles itself, but SQL column aliases (the model's `data_sql`
// output) otherwise flow straight through `encoding.<channel>.field` into
// the rendered axis title unless something rewrites them.
func injectFriendlyTitles(m map[string]any) {
	if m == nil {
		return
	}
	for _, key := range []string{"layer", "concat", "hconcat", "vconcat"} {
		if arr, ok := m[key].([]any); ok {
			for _, item := range arr {
				if child, ok := item.(map[string]any); ok {
					injectFriendlyTitles(child)
				}
			}
		}
	}
	if child, ok := m["spec"].(map[string]any); ok {
		injectFriendlyTitles(child)
	}
	if faceted, ok := m["facet"].(map[string]any); ok {
		injectFriendlyTitles(faceted)
	}
	encoding, ok := m["encoding"].(map[string]any)
	if !ok {
		return
	}
	for channel, v := range encoding {
		if !titleableChannels[channel] {
			continue
		}
		if channelMap, ok := v.(map[string]any); ok {
			applyFriendlyTitle(channelMap)
		}
	}
}

func applyFriendlyTitle(channelMap map[string]any) {
	if _, exists := channelMap["title"]; exists {
		return
	}
	field, _ := channelMap["field"].(string)
	friendly := humanizeFieldName(field)
	if friendly == "" {
		return
	}
	channelMap["title"] = friendly
}

// humanizeFieldName turns a SQL column alias into a human-readable label:
// "total_billing_usage" -> "Total Billing Usage", "reviewCount" -> "Review
// Count". Also setting an explicit title on a temporal channel suppresses
// Vega-Lite's own auto-generated "field (year-month)" suffix for a
// timeUnit-bucketed field, which is the other half of why raw field names
// were leaking into chart axes.
func humanizeFieldName(field string) string {
	if field == "" || field == "*" {
		return ""
	}
	if idx := strings.LastIndex(field, "."); idx >= 0 {
		field = field[idx+1:]
	}
	field = strings.NewReplacer("_", " ", "-", " ").Replace(field)
	var withSpaces strings.Builder
	runes := []rune(field)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && !unicode.IsUpper(runes[i-1]) {
			withSpaces.WriteByte(' ')
		}
		withSpaces.WriteRune(r)
	}
	words := strings.Fields(withSpaces.String())
	if len(words) == 0 {
		return ""
	}
	for i, w := range words {
		if upper, ok := axisTitleAcronyms[strings.ToLower(w)]; ok {
			words[i] = upper
			continue
		}
		lower := strings.ToLower(w)
		words[i] = strings.ToUpper(lower[:1]) + lower[1:]
	}
	return strings.Join(words, " ")
}

// FriendlyTitle combines a chart title and subtitle for display.
func FriendlyTitle(title, subtitle string) string {
	title = strings.TrimSpace(title)
	subtitle = strings.TrimSpace(subtitle)
	if title == "" {
		return "LiveReview Chart"
	}
	if subtitle != "" {
		return title + " — " + subtitle
	}
	return title
}

// ExtractJSONBlock pulls a JSON payload out of a markdown code fence, or, if
// no fence is present, out of a leading prose note tacked onto the front of
// it - as assembleAnalyticsResponse does for a skipped interpretation, e.g.
// `Skipped "X": single-row result (weak)\n{"reports":[...]}`. Without this
// fallback, that note text made every multi-interpretation turn with a
// skipped chart fail JSON parsing entirely, hiding otherwise-successful
// charts behind "Having an issue generating the data, please try again."
func ExtractJSONBlock(raw string) string {
	s := strings.TrimSpace(raw)
	if idx := strings.Index(s, "```json"); idx >= 0 {
		start := idx + len("```json")
		end := strings.Index(s[start:], "```")
		if end >= 0 {
			return strings.TrimSpace(s[start : start+end])
		}
	}
	if idx := strings.Index(s, "```"); idx >= 0 {
		start := idx + len("```")
		end := strings.Index(s[start:], "```")
		if end >= 0 {
			return strings.TrimSpace(s[start : start+end])
		}
	}
	if !strings.HasPrefix(s, "{") {
		if idx := strings.Index(s, "\n{"); idx >= 0 {
			return strings.TrimSpace(s[idx+1:])
		}
	}
	return s
}

// ConvertVegaLiteToPNG renders a normalized spec to a PNG via vl-convert and
// returns the in-memory PNG data plus the temp directory containing report.png.
// The caller is responsible for removing the temp dir once it is no longer
// needed. scale sets the output resolution (e.g. "1.0" or "2.0").
//
// When the VL_CONVERT_DEBUG_DIR env var is set, the temp dir is created inside
// it and retained even on failure so failed renders can be inspected.
func ConvertVegaLiteToPNG(ctx context.Context, spec []byte, scale string) ([]byte, string, error) {
	tmpDir, keepOnError, err := makeRenderDir()
	if err != nil {
		return nil, "", err
	}

	inputPath := filepath.Join(tmpDir, "report.vl.json")
	outputPath := filepath.Join(tmpDir, "report.png")

	if err := os.WriteFile(inputPath, spec, 0644); err != nil {
		cleanupRenderDir(tmpDir, keepOnError)
		return nil, "", fmt.Errorf("write spec: %w", err)
	}

	binary := os.Getenv("VL_CONVERT_BIN")
	if binary == "" {
		binary = vlConvertDefault
	}

	theme := os.Getenv("VL_CONVERT_THEME")
	if theme == "" {
		theme = vlThemeDefault
	}

	// exec.Command with arg slice (no shell); binary/theme come from server env vars, spec is written to a temp file, not argv.
	cmd := exec.CommandContext(ctx, binary, "vl2png", // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
		"-i", inputPath,
		"-o", outputPath,
		"-v", vlVersion,
		"--scale", scale,
		"--theme", theme,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		cleanupRenderDir(tmpDir, keepOnError)
		return nil, "", fmt.Errorf("vl-convert failed: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	pngData, err := os.ReadFile(outputPath)
	if err != nil {
		cleanupRenderDir(tmpDir, keepOnError)
		return nil, "", fmt.Errorf("read png: %w", err)
	}

	return pngData, tmpDir, nil
}

// makeRenderDir creates the temp dir used for a render. When VL_CONVERT_DEBUG_DIR
// is set, the dir is created inside it and retained even on error (keepOnError),
// so failed renders can be inspected.
func makeRenderDir() (tmpDir string, keepOnError bool, err error) {
	if debugDir := os.Getenv("VL_CONVERT_DEBUG_DIR"); debugDir != "" {
		if mkErr := os.MkdirAll(debugDir, 0755); mkErr == nil {
			if d, mkErr := os.MkdirTemp(debugDir, "vl-report-*"); mkErr == nil {
				return d, true, nil
			}
		}
	}
	d, mkErr := os.MkdirTemp("", "vl-report-*")
	if mkErr != nil {
		return "", false, fmt.Errorf("create temp dir: %w", mkErr)
	}
	return d, false, nil
}

func cleanupRenderDir(tmpDir string, keepOnError bool) {
	if !keepOnError {
		os.RemoveAll(tmpDir)
	}
}

// HasVegaLiteSpec reports whether the text plausibly contains a Vega-Lite spec.
func HasVegaLiteSpec(text string) bool {
	return strings.Contains(text, `"$schema"`) ||
		(strings.Contains(text, `"mark"`) && strings.Contains(text, `"encoding"`)) ||
		(strings.Contains(text, `"title"`) && strings.Contains(text, `"spec"`)) ||
		strings.Contains(text, `"reports"`)
}

// StripVegaJSON removes a leading Vega-Lite JSON payload (single report, wrapped
// report, or reports array) from the raw text, returning the remaining prose.
func StripVegaJSON(raw string) string {
	idx := strings.Index(raw, "\n{")
	if idx < 0 {
		idx = strings.Index(raw, "{")
	}
	if idx < 0 {
		return raw
	}

	depth := 0
	inStr := false
	esc := false
	start := -1

	for i := idx; i < len(raw); i++ {
		ch := raw[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			if ch == '\\' {
				esc = true
			} else if ch == '"' {
				inStr = false
			}
			continue
		}
		if ch == '"' {
			inStr = true
			esc = false
			continue
		}
		if ch == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 && start >= 0 {
				if IsVegaJSON([]byte(raw[start : i+1])) {
					return strings.TrimSpace(raw[:start] + raw[i+1:])
				}
				start = -1
			}
		}
	}
	return raw
}

// IsVegaJSON reports whether the given JSON document looks like a Vega-Lite spec
// (or a wrapper containing one).
func IsVegaJSON(data []byte) bool {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	switch v := raw.(type) {
	case map[string]any:
		if _, ok := v["reports"]; ok {
			return true
		}
		if _, ok := v["$schema"]; ok {
			return true
		}
		if _, ok := v["spec"]; ok {
			if _, hasTitle := v["title"]; hasTitle {
				return true
			}
		}
		if mark := v["mark"]; mark != nil {
			_, hasEnc := v["encoding"]
			_, hasTitle := v["title"]
			if hasEnc || hasTitle {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if _, ok := m["$schema"]; ok {
					return true
				}
				if _, ok := m["spec"]; ok {
					return true
				}
			}
		}
	}
	return false
}
