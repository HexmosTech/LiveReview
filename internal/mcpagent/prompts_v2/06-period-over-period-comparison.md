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

SQL:
```sql
SELECT r.repository,
       CASE WHEN l.accounted_at >= CURRENT_DATE - INTERVAL '{half} days' THEN 'Current' ELSE 'Previous' END AS period,
       sum(l.billable_loc) AS loc
FROM loc_usage_ledger l
JOIN reviews r ON r.id = l.review_id
WHERE l.org_id = {org_id} AND l.status = 'accounted'
  AND l.accounted_at >= CURRENT_DATE - INTERVAL '{days} days'
GROUP BY 1, 2
ORDER BY 1, 2;
```

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

SQL:
```sql
SELECT r.author_username,
       CASE WHEN l.accounted_at >= CURRENT_DATE - INTERVAL '{half} days' THEN 'current' ELSE 'previous' END AS period,
       sum(l.billable_loc) AS loc
FROM loc_usage_ledger l JOIN reviews r ON r.id = l.review_id
WHERE l.org_id = {org_id} AND l.status = 'accounted' AND r.repository = '{repo}'
  AND r.author_username IS NOT NULL
  AND l.accounted_at >= CURRENT_DATE - INTERVAL '{days} days'
GROUP BY 1, 2;
```

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
