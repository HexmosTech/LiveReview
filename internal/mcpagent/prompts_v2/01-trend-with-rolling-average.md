# §1 — Trend over time

> §0 applies in full. Only deviations from it are stated here.

## §1.0 General rule

**When the question asks whether a metric is changing over time, and the
metric is noisy day to day, never show the raw series alone.** Layer a
smoothing signal — a rolling average, or a percentile band — over it, so
the reader can tell a real trend from daily noise.

## §1.1 "Is LiveReview adoption increasing since my team started using it?" (query #1)

Data: `reviews`, counted per day. Compute the 7-day rolling average and
the period average as window functions alongside the daily count.

Vega-Lite spec — raw area, rolling-average line, dashed period-average rule:
```json
{
  "width": 900, "height": 420,
  "layer": [
    {"mark": {"type": "area", "opacity": 0.25, "color": "#7c9cff", "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "reviews", "type": "quantitative"}}},
    {"mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2.5, "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "rolling_avg_7d", "type": "quantitative"}}},
    {"mark": {"type": "rule", "color": "#ff5c7c", "strokeDash": [6, 4], "strokeWidth": 1.5},
     "data": {"values": [{"period_avg": "<period_avg>"}]},
     "encoding": {"y": {"field": "period_avg", "type": "quantitative"}}}
  ],
  "resolve": {"scale": {"y": "shared"}}
}
```

## §1.2 "What happened to a repository's velocity?" (query #10)

Same mechanism as §1.1 but on **daily LOC for one named repository**, plus
a highlight band over the recent interval.

Data: `loc_usage_ledger` joined to `reviews` for the repository filter.
Sum LOC, not review count — one large review is not the same event as one
trivial one. The highlight band is two dates you supply, not query output.

Vega-Lite spec — highlight rect, thin raw line, heavier rolling-average line:
```json
{
  "width": 800, "height": 340,
  "layer": [
    {"data": {"values": [{"start": "<highlight_start>", "end": "<highlight_end>"}]},
     "mark": {"type": "rect", "color": "#7c9cff", "opacity": 0.12},
     "encoding": {"x": {"field": "start", "type": "temporal"}, "x2": {"field": "end"}}},
    {"mark": {"type": "line", "color": "#3a4358", "strokeWidth": 1},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "loc", "type": "quantitative"}}},
    {"mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2.5},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "rolling_avg", "type": "quantitative"}}}
  ],
  "resolve": {"scale": {"y": "shared"}}
}
```

## §1.3 "How much engineering work is being covered by LR?" (query #23)

Two metrics on **independent y-scales**, showing whether more reviews
means more code inspected or just more, smaller reviews.

Data: two daily aggregates — review count from `reviews`, LOC from
`loc_usage_ledger` — each aggregated separately, then joined onto one
shared date series. No rolling average: the second line is the comparison.

Vega-Lite spec:
```json
{
  "width": 800, "height": 340,
  "layer": [
    {"mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "loc", "type": "quantitative", "axis": {"titleColor": "#ffb454"}}}},
    {"mark": {"type": "line", "color": "#7c9cff", "strokeWidth": 2},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "reviews", "type": "quantitative", "axis": {"titleColor": "#7c9cff", "orient": "right"}}}}
  ],
  "resolve": {"scale": {"y": "independent"}}
}
```

## §1.4 Exception — "Are reviews getting faster?" (query #22)

**Exception to §1.0.** Not about the centre of the metric — about whether
the *tail* is getting worse while the median still looks fine. A rolling
average would hide exactly that. Use a percentile band plus a median line.

Data: `reviews` that actually finished (completed status and a real
completion timestamp). Duration is completion minus creation, converted to
minutes or hours. Return three percentiles per bucket via an ordered-set
aggregate. **Bucket weekly, not daily** — a daily p90 over three reviews
is noise wearing a statistic's clothing.

Vega-Lite spec:
```json
{
  "width": 700, "height": 340,
  "layer": [
    {"mark": {"type": "errorband", "color": "#7c9cff", "opacity": 0.25},
     "encoding": {"x": {"field": "week", "type": "temporal"}, "y": {"field": "p10", "type": "quantitative"}, "y2": {"field": "p90"}}},
    {"mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2.5, "point": true},
     "encoding": {"x": {"field": "week", "type": "temporal"}, "y": {"field": "p50", "type": "quantitative"}}}
  ]
}
```
