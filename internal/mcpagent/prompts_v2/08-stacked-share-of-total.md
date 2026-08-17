---
id: chart.share
number: 8
title: Stacked Share-of-Total Over Time
---

# §8 — Stacked share-of-total over time

## §8.0 General rule

**When the question asks how the composition of a total is shifting over
time (which category's share is growing/shrinking), render a 100%-stacked
mark (`"stack": "normalize"`) with color = category, not a plain count per
category.** A raw stacked count chart conflates "this category grew" with
"overall volume grew" — normalizing to 100% isolates the mix shift, which
is what these questions are actually asking about.

## §8.1 Specific rule — "Where are reviews happening?" (trigger-source mix, discrete) (query #14)

- Mark: `bar`, weekly buckets — more useful than a single pie chart because
  periods can be compared side by side.

SQL:
```sql
SELECT date_trunc('week', COALESCE(completed_at, created_at))::date AS week,
       trigger_type, count(*) AS n
FROM reviews
WHERE org_id = {org_id}
  AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{days} days'
GROUP BY 1, 2
ORDER BY 1;
```

Vega-Lite spec:
```json
{
  "width": 700, "height": 340,
  "mark": {"type": "bar"},
  "encoding": {
    "x": {"field": "week", "type": "temporal"},
    "y": {"field": "n", "type": "quantitative", "stack": "normalize", "axis": {"format": "%"}},
    "color": {"field": "trigger_type", "type": "nominal"}
  }
}
```

## §8.2 Specific rule — "Are we moving review earlier in the development lifecycle?" (trigger-source mix, continuous) (query #15)

- Refines §8.1: **identical SQL**, only the mark changes — `area`
  (`interpolate: monotone`) instead of `bar`, because this question is
  about a continuous transition/trend in the mix, not a period-by-period
  comparison. Same `"stack": "normalize"` mechanism.

SQL: identical to §8.1.

Vega-Lite spec:
```json
{
  "width": 700, "height": 340,
  "mark": {"type": "area", "interpolate": "monotone"},
  "encoding": {
    "x": {"field": "week", "type": "temporal"},
    "y": {"field": "n", "type": "quantitative", "stack": "normalize", "axis": {"format": "%"}},
    "color": {"field": "trigger_type", "type": "nominal"}
  }
}
```

**Consolidation note:** §8.1 and §8.2 share one SQL query and differ only
in `mark.type` (`bar` vs `area`) — a strong candidate for a single rule
with a "discrete comparison vs. continuous trend" mark parameter, the same
pattern flagged for §5.1/§5.2.
