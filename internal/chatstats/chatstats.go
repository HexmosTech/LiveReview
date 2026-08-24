// Package chatstats computes the KPI chips shown under a Livi chat chart
// (Total/Avg per period/Peak/Low/Trend, and the analogous chips for band/
// heatmap/slope/category charts) on the backend, once, at chart-build time -
// instead of the client recomputing them from raw row data on every
// day/week/month toggle click.
//
// This is a deliberate 1:1 port of the read-only detection + bucketing +
// stat-computation logic in ui/src/pages/Chatbot/rebucketChart.ts (NOT its
// buildTrendSpec/withRollingAverage/formatAxisDate, which build chart pixels
// rather than numbers, and stay a frontend rendering concern). Keep this
// file's logic byte-for-byte equivalent to that TS file - a silent
// divergence here is a wrong number shown to a CTO. When rebucketChart.ts
// changes its stat math, mirror the change here too.
package chatstats

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Granularity mirrors rebucketChart.ts's Granularity union.
type Granularity string

const (
	GranularityDay   Granularity = "day"
	GranularityWeek  Granularity = "week"
	GranularityMonth Granularity = "month"
)

var allGranularities = []Granularity{GranularityDay, GranularityWeek, GranularityMonth}

// ---- Output shapes (JSON field names match rebucketChart.ts's TrendStats /
// MultiSeriesTrendStats / BandStats / CategoryStats / HeatmapStats / SlopeStats) ----

type pointStat struct {
	Value float64 `json:"value"`
	Date  string  `json:"date"`
}

type TrendStats struct {
	Total        float64  `json:"total"`
	AvgPerPeriod float64  `json:"avgPerPeriod"`
	Peak         pointStat `json:"peak"`
	Low          pointStat `json:"low"`
	TrendPct     *float64 `json:"trendPct"`
	FirstDate    string   `json:"firstDate"`
	LastDate     string   `json:"lastDate"`
}

type seriesStat struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

type MultiSeriesTrendStats struct {
	Total       float64    `json:"total"`
	SeriesCount int        `json:"seriesCount"`
	TopSeries   seriesStat `json:"topSeries"`
	FirstDate   string     `json:"firstDate"`
	LastDate    string     `json:"lastDate"`
}

type CategoryStat struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

type BandStats struct {
	TotalActive     float64      `json:"totalActive"`
	Largest         CategoryStat `json:"largest"`
	LargestSharePct int          `json:"largestSharePct"`
}

type CategoryStats struct {
	Highest CategoryStat   `json:"highest"`
	Lowest  CategoryStat   `json:"lowest"`
	Top3    []CategoryStat `json:"top3"`
	Bottom3 []CategoryStat `json:"bottom3"`
}

type HeatmapStats struct {
	Total           float64   `json:"total"`
	ActiveDays      int       `json:"activeDays"`
	AvgOnActiveDays float64   `json:"avgOnActiveDays"`
	Busiest         pointStat `json:"busiest"`
}

type deltaStat struct {
	Label string  `json:"label"`
	Delta float64 `json:"delta"`
}

type SlopeStats struct {
	EntityCount int       `json:"entityCount"`
	Gained      int       `json:"gained"`
	Lost        int       `json:"lost"`
	Flat        int       `json:"flat"`
	BiggestGain deltaStat `json:"biggestGain"`
	BiggestLoss deltaStat `json:"biggestLoss"`
}

// AllStats is the single JSON blob persisted on chat_charts.stats and served
// to the frontend as chart.stats. Kind discriminates which of Day/Week/Month
// (trend-shaped charts) vs Stats (everything else) is populated.
type AllStats struct {
	Kind  string          `json:"kind"`
	Day   json.RawMessage `json:"day,omitempty"`
	Week  json.RawMessage `json:"week,omitempty"`
	Month json.RawMessage `json:"month,omitempty"`
	Stats json.RawMessage `json:"stats,omitempty"`
}

// ComputeAllStats detects the shape of a finalized Vega-Lite spec (the exact
// bytes the frontend receives as chart.spec) and returns the precomputed KPI
// blob for it, or nil if the chart doesn't match any of the shapes
// rebucketChart.ts knows how to summarize (e.g. a layered/faceted spec) -
// matching today's frontend behavior of showing no stat chips for those.
func ComputeAllStats(specJSON []byte) (json.RawMessage, error) {
	var spec map[string]any
	if err := json.Unmarshal(specJSON, &spec); err != nil {
		return nil, err
	}

	switch {
	case isDailyTrendChart(spec):
		if day := computeTrendStats(spec, GranularityDay); day != nil {
			out := AllStats{Kind: "trend"}
			var err error
			if out.Day, err = json.Marshal(day); err != nil {
				return nil, fmt.Errorf("marshal trend day stats: %w", err)
			}
			if week := computeTrendStats(spec, GranularityWeek); week != nil {
				if out.Week, err = json.Marshal(week); err != nil {
					return nil, fmt.Errorf("marshal trend week stats: %w", err)
				}
			}
			if month := computeTrendStats(spec, GranularityMonth); month != nil {
				if out.Month, err = json.Marshal(month); err != nil {
					return nil, fmt.Errorf("marshal trend month stats: %w", err)
				}
			}
			return json.Marshal(out)
		}
		if day := computeMultiSeriesTrendStats(spec, GranularityDay); day != nil {
			out := AllStats{Kind: "multi_series_trend"}
			var err error
			if out.Day, err = json.Marshal(day); err != nil {
				return nil, fmt.Errorf("marshal multi-series trend day stats: %w", err)
			}
			if week := computeMultiSeriesTrendStats(spec, GranularityWeek); week != nil {
				if out.Week, err = json.Marshal(week); err != nil {
					return nil, fmt.Errorf("marshal multi-series trend week stats: %w", err)
				}
			}
			if month := computeMultiSeriesTrendStats(spec, GranularityMonth); month != nil {
				if out.Month, err = json.Marshal(month); err != nil {
					return nil, fmt.Errorf("marshal multi-series trend month stats: %w", err)
				}
			}
			return json.Marshal(out)
		}
		return nil, nil
	case isBandChart(spec):
		if stats := computeBandStats(spec); stats != nil {
			out := AllStats{Kind: "band"}
			var err error
			if out.Stats, err = json.Marshal(stats); err != nil {
				return nil, fmt.Errorf("marshal band stats: %w", err)
			}
			return json.Marshal(out)
		}
		return nil, nil
	case isCalendarHeatmap(spec):
		if stats := computeHeatmapStats(spec); stats != nil {
			out := AllStats{Kind: "heatmap"}
			var err error
			if out.Stats, err = json.Marshal(stats); err != nil {
				return nil, fmt.Errorf("marshal heatmap stats: %w", err)
			}
			return json.Marshal(out)
		}
		return nil, nil
	case isSlopeGraph(spec):
		if stats := computeSlopeStats(spec); stats != nil {
			out := AllStats{Kind: "slope"}
			var err error
			if out.Stats, err = json.Marshal(stats); err != nil {
				return nil, fmt.Errorf("marshal slope stats: %w", err)
			}
			return json.Marshal(out)
		}
		return nil, nil
	case isCategoricalChart(spec):
		if stats := computeCategoryStats(spec); stats != nil {
			out := AllStats{Kind: "category"}
			var err error
			if out.Stats, err = json.Marshal(stats); err != nil {
				return nil, fmt.Errorf("marshal category stats: %w", err)
			}
			return json.Marshal(out)
		}
		return nil, nil
	default:
		return nil, nil
	}
}

// ---- shared helpers -------------------------------------------------------

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func getEncoding(spec map[string]any) (map[string]any, bool) {
	return asMap(spec["encoding"])
}

func hasAny(spec map[string]any, keys ...string) bool {
	for _, k := range keys {
		if _, ok := spec[k]; ok {
			return true
		}
	}
	return false
}

func dataValues(spec map[string]any) ([]map[string]any, bool) {
	data, ok := asMap(spec["data"])
	if !ok {
		return nil, false
	}
	arr, ok := data["values"].([]any)
	if !ok || len(arr) == 0 {
		return nil, false
	}
	rows := make([]map[string]any, 0, len(arr))
	for _, v := range arr {
		if m, ok := asMap(v); ok {
			rows = append(rows, m)
		}
	}
	if len(rows) == 0 {
		return nil, false
	}
	return rows, true
}

type encChannel struct {
	channel string
	field   string
	enc     map[string]any
}

type analyzedEncoding struct {
	temporal      *encChannel
	categorical   *encChannel
	quantitative  []encChannel
	colorField    string
	hasColorField bool
}

func analyzeEncoding(enc map[string]any) analyzedEncoding {
	var result analyzedEncoding
	// Deterministic order keeps "first quantitative field found" stable.
	channels := make([]string, 0, len(enc))
	for k := range enc {
		channels = append(channels, k)
	}
	sort.Strings(channels)
	for _, channel := range channels {
		e, ok := asMap(enc[channel])
		if !ok {
			continue
		}
		field, _ := e["field"].(string)
		if field == "" {
			continue
		}
		typ, _ := e["type"].(string)
		if channel == "color" {
			continue
		}
		switch {
		case typ == "temporal" && result.temporal == nil:
			result.temporal = &encChannel{channel, field, e}
		case (typ == "nominal" || typ == "ordinal") && result.categorical == nil:
			result.categorical = &encChannel{channel, field, e}
		case typ == "quantitative":
			result.quantitative = append(result.quantitative, encChannel{channel, field, e})
		}
	}
	if color, ok := asMap(enc["color"]); ok {
		if f, ok := color["field"].(string); ok && f != "" {
			result.colorField = f
			result.hasColorField = true
		}
	}
	return result
}

type trendFields struct {
	x           map[string]any
	xField      string
	y           map[string]any
	yField      string
	seriesField string
}

func getTrendFields(spec map[string]any) (*trendFields, bool) {
	enc, ok := getEncoding(spec)
	if !ok {
		return nil, false
	}
	found := analyzeEncoding(enc)
	if found.temporal == nil || len(found.quantitative) == 0 {
		return nil, false
	}
	tf := &trendFields{
		x:      found.temporal.enc,
		xField: found.temporal.field,
		y:      found.quantitative[0].enc,
		yField: found.quantitative[0].field,
	}
	if found.hasColorField {
		tf.seriesField = found.colorField
	}
	return tf, true
}

// isDailyTrendChart mirrors rebucketChart.ts's export of the same name: only
// the simple single-mark shape (no facet/layer/concat) is eligible.
func isDailyTrendChart(spec map[string]any) bool {
	if hasAny(spec, "layer", "facet", "hconcat", "vconcat") {
		return false
	}
	if _, ok := getTrendFields(spec); !ok {
		return false
	}
	_, ok := dataValues(spec)
	return ok
}

// bucketStart mirrors rebucketChart.ts's bucketStart: month buckets to the
// 1st, week buckets to the Monday of that ISO week, both in UTC.
func bucketStart(t time.Time, granularity Granularity) string {
	t = t.UTC()
	if granularity == GranularityMonth {
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	// week: Monday of that week. Go's Weekday: Sunday=0..Saturday=6, same as JS.
	day := int(t.Weekday())
	diffToMonday := (day + 6) % 7
	monday := time.Date(t.Year(), t.Month(), t.Day()-diffToMonday, 0, 0, 0, 0, time.UTC)
	return monday.Format("2006-01-02")
}

// parseDate mirrors the leniency of JS's `new Date(dateStr)` for the ISO-ish
// strings this data ever actually contains (see analytics.go's
// smartAggregateTime, which already normalizes some of these layouts
// upstream - but not every path does, so a Postgres timestamptz value can
// still reach here with its own zone offset, e.g. "+05:30", not just "Z" or
// "+00:00"). Offset-aware layouts are tried first so bucketStart's later
// .UTC() call converts correctly instead of silently misbucketing.
func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05.999999999Z07:00",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	naive := strings.TrimSuffix(s, "Z")
	naive = strings.Replace(naive, "+00:00", "", 1)
	for _, layout := range []string{
		"2006-01-02T15:04:05.000000",
		"2006-01-02T15:04:05.000",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, naive); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return strings.Trim(string(b), `"`)
}

// bucketRows mirrors rebucketChart.ts's bucketRows: sums yField within each
// bucket, grouped by seriesField too when present, sorted by bucket key.
func bucketRows(values []map[string]any, xField, yField, seriesField string, granularity Granularity) []map[string]any {
	type bucketKey struct {
		bucket string
		series string
	}
	grouped := make(map[bucketKey]map[string]any)
	order := make([]bucketKey, 0)
	for _, row := range values {
		raw := row[xField]
		s, ok := raw.(string)
		if !ok {
			continue
		}
		var bucket string
		if granularity == GranularityDay {
			if len(s) >= 10 {
				bucket = s[:10]
			} else {
				bucket = s
			}
		} else {
			t, ok := parseDate(s)
			if !ok {
				continue
			}
			bucket = bucketStart(t, granularity)
		}
		seriesKey := ""
		if seriesField != "" {
			seriesKey = toString(row[seriesField])
		}
		key := bucketKey{bucket, seriesKey}
		yVal := toFloat(row[yField])
		if existing, ok := grouped[key]; ok {
			existing[yField] = toFloat(existing[yField]) + yVal
		} else {
			clone := make(map[string]any, len(row)+1)
			for k, v := range row {
				clone[k] = v
			}
			clone[xField] = bucket
			clone[yField] = yVal
			grouped[key] = clone
			order = append(order, key)
		}
	}
	out := make([]map[string]any, 0, len(order))
	for _, k := range order {
		out = append(out, grouped[k])
	}
	sort.Slice(out, func(i, j int) bool {
		return toString(out[i][xField]) < toString(out[j][xField])
	})
	return out
}

// ---- trend stats ------------------------------------------------------

func computeTrendStats(spec map[string]any, granularity Granularity) *TrendStats {
	trend, ok := getTrendFields(spec)
	if !ok || trend.seriesField != "" {
		return nil
	}
	values, ok := dataValues(spec)
	if !ok {
		return nil
	}
	rows := bucketRows(values, trend.xField, trend.yField, "", granularity)
	if len(rows) == 0 {
		return nil
	}

	total := 0.0
	peakIdx, lowIdx := 0, 0
	peakVal := toFloat(rows[0][trend.yField])
	lowVal := peakVal
	for i, row := range rows {
		v := toFloat(row[trend.yField])
		total += v
		if v > peakVal {
			peakVal = v
			peakIdx = i
		}
		if v < lowVal {
			lowVal = v
			lowIdx = i
		}
	}
	avg := total / float64(len(rows))

	first := toFloat(rows[0][trend.yField])
	last := toFloat(rows[len(rows)-1][trend.yField])
	var trendPct *float64
	if first != 0 {
		pct := round2(((last - first) / first) * 100)
		trendPct = &pct
	}

	return &TrendStats{
		Total:        round2(total),
		AvgPerPeriod: round2(avg),
		Peak:         pointStat{Value: toFloat(rows[peakIdx][trend.yField]), Date: toString(rows[peakIdx][trend.xField])},
		Low:          pointStat{Value: toFloat(rows[lowIdx][trend.yField]), Date: toString(rows[lowIdx][trend.xField])},
		TrendPct:     trendPct,
		FirstDate:    toString(rows[0][trend.xField]),
		LastDate:     toString(rows[len(rows)-1][trend.xField]),
	}
}

func computeMultiSeriesTrendStats(spec map[string]any, granularity Granularity) *MultiSeriesTrendStats {
	trend, ok := getTrendFields(spec)
	if !ok || trend.seriesField == "" {
		return nil
	}
	values, ok := dataValues(spec)
	if !ok {
		return nil
	}
	rows := bucketRows(values, trend.xField, trend.yField, trend.seriesField, granularity)
	if len(rows) == 0 {
		return nil
	}

	bySeries := make(map[string]float64)
	seriesOrder := make([]string, 0)
	for _, row := range rows {
		key := toString(row[trend.seriesField])
		if _, ok := bySeries[key]; !ok {
			seriesOrder = append(seriesOrder, key)
		}
		bySeries[key] += toFloat(row[trend.yField])
	}
	sort.SliceStable(seriesOrder, func(i, j int) bool {
		return bySeries[seriesOrder[i]] > bySeries[seriesOrder[j]]
	})

	total := 0.0
	for _, v := range bySeries {
		total += v
	}
	if total == 0 {
		return nil
	}

	return &MultiSeriesTrendStats{
		Total:       round2(total),
		SeriesCount: len(seriesOrder),
		TopSeries:   seriesStat{Label: seriesOrder[0], Value: round2(bySeries[seriesOrder[0]])},
		FirstDate:   toString(rows[0][trend.xField]),
		LastDate:    toString(rows[len(rows)-1][trend.xField]),
	}
}

// ---- categorical / band stats ------------------------------------------

type categoryParts struct {
	catField string
	valField string
}

func markTypeLabel(spec map[string]any) string {
	mark := spec["mark"]
	if layer, ok := spec["layer"].([]any); ok && len(layer) > 0 {
		if first, ok := asMap(layer[0]); ok {
			mark = first["mark"]
		}
	}
	if s, ok := mark.(string); ok {
		return s
	}
	if m, ok := asMap(mark); ok {
		if s, ok := m["type"].(string); ok {
			return s
		}
	}
	return "line"
}

func getPrimaryEncoding(spec map[string]any) (map[string]any, bool) {
	if enc, ok := getEncoding(spec); ok {
		return enc, true
	}
	if layer, ok := spec["layer"].([]any); ok && len(layer) > 0 {
		if first, ok := asMap(layer[0]); ok {
			if enc, ok := asMap(first["encoding"]); ok {
				return enc, true
			}
		}
	}
	return nil, false
}

func getCategoryEncoding(spec map[string]any) (*categoryParts, bool) {
	enc, ok := getPrimaryEncoding(spec)
	if !ok {
		return nil, false
	}
	found := analyzeEncoding(enc)
	if found.categorical != nil && len(found.quantitative) > 0 {
		return &categoryParts{catField: found.categorical.field, valField: found.quantitative[0].field}, true
	}
	// Histogram fallback: bucket axis mistyped quantitative, bar mark, two
	// quantitative fields.
	if markTypeLabel(spec) == "bar" && len(found.quantitative) >= 2 {
		x, _ := asMap(enc["x"])
		y, _ := asMap(enc["y"])
		xField, _ := x["field"].(string)
		yField, _ := y["field"].(string)
		if xField != "" && yField != "" && xField != yField {
			return &categoryParts{catField: xField, valField: yField}, true
		}
	}
	return nil, false
}

func isCategoricalChart(spec map[string]any) bool {
	if hasAny(spec, "facet", "hconcat", "vconcat") {
		return false
	}
	enc, _ := getPrimaryEncoding(spec)
	parts, ok := getCategoryEncoding(spec)
	if !ok {
		return false
	}
	if enc != nil {
		if color, ok := asMap(enc["color"]); ok {
			colorField, _ := color["field"].(string)
			isDirectionColor := strings.Contains(strings.ToLower(colorField), "direction")
			if colorField != "" && colorField != parts.catField && !isDirectionColor {
				return false
			}
		}
	}
	_, ok = dataValues(spec)
	return ok
}

func isBandChart(spec map[string]any) bool {
	if !isCategoricalChart(spec) {
		return false
	}
	parts, _ := getCategoryEncoding(spec)
	field := strings.ToLower(parts.catField)
	return strings.Contains(field, "band") || strings.Contains(field, "level") || strings.Contains(field, "tier")
}

func computeCategoryStats(spec map[string]any) *CategoryStats {
	parts, ok := getCategoryEncoding(spec)
	if !ok {
		return nil
	}
	values, ok := dataValues(spec)
	if !ok {
		return nil
	}
	grouped := make(map[string]float64)
	order := make([]string, 0)
	for _, row := range values {
		label := toString(row[parts.catField])
		if _, ok := grouped[label]; !ok {
			order = append(order, label)
		}
		grouped[label] += toFloat(row[parts.valField])
	}
	if len(order) == 0 {
		return nil
	}
	sorted := make([]CategoryStat, 0, len(order))
	for _, label := range order {
		sorted = append(sorted, CategoryStat{Label: label, Value: grouped[label]})
	}
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Value > sorted[j].Value })

	top3 := sorted[:min(3, len(sorted))]
	bottom3raw := sorted[max(0, len(sorted)-3):]
	bottom3 := make([]CategoryStat, len(bottom3raw))
	for i := range bottom3raw {
		bottom3[len(bottom3raw)-1-i] = bottom3raw[i]
	}

	return &CategoryStats{
		Highest: sorted[0],
		Lowest:  sorted[len(sorted)-1],
		Top3:    append([]CategoryStat{}, top3...),
		Bottom3: bottom3,
	}
}

func computeBandStats(spec map[string]any) *BandStats {
	stats := computeCategoryStats(spec)
	if stats == nil {
		return nil
	}
	values, ok := dataValues(spec)
	if !ok {
		return nil
	}
	parts, _ := getCategoryEncoding(spec)
	totalActive := 0.0
	for _, row := range values {
		label := strings.ToLower(strings.TrimSpace(toString(row[parts.catField])))
		if strings.HasPrefix(label, "0") || strings.Contains(label, "none") {
			continue
		}
		totalActive += toFloat(row[parts.valField])
	}
	if totalActive == 0 {
		return nil
	}
	return &BandStats{
		TotalActive:     totalActive,
		Largest:         stats.Highest,
		LargestSharePct: int(round2(stats.Highest.Value / totalActive * 100)),
	}
}

// ---- calendar heatmap stats ---------------------------------------------

type heatmapParts struct {
	dateField  string
	colorField string
}

func isWeekTimeUnit(enc map[string]any) bool {
	tu, _ := enc["timeUnit"].(string)
	return strings.Contains(strings.ToLower(tu), "week")
}

func isDayTimeUnit(enc map[string]any) bool {
	tu, _ := enc["timeUnit"].(string)
	return tu == "day"
}

func getHeatmapEncoding(spec map[string]any) (*heatmapParts, bool) {
	if hasAny(spec, "layer", "facet", "hconcat", "vconcat") {
		return nil, false
	}
	enc, ok := getEncoding(spec)
	if !ok {
		return nil, false
	}
	x, _ := asMap(enc["x"])
	y, _ := asMap(enc["y"])
	color, _ := asMap(enc["color"])
	xField, _ := x["field"].(string)
	yField, _ := y["field"].(string)
	if xField == "" || yField == "" || xField != yField {
		return nil, false
	}
	colorField, _ := color["field"].(string)
	colorType, _ := color["type"].(string)
	if colorField == "" || colorType != "quantitative" {
		return nil, false
	}
	if !((isWeekTimeUnit(x) && isDayTimeUnit(y)) || (isWeekTimeUnit(y) && isDayTimeUnit(x))) {
		return nil, false
	}
	return &heatmapParts{dateField: xField, colorField: colorField}, true
}

func isCalendarHeatmap(spec map[string]any) bool {
	if _, ok := getHeatmapEncoding(spec); !ok {
		return false
	}
	_, ok := dataValues(spec)
	return ok
}

func computeHeatmapStats(spec map[string]any) *HeatmapStats {
	parts, ok := getHeatmapEncoding(spec)
	if !ok {
		return nil
	}
	values, ok := dataValues(spec)
	if !ok {
		return nil
	}
	total := 0.0
	activeDays := 0
	busiestIdx := 0
	busiestVal := toFloat(values[0][parts.colorField])
	for i, row := range values {
		v := toFloat(row[parts.colorField])
		total += v
		if v > 0 {
			activeDays++
		}
		if v > busiestVal {
			busiestVal = v
			busiestIdx = i
		}
	}
	if total == 0 {
		return nil
	}
	avg := 0.0
	if activeDays > 0 {
		avg = round2(total / float64(activeDays))
	}
	return &HeatmapStats{
		Total:           round2(total),
		ActiveDays:      activeDays,
		AvgOnActiveDays: avg,
		Busiest:         pointStat{Date: toString(values[busiestIdx][parts.dateField]), Value: toFloat(values[busiestIdx][parts.colorField])},
	}
}

// ---- slope graph stats ---------------------------------------------------

type slopeParts struct {
	periodField string
	valField    string
	entityField string
	periodOrder []string
}

func getSlopeEncoding(spec map[string]any) (*slopeParts, bool) {
	if hasAny(spec, "layer", "facet", "hconcat", "vconcat") {
		return nil, false
	}
	enc, ok := getEncoding(spec)
	if !ok {
		return nil, false
	}
	x, _ := asMap(enc["x"])
	y, _ := asMap(enc["y"])
	detail, _ := asMap(enc["detail"])
	xField, _ := x["field"].(string)
	xType, _ := x["type"].(string)
	yField, _ := y["field"].(string)
	yType, _ := y["type"].(string)
	detailField, _ := detail["field"].(string)
	if xField == "" || (xType != "nominal" && xType != "ordinal") {
		return nil, false
	}
	if yField == "" || yType != "quantitative" {
		return nil, false
	}
	if detailField == "" {
		return nil, false
	}
	var periodOrder []string
	if sortArr, ok := x["sort"].([]any); ok {
		for _, s := range sortArr {
			periodOrder = append(periodOrder, toString(s))
		}
	}
	return &slopeParts{periodField: xField, valField: yField, entityField: detailField, periodOrder: periodOrder}, true
}

func isSlopeGraph(spec map[string]any) bool {
	if _, ok := getSlopeEncoding(spec); !ok {
		return false
	}
	_, ok := dataValues(spec)
	return ok
}

func computeSlopeStats(spec map[string]any) *SlopeStats {
	parts, ok := getSlopeEncoding(spec)
	if !ok {
		return nil
	}
	values, ok := dataValues(spec)
	if !ok {
		return nil
	}

	byEntity := make(map[string]map[string]float64)
	entityOrder := make([]string, 0)
	seenPeriods := make([]string, 0)
	seenPeriodSet := make(map[string]bool)
	for _, row := range values {
		entity := toString(row[parts.entityField])
		period := toString(row[parts.periodField])
		val := toFloat(row[parts.valField])
		if !seenPeriodSet[period] {
			seenPeriodSet[period] = true
			seenPeriods = append(seenPeriods, period)
		}
		if _, ok := byEntity[entity]; !ok {
			byEntity[entity] = make(map[string]float64)
			entityOrder = append(entityOrder, entity)
		}
		byEntity[entity][period] = val
	}

	order := parts.periodOrder
	if len(order) < 2 {
		order = seenPeriods
	}
	if len(order) < 2 {
		return nil
	}
	before, after := order[0], order[1]

	var gained, lost, flat, entityCount int
	var biggestGain, biggestLoss *deltaStat
	for _, entity := range entityOrder {
		periods := byEntity[entity]
		beforeVal, hasBefore := periods[before]
		afterVal, hasAfter := periods[after]
		if !hasBefore || !hasAfter {
			continue
		}
		entityCount++
		delta := afterVal - beforeVal
		switch {
		case delta > 0:
			gained++
		case delta < 0:
			lost++
		default:
			flat++
		}
		if biggestGain == nil || delta > biggestGain.Delta {
			biggestGain = &deltaStat{Label: entity, Delta: delta}
		}
		if biggestLoss == nil || delta < biggestLoss.Delta {
			biggestLoss = &deltaStat{Label: entity, Delta: delta}
		}
	}
	if entityCount == 0 || biggestGain == nil || biggestLoss == nil {
		return nil
	}

	return &SlopeStats{
		EntityCount: entityCount,
		Gained:      gained,
		Lost:        lost,
		Flat:        flat,
		BiggestGain: deltaStat{Label: biggestGain.Label, Delta: round2(biggestGain.Delta)},
		BiggestLoss: deltaStat{Label: biggestLoss.Label, Delta: round2(biggestLoss.Delta)},
	}
}

// ---- misc -----------------------------------------------------------------

func round2(f float64) float64 {
	return float64(int64(f*100+sign(f)*0.5)) / 100
}

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
