---
id: chart.comparison
number: 6
title: Period-over-Period Comparison
---

# §6 — Period-over-period comparison

## §6.0 General rule

**When the question asks whether something is "gaining or losing," "went
up or down," or "changed between two points in time" — collapse the
window into exactly two buckets (Previous/Current, W1/W2) in SQL, never
leave it as a multi-bucket trend.** A monthly/weekly trend answers a
different question ("how has this moved continuously") than a two-period
comparison ("where do we stand now vs. before"). This is the general rule
`analytics_plan.md` is missing entirely today — see `PROMPT_LOGIC.md`
§2's gap table.

## §6.1 Specific rule — "Which repositories are gaining or losing engineering velocity?" (query #6)

- Shape: slope graph — one line per repository between exactly two x
  points (`Previous`, `Current`), colored by direction.

Where the data lives:

- **Tables:** `loc_usage_ledger` joined to `reviews`; settled rows only.
- **The key move is labelling each row Previous or Current** with a
  conditional expression on its date, then grouping by repository *and*
  that label. Take a window — 90 days is comfortable — and split it down
  the middle, so both halves are the same length and the comparison is
  fair.
- **Exactly two buckets.** Not six months of monthly points: the question
  is "where do we stand now versus before", and a multi-bucket trend
  forces the reader to eyeball a direction that the chart should be
  stating outright.
- **Derive the gain/loss label per repository** by comparing its two
  values, and encode that as the line colour. That label is what turns a
  tangle of lines into an answer.

Vega-Lite spec:
```json
{
  "width": 500, "height": 380,
  "mark": {"type": "line", "point": true, "strokeWidth": 2.5},
  "encoding": {
    "x": {"field": "period", "type": "nominal", "sort": ["Previous", "Current"]},
    "y": {"field": "loc", "type": "quantitative"},
    "color": {"field": "trend", "type": "nominal",
              "scale": {"domain": ["gain", "flat", "loss"], "range": ["#39d353", "#8b949e", "#ff5c7c"]}},
    "detail": {"field": "repository", "type": "nominal"}
  }
}
```

## §6.2 Specific rule — "What changed between week 1 and week 2?" (query #30)

- Refines §6.0 for **multiple metrics at once** (reviews, LOC, active
  engineers, repos, ...), not one metric for many entities like §6.1 — a
  `rect`+`color` heatmap (metric × period, color = delta) with a `text`
  layer overlaying the actual numbers, instead of a slope line per entity.

Where the data lives:

- **Several small queries, not one big one.** Each row of the matrix is a
  different metric from a different place — review counts from `reviews`,
  LOC from `loc_usage_ledger`, active engineers as a distinct author
  count, repositories touched, trigger-type share. There is no single
  join that produces all of them sensibly.
- **Run each metric for both halves of the window**, then assemble the
  results into one small table of metric / period / value rows.
- **The delta is a third "period" column**, computed once you have the
  two values — that is what the colour scale reads.
- Keep the metric list short and stable. This chart is a verdict, and a
  verdict with fifteen rows is a spreadsheet.

Vega-Lite spec (2 layers — delta-colored rect, text overlay):
```json
{
  "width": 300, "height": 220,
  "layer": [
    {"mark": {"type": "rect"},
     "encoding": {
       "x": {"field": "period", "type": "nominal", "sort": ["W1", "W2", "Delta"]},
       "y": {"field": "metric", "type": "nominal", "sort": "<metric_order>"},
       "color": {"condition": {"test": "datum.is_delta", "field": "value", "type": "quantitative",
                                "scale": {"domainMid": 0, "range": ["#ff5c7c", "#232a3d", "#39d353"]}},
                 "value": "#161b22"}
     }},
    {"mark": {"type": "text", "color": "#e6ebf5", "fontSize": 12},
     "encoding": {
       "x": {"field": "period", "type": "nominal", "sort": ["W1", "W2", "Delta"]},
       "y": {"field": "metric", "type": "nominal", "sort": "<metric_order>"},
       "text": {"field": "value", "type": "quantitative", "format": ".1f"}
     }}
  ]
}
```

## §6.3 Exception — "Why did this repository's velocity change?" (decomposition, not just the delta) (query #11)

- **Exception to §6.0/§6.1:** "why" is not answered by showing that
  velocity changed (§6.1 already does that) — it needs the change
  decomposed per contributor, so a sorted diverging bar (one bar per
  engineer, colored up/down) is used instead of a slope line or a
  metric×period matrix.

Where the data lives:

- Same two-period split as §6.1 and the same tables, but grouped by
  **author** and filtered to **one repository** — the question is about
  what happened inside a single repo.
- **Then subtract:** one row per engineer carrying the change between
  their two periods, plus the direction of that change for the colour. The
  chart plots the delta, not the two raw values, so the arithmetic belongs
  in the query.
- Sort by the delta so the biggest movers sit at the ends, which is where
  the eye goes first.

Vega-Lite spec:
```json
{
  "width": 600, "height": "<max(200, 30 * n_engineers)>",
  "mark": {"type": "bar"},
  "encoding": {
    "y": {"field": "engineer", "type": "nominal", "sort": "x"},
    "x": {"field": "delta", "type": "quantitative"},
    "color": {"field": "direction", "type": "nominal", "legend": null,
              "scale": {"domain": ["up", "down"], "range": ["#39d353", "#ff5c7c"]}}
  }
}
```
