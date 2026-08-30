package chatstats

import (
	"encoding/json"
	"testing"
)

func mustSpec(t *testing.T, spec map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	return b
}

func parseAllStats(t *testing.T, raw json.RawMessage) AllStats {
	t.Helper()
	var out AllStats
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal AllStats: %v", err)
	}
	return out
}

func TestComputeAllStats_SingleSeriesTrend(t *testing.T) {
	spec := map[string]any{
		"mark": "line",
		"encoding": map[string]any{
			"x": map[string]any{"field": "day", "type": "temporal"},
			"y": map[string]any{"field": "reviews", "type": "quantitative"},
		},
		"data": map[string]any{"values": []any{
			map[string]any{"day": "2026-08-01", "reviews": 10},
			map[string]any{"day": "2026-08-02", "reviews": 20},
			map[string]any{"day": "2026-08-03", "reviews": 5},
		}},
	}
	raw, err := ComputeAllStats(mustSpec(t, spec))
	if err != nil {
		t.Fatalf("ComputeAllStats: %v", err)
	}
	if raw == nil {
		t.Fatal("expected non-nil stats")
	}
	out := parseAllStats(t, raw)
	if out.Kind != "trend" {
		t.Fatalf("kind = %q, want trend", out.Kind)
	}
	var day TrendStats
	if err := json.Unmarshal(out.Day, &day); err != nil {
		t.Fatalf("unmarshal day: %v", err)
	}
	if day.Total != 35 {
		t.Errorf("total = %v, want 35", day.Total)
	}
	if day.Peak.Value != 20 || day.Peak.Date != "2026-08-02" {
		t.Errorf("peak = %+v, want 20 on 2026-08-02", day.Peak)
	}
	if day.Low.Value != 5 || day.Low.Date != "2026-08-03" {
		t.Errorf("low = %+v, want 5 on 2026-08-03", day.Low)
	}
	if day.TrendPct == nil || *day.TrendPct != -50 {
		t.Errorf("trendPct = %v, want -50", day.TrendPct)
	}
	if out.Week == nil {
		t.Error("expected week bucket to be populated too")
	}
}

func TestComputeAllStats_MultiSeriesTrend(t *testing.T) {
	spec := map[string]any{
		"mark": "line",
		"encoding": map[string]any{
			"x":     map[string]any{"field": "day", "type": "temporal"},
			"y":     map[string]any{"field": "reviews", "type": "quantitative"},
			"color": map[string]any{"field": "trigger", "type": "nominal"},
		},
		"data": map[string]any{"values": []any{
			map[string]any{"day": "2026-08-01", "reviews": 10, "trigger": "pr"},
			map[string]any{"day": "2026-08-01", "reviews": 5, "trigger": "manual"},
			map[string]any{"day": "2026-08-02", "reviews": 30, "trigger": "pr"},
		}},
	}
	raw, err := ComputeAllStats(mustSpec(t, spec))
	if err != nil {
		t.Fatalf("ComputeAllStats: %v", err)
	}
	out := parseAllStats(t, raw)
	if out.Kind != "multi_series_trend" {
		t.Fatalf("kind = %q, want multi_series_trend", out.Kind)
	}
	var day MultiSeriesTrendStats
	if err := json.Unmarshal(out.Day, &day); err != nil {
		t.Fatalf("unmarshal day: %v", err)
	}
	if day.Total != 45 {
		t.Errorf("total = %v, want 45", day.Total)
	}
	if day.SeriesCount != 2 {
		t.Errorf("seriesCount = %v, want 2", day.SeriesCount)
	}
	if day.TopSeries.Label != "pr" || day.TopSeries.Value != 40 {
		t.Errorf("topSeries = %+v, want pr/40", day.TopSeries)
	}
}

func TestComputeAllStats_BandChart(t *testing.T) {
	spec := map[string]any{
		"mark": "bar",
		"encoding": map[string]any{
			"x": map[string]any{"field": "adoption_band", "type": "nominal"},
			"y": map[string]any{"field": "engineers", "type": "quantitative"},
		},
		"data": map[string]any{"values": []any{
			map[string]any{"adoption_band": "0 reviews", "engineers": 3},
			map[string]any{"adoption_band": "1-4", "engineers": 5},
			map[string]any{"adoption_band": "5+", "engineers": 12},
		}},
	}
	raw, err := ComputeAllStats(mustSpec(t, spec))
	if err != nil {
		t.Fatalf("ComputeAllStats: %v", err)
	}
	out := parseAllStats(t, raw)
	if out.Kind != "band" {
		t.Fatalf("kind = %q, want band", out.Kind)
	}
	var stats BandStats
	if err := json.Unmarshal(out.Stats, &stats); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	if stats.TotalActive != 17 {
		t.Errorf("totalActive = %v, want 17 (excludes the 0-reviews band)", stats.TotalActive)
	}
	if stats.Largest.Label != "5+" || stats.Largest.Value != 12 {
		t.Errorf("largest = %+v, want 5+/12", stats.Largest)
	}
}

func TestComputeAllStats_CalendarHeatmap(t *testing.T) {
	spec := map[string]any{
		"encoding": map[string]any{
			"x":     map[string]any{"field": "day", "type": "temporal", "timeUnit": "yearweek"},
			"y":     map[string]any{"field": "day", "type": "temporal", "timeUnit": "day"},
			"color": map[string]any{"field": "reviews", "type": "quantitative"},
		},
		"data": map[string]any{"values": []any{
			map[string]any{"day": "2026-08-01", "reviews": 4},
			map[string]any{"day": "2026-08-02", "reviews": 0},
			map[string]any{"day": "2026-08-03", "reviews": 9},
		}},
	}
	raw, err := ComputeAllStats(mustSpec(t, spec))
	if err != nil {
		t.Fatalf("ComputeAllStats: %v", err)
	}
	out := parseAllStats(t, raw)
	if out.Kind != "heatmap" {
		t.Fatalf("kind = %q, want heatmap", out.Kind)
	}
	var stats HeatmapStats
	if err := json.Unmarshal(out.Stats, &stats); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	if stats.Total != 13 || stats.ActiveDays != 2 {
		t.Errorf("stats = %+v, want total=13 activeDays=2", stats)
	}
	if stats.Busiest.Date != "2026-08-03" || stats.Busiest.Value != 9 {
		t.Errorf("busiest = %+v, want 9 on 2026-08-03", stats.Busiest)
	}
}

func TestComputeAllStats_SlopeGraph(t *testing.T) {
	spec := map[string]any{
		"mark": "line",
		"encoding": map[string]any{
			"x":      map[string]any{"field": "period", "type": "nominal", "sort": []any{"Previous", "Current"}},
			"y":      map[string]any{"field": "loc", "type": "quantitative"},
			"detail": map[string]any{"field": "repo"},
		},
		"data": map[string]any{"values": []any{
			map[string]any{"period": "Previous", "loc": 100, "repo": "a"},
			map[string]any{"period": "Current", "loc": 150, "repo": "a"},
			map[string]any{"period": "Previous", "loc": 200, "repo": "b"},
			map[string]any{"period": "Current", "loc": 120, "repo": "b"},
		}},
	}
	raw, err := ComputeAllStats(mustSpec(t, spec))
	if err != nil {
		t.Fatalf("ComputeAllStats: %v", err)
	}
	out := parseAllStats(t, raw)
	if out.Kind != "slope" {
		t.Fatalf("kind = %q, want slope", out.Kind)
	}
	var stats SlopeStats
	if err := json.Unmarshal(out.Stats, &stats); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	if stats.Gained != 1 || stats.Lost != 1 {
		t.Errorf("stats = %+v, want gained=1 lost=1", stats)
	}
	if stats.BiggestGain.Label != "a" || stats.BiggestGain.Delta != 50 {
		t.Errorf("biggestGain = %+v, want a/+50", stats.BiggestGain)
	}
	if stats.BiggestLoss.Label != "b" || stats.BiggestLoss.Delta != -80 {
		t.Errorf("biggestLoss = %+v, want b/-80", stats.BiggestLoss)
	}
}

func TestComputeAllStats_SlopeGraphFloatDeltas(t *testing.T) {
	spec := map[string]any{
		"mark": "line",
		"encoding": map[string]any{
			"x":      map[string]any{"field": "period", "type": "nominal", "sort": []any{"Previous", "Current"}},
			"y":      map[string]any{"field": "score", "type": "quantitative"},
			"detail": map[string]any{"field": "repo"},
		},
		"data": map[string]any{"values": []any{
			map[string]any{"period": "Previous", "score": 10.100, "repo": "a"},
			map[string]any{"period": "Current", "score": 10.117, "repo": "a"},
			map[string]any{"period": "Previous", "score": 5.010, "repo": "b"},
			map[string]any{"period": "Current", "score": 5.001, "repo": "b"},
		}},
	}
	raw, err := ComputeAllStats(mustSpec(t, spec))
	if err != nil {
		t.Fatalf("ComputeAllStats: %v", err)
	}
	out := parseAllStats(t, raw)
	var stats SlopeStats
	if err := json.Unmarshal(out.Stats, &stats); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	// 10.117 - 10.100 = 0.017, rounded to 2 decimals -> 0.02.
	if stats.BiggestGain.Label != "a" || stats.BiggestGain.Delta != 0.02 {
		t.Errorf("biggestGain = %+v, want a/0.02 (round2 of 0.017)", stats.BiggestGain)
	}
	// 5.001 - 5.010 = -0.009, rounded to 2 decimals -> -0.01.
	if stats.BiggestLoss.Label != "b" || stats.BiggestLoss.Delta != -0.01 {
		t.Errorf("biggestLoss = %+v, want b/-0.01 (round2 of -0.009)", stats.BiggestLoss)
	}
}

func TestComputeAllStats_PlainCategoryChart(t *testing.T) {
	spec := map[string]any{
		"mark": "bar",
		"encoding": map[string]any{
			"x": map[string]any{"field": "repository", "type": "nominal"},
			"y": map[string]any{"field": "loc", "type": "quantitative"},
		},
		"data": map[string]any{"values": []any{
			map[string]any{"repository": "alpha", "loc": 500},
			map[string]any{"repository": "beta", "loc": 900},
			map[string]any{"repository": "gamma", "loc": 100},
		}},
	}
	raw, err := ComputeAllStats(mustSpec(t, spec))
	if err != nil {
		t.Fatalf("ComputeAllStats: %v", err)
	}
	out := parseAllStats(t, raw)
	if out.Kind != "category" {
		t.Fatalf("kind = %q, want category", out.Kind)
	}
	var stats CategoryStats
	if err := json.Unmarshal(out.Stats, &stats); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	if stats.Highest.Label != "beta" || stats.Highest.Value != 900 {
		t.Errorf("highest = %+v, want beta/900", stats.Highest)
	}
	if stats.Lowest.Label != "gamma" || stats.Lowest.Value != 100 {
		t.Errorf("lowest = %+v, want gamma/100", stats.Lowest)
	}
}

// A layered spec whose first layer is itself a plain trend chart - the
// maybeAddRollingAverageLayer shape (internal/mcpagent/analytics.go), where
// the base bar/line layer carries the real data and later layers are
// derived rolling-average/baseline overlays with no data of their own - now
// gets real trend stats via flattenFirstLayer, not nil. This used to return
// nil (matching the frontend's layer-excludes-the-toggle rule) before
// ComputeAllStats grew layer/facet flattening + a generic fallback so every
// chart shape gets *some* numbers.
func TestComputeAllStats_LayeredTrendUsesFirstLayer(t *testing.T) {
	spec := map[string]any{
		"data": map[string]any{"values": []any{
			map[string]any{"day": "2026-08-01", "reviews": 10},
			map[string]any{"day": "2026-08-02", "reviews": 30},
		}},
		"layer": []any{
			map[string]any{
				"mark": "bar",
				"encoding": map[string]any{
					"x": map[string]any{"field": "day", "type": "temporal"},
					"y": map[string]any{"field": "reviews", "type": "quantitative"},
				},
			},
			map[string]any{
				"mark": "line",
				"encoding": map[string]any{
					"x": map[string]any{"field": "day", "type": "temporal"},
					"y": map[string]any{"field": "rolling_avg", "type": "quantitative"},
				},
			},
		},
	}
	raw, err := ComputeAllStats(mustSpec(t, spec))
	if err != nil {
		t.Fatalf("ComputeAllStats: %v", err)
	}
	out := parseAllStats(t, raw)
	if out.Kind != "trend" {
		t.Fatalf("kind = %q, want trend", out.Kind)
	}
	var day TrendStats
	if err := json.Unmarshal(out.Day, &day); err != nil {
		t.Fatalf("unmarshal day: %v", err)
	}
	if day.Total != 40 {
		t.Errorf("total = %v, want 40 (from the base layer's data, not the rolling-avg layer)", day.Total)
	}
}

// A pie/arc chart (theta + color, no x/y at all) matches none of the
// specific detectors - findGenericEncoding still finds theta as the value
// field and color as the label, so it lands as "generic" instead of nil.
func TestComputeAllStats_PieChartFallsBackToGeneric(t *testing.T) {
	spec := map[string]any{
		"mark": "arc",
		"encoding": map[string]any{
			"theta": map[string]any{"field": "count", "type": "quantitative"},
			"color": map[string]any{"field": "provider", "type": "nominal"},
		},
		"data": map[string]any{"values": []any{
			map[string]any{"provider": "GitHub", "count": 40},
			map[string]any{"provider": "GitLab", "count": 30},
			map[string]any{"provider": "Bitbucket", "count": 10},
		}},
	}
	raw, err := ComputeAllStats(mustSpec(t, spec))
	if err != nil {
		t.Fatalf("ComputeAllStats: %v", err)
	}
	out := parseAllStats(t, raw)
	if out.Kind != "generic" {
		t.Fatalf("kind = %q, want generic", out.Kind)
	}
	var stats GenericStats
	if err := json.Unmarshal(out.Stats, &stats); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	if stats.Total != 80 || stats.Count != 3 {
		t.Errorf("stats = %+v, want total=80 count=3", stats)
	}
	if stats.Highest.Label != "GitHub" || stats.Highest.Value != 40 {
		t.Errorf("highest = %+v, want GitHub/40", stats.Highest)
	}
	if stats.Lowest.Label != "Bitbucket" || stats.Lowest.Value != 10 {
		t.Errorf("lowest = %+v, want Bitbucket/10", stats.Lowest)
	}
}

// A truly unrecoverable spec - no quantitative field anywhere, in any
// layer/facet - still returns nil rather than fabricating a number from
// nothing.
func TestComputeAllStats_NoQuantitativeFieldReturnsNil(t *testing.T) {
	spec := map[string]any{
		"mark": "point",
		"encoding": map[string]any{
			"x": map[string]any{"field": "repository", "type": "nominal"},
			"y": map[string]any{"field": "author", "type": "nominal"},
		},
		"data": map[string]any{"values": []any{
			map[string]any{"repository": "a", "author": "alice"},
		}},
	}
	raw, err := ComputeAllStats(mustSpec(t, spec))
	if err != nil {
		t.Fatalf("ComputeAllStats: %v", err)
	}
	if raw != nil {
		t.Fatalf("expected nil stats for a spec with no quantitative field anywhere, got %s", raw)
	}
}

func TestComputeAllStats_EmptyDataReturnsNil(t *testing.T) {
	spec := map[string]any{
		"mark": "line",
		"encoding": map[string]any{
			"x": map[string]any{"field": "day", "type": "temporal"},
			"y": map[string]any{"field": "reviews", "type": "quantitative"},
		},
		"data": map[string]any{"values": []any{}},
	}
	raw, err := ComputeAllStats(mustSpec(t, spec))
	if err != nil {
		t.Fatalf("ComputeAllStats: %v", err)
	}
	if raw != nil {
		t.Fatalf("expected nil stats for empty data, got %s", raw)
	}
}

// Regression test: a Postgres timestamptz value can reach here with a
// non-UTC, non-Z offset (e.g. IST "+05:30"), which the naive layouts don't
// match. Before parseDate tried RFC3339 first, this silently dropped every
// row from week/month bucketing (0 rows -> nil stats) while day bucketing
// (plain string-slice) kept working - so the bug only showed up as week/
// month being mysteriously absent, never as a crash or an obviously wrong
// number.
func TestComputeAllStats_TrendWithNonUTCOffset(t *testing.T) {
	spec := map[string]any{
		"mark": "line",
		"encoding": map[string]any{
			"x": map[string]any{"field": "month", "type": "temporal"},
			"y": map[string]any{"field": "reviews", "type": "quantitative"},
		},
		"data": map[string]any{"values": []any{
			map[string]any{"month": "2026-02-01T00:00:00+05:30", "reviews": 49},
			map[string]any{"month": "2026-03-01T00:00:00+05:30", "reviews": 60},
			map[string]any{"month": "2026-04-01T00:00:00+05:30", "reviews": 31},
		}},
	}
	raw, err := ComputeAllStats(mustSpec(t, spec))
	if err != nil {
		t.Fatalf("ComputeAllStats: %v", err)
	}
	out := parseAllStats(t, raw)
	if out.Kind != "trend" {
		t.Fatalf("kind = %q, want trend", out.Kind)
	}
	if out.Week == nil {
		t.Fatal("week bucket is nil - offset dates broke week bucketing")
	}
	if out.Month == nil {
		t.Fatal("month bucket is nil - offset dates broke month bucketing")
	}
	var month TrendStats
	if err := json.Unmarshal(out.Month, &month); err != nil {
		t.Fatalf("unmarshal month: %v", err)
	}
	if month.Total != 140 {
		t.Errorf("month total = %v, want 140", month.Total)
	}
}

func TestBucketRows_WeekAndMonth(t *testing.T) {
	values := []map[string]any{
		{"day": "2026-08-01", "reviews": float64(10)}, // Saturday -> week of 2026-07-27 (Monday)
		{"day": "2026-08-03", "reviews": float64(5)},  // Monday -> week of 2026-08-03
		{"day": "2026-08-04", "reviews": float64(7)},  // Tuesday -> same week as above
	}
	weekRows := bucketRows(values, "day", "reviews", "", GranularityWeek)
	if len(weekRows) != 2 {
		t.Fatalf("week buckets = %d, want 2", len(weekRows))
	}
	monthRows := bucketRows(values, "day", "reviews", "", GranularityMonth)
	if len(monthRows) != 1 {
		t.Fatalf("month buckets = %d, want 1", len(monthRows))
	}
	if toFloat(monthRows[0]["reviews"]) != 22 {
		t.Errorf("month total = %v, want 22", monthRows[0]["reviews"])
	}
}
