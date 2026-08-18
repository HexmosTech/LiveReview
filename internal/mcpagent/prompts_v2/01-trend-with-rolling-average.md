---
id: chart.trend
number: 1
title: Trend Over Time (with Rolling Average / Confidence Band)
---

# §1 — Trend over time

## §1.0 General rule

**When the question asks whether a metric is changing over time, and the
metric is noisy day-to-day, never render the raw series alone.** Layer a
smoothing signal (rolling average, or a percentile band) on top of the raw
line/area so the reader can tell a real trend from daily noise. This
general rule is refined by the specific rules below, one per query shape
that has actually been asked.

## §1.1 Specific rule — "Is LiveReview adoption increasing since my team started using it?" (query #1)

Where the data lives:

- **Table:** `reviews` — one row per review.
- **Date column:** use whichever of `completed_at` / `created_at` is
  populated (fall back from the first to the second), since not every
  review has completed.
- **Count:** number of review rows per calendar day.
- **Window:** a trailing stretch long enough for a 7-day average to mean
  something — 90 days is the usual choice.
- **Zero-fill the calendar.** Days with no reviews must appear as 0, not
  go missing. Generate the date series and left-join the counts onto it,
  otherwise a quiet week silently closes up and the trend looks better
  than it was.
- **The smoothing and the baseline are the query's job, not the chart's.**
  Compute the 7-day rolling average and the period average as window
  functions alongside the daily count, so the chart just plots columns
  that already exist.

Vega-Lite spec (3 layers — area of raw daily reviews, 7-day rolling
average line, dashed period-average rule):
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

## §1.2 Specific rule — "What happened to a repository's velocity?" (query #10)

- Refines §1.1: same rolling-average mechanism, but on **daily LOC for one
  named repository** (not org-wide review count), plus a highlighted
  recent-interval rectangle layer §1.1 does not have.

Where the data lives:

- **Tables:** `loc_usage_ledger` joined to `reviews` — the ledger carries
  the lines-of-code figure, `reviews` carries the repository name you
  filter on.
- **Measure:** billable LOC summed per day, not a review count. "Velocity"
  is about how much code moved, and one large review is not the same
  event as one trivial one.
- **Ledger rows only count when they are settled** — filter to accounted
  status, or you will mix provisional numbers into the history.
- **Window and zero-fill:** same as §1.1 — daily granularity, trailing ~90
  days, gaps filled with 0.
- **Rolling average:** 7-day window function in the query.
- The highlighted interval (the last two weeks) is a chart-side band you
  supply as two dates; it does not come from this query.

Vega-Lite spec (3 layers — highlight rect, thin raw line, heavier rolling
average line):
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

## §1.3 Specific rule — "How much engineering work is being covered by LR?" (query #23)

- Variant of §1.0's general rule: two independent daily metrics on
  **independent y-scales** (not one metric + its own smoothed self) — LOC
  and review count plotted together to show whether "more reviews" means
  "more code inspected" or just "more, smaller reviews."

Where the data lives:

- **Two separate daily aggregates**, joined onto one shared date series:
  review count from `reviews`, and summed billable LOC from
  `loc_usage_ledger` (settled rows only).
- Aggregate each one **before** joining them together. Counting rows after
  a join between the two would multiply reviews by their ledger entries
  and inflate both numbers.
- Same daily granularity, trailing window and zero-fill as §1.1.
- No rolling average here — the second line *is* the comparison, so
  smoothing is not what makes the chart readable.

Vega-Lite spec (2 line layers, independent y-scales, one axis left/one right):
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

## §1.4 Exception — "Are reviews getting faster?" (band, not average) (query #22)

- **This is an exception to §1.0, not another instance of it.** §1.0
  smooths a single noisy line. This question is not about the center of
  the metric at all — it's about whether the *tail* (p90) is getting worse
  even if the median (p50) looks fine. A rolling average would hide
  exactly the signal this question needs, so the correct layer is an
  `errorband` (p10–p90) plus a median line, not a rolling-average line.

Where the data lives:

- **Table:** `reviews`, restricted to reviews that actually finished —
  both a completed status and a non-null completion timestamp, since an
  unfinished review has no duration.
- **Measure:** duration is the gap between creation and completion. Derive
  it per review, then convert to a unit a person reads easily (minutes or
  hours, not raw seconds).
- **Three numbers per bucket, not one:** the median plus a low and high
  percentile. Postgres computes these with an ordered-set aggregate over
  the per-review duration.
- **Bucket weekly, not daily.** Percentiles need enough reviews inside a
  bucket to be stable; a daily p90 over three reviews is noise wearing a
  statistic's clothing.

Vega-Lite spec (errorband + median line):
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
