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

SQL:
```sql
WITH days AS (
  SELECT generate_series(
    (CURRENT_DATE - INTERVAL '{days} days')::date,
    CURRENT_DATE::date,
    '1 day'
  )::date AS day
),
daily AS (
  SELECT date_trunc('day', COALESCE(completed_at, created_at))::date AS day, count(*) AS n
  FROM reviews
  WHERE org_id = {org_id}
    AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{days} days'
  GROUP BY 1
),
filled AS (
  SELECT d.day, COALESCE(daily.n, 0) AS reviews
  FROM days d LEFT JOIN daily ON daily.day = d.day
)
SELECT day, reviews,
       round(avg(reviews) OVER (ORDER BY day ROWS BETWEEN 6 PRECEDING AND CURRENT ROW), 2) AS rolling_avg_7d,
       round(avg(reviews) OVER (), 2) AS period_avg
FROM filled
ORDER BY day;
```

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

SQL:
```sql
WITH days AS (
  SELECT generate_series((CURRENT_DATE - INTERVAL '{days} days')::date, CURRENT_DATE::date, '1 day')::date AS day
),
daily AS (
  SELECT l.accounted_at::date AS day, sum(l.billable_loc) AS loc
  FROM loc_usage_ledger l JOIN reviews r ON r.id = l.review_id
  WHERE l.org_id = {org_id} AND l.status = 'accounted' AND r.repository = '{repo}'
    AND l.accounted_at >= CURRENT_DATE - INTERVAL '{days} days'
  GROUP BY 1
),
filled AS (
  SELECT d.day, COALESCE(daily.loc, 0) AS loc FROM days d LEFT JOIN daily ON daily.day = d.day
)
SELECT day, loc, round(avg(loc) OVER (ORDER BY day ROWS BETWEEN 6 PRECEDING AND CURRENT ROW), 1) AS rolling_avg
FROM filled ORDER BY day;
```

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

SQL:
```sql
WITH days AS (
  SELECT generate_series((CURRENT_DATE - INTERVAL '{days} days')::date, CURRENT_DATE::date, '1 day')::date AS day
),
reviews_d AS (
  SELECT date_trunc('day', COALESCE(completed_at, created_at))::date AS day, count(*) AS n
  FROM reviews WHERE org_id = {org_id}
    AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{days} days'
  GROUP BY 1
),
loc_d AS (
  SELECT accounted_at::date AS day, sum(billable_loc) AS loc
  FROM loc_usage_ledger WHERE org_id = {org_id} AND status = 'accounted'
    AND accounted_at >= CURRENT_DATE - INTERVAL '{days} days'
  GROUP BY 1
)
SELECT d.day, COALESCE(reviews_d.n, 0) AS reviews, COALESCE(loc_d.loc, 0) AS loc
FROM days d
LEFT JOIN reviews_d ON reviews_d.day = d.day
LEFT JOIN loc_d ON loc_d.day = d.day
ORDER BY d.day;
```

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

SQL:
```sql
SELECT date_trunc('week', completed_at)::date AS week,
       percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM completed_at - created_at) / 60) AS p50,
       percentile_cont(0.1) WITHIN GROUP (ORDER BY extract(epoch FROM completed_at - created_at) / 60) AS p10,
       percentile_cont(0.9) WITHIN GROUP (ORDER BY extract(epoch FROM completed_at - created_at) / 60) AS p90
FROM reviews
WHERE org_id = {org_id} AND status = 'completed' AND completed_at IS NOT NULL
  AND completed_at >= CURRENT_DATE - INTERVAL '{days} days'
GROUP BY 1
ORDER BY 1;
```

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
