---
id: chart.leaderboard
number: 4
title: Ranked Leaderboard
---

# §4 — Ranked leaderboard

## §4.0 General rule

**When the question ranks named entities against each other (who did the
most/least), render ONE sorted horizontal bar — never split "most" and
"least" into two separate charts (they are the two ends of the same
ranking, not two questions).** Add a dashed target-rule layer when a
threshold is meaningful, and color by tier when a banded read (light /
regular / heavy) adds more than a plain sort does.

## §4.1 Specific rule — "Who has adopted LiveReview the most and least?" (query #4)

- Metric is configurable (`--metric reviews|loc`); default target is 5
  reviews / 200 LOC over the trailing window. Uses the same `BANDS` /
  `band_for()` tiering as §3.1 so a "tier" means the same thing across
  both charts.

SQL (`--metric reviews`, the default):
```sql
SELECT author_username, count(*) AS value
FROM reviews
WHERE org_id = {org_id}
  AND author_username IS NOT NULL
  AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{days} days'
GROUP BY 1
ORDER BY 2 DESC;
```

SQL (`--metric loc`):
```sql
SELECT r.author_username, sum(l.billable_loc) AS value
FROM loc_usage_ledger l
JOIN reviews r ON r.id = l.review_id
WHERE l.org_id = {org_id} AND l.status = 'accounted'
  AND r.author_username IS NOT NULL
  AND l.accounted_at >= CURRENT_DATE - INTERVAL '{days} days'
GROUP BY 1
ORDER BY 2 DESC;
```

Vega-Lite spec (2 layers — sorted bar colored by tier, + dashed target rule):
```json
{
  "width": 700, "height": "<max(200, 28 * n_engineers)>",
  "layer": [
    {"mark": {"type": "bar", "cornerRadiusTopRight": 3, "cornerRadiusBottomRight": 3},
     "encoding": {
       "y": {"field": "engineer", "type": "nominal", "sort": "-x"},
       "x": {"field": "value", "type": "quantitative"},
       "color": {"field": "band", "type": "nominal",
                 "scale": {"domain": "<band_order>", "range": "<color_range>"}, "legend": null}
     }},
    {"data": {"values": [{"target": "<target>"}]},
     "mark": {"type": "rule", "color": "#ff5c7c", "strokeDash": [6, 4], "strokeWidth": 1.5},
     "encoding": {"x": {"field": "target", "type": "quantitative"}}}
  ],
  "resolve": {"scale": {"y": "shared"}}
}
```
