---
id: chart.distribution
number: 3
title: Distribution Across a Population
---

# §3 — Distribution across a population

## §3.0 General rule

**When the question asks how spread out a metric is across many rows —
not the total, not the ranking, but the shape of the spread — bucket the
metric and render a histogram (`bar` over SQL-computed bins), or keep
every point visible with a jittered strip/beeswarm plot if the population
is small enough to show individually (roughly under ~30 points).**

## §3.1 Specific rule — "How broadly has the organization adopted LiveReview?" (query #3)

- Bucketing happens in Python (`band_for()` in `generate_breadth.py`,
  shared with §4/§1's leaderboard/growth charts), not in SQL — bands are
  `1-4 (light)` / `5-19 (regular)` / `20+ (heavy)`.

SQL (raw per-engineer counts; binning happens after, in application code):
```sql
SELECT author_username, count(*) AS reviews
FROM reviews
WHERE org_id = {org_id}
  AND author_username IS NOT NULL
  AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{days} days'
GROUP BY 1
ORDER BY 2 DESC;
```

Vega-Lite spec:
```json
{
  "width": 700, "height": 340,
  "mark": {"type": "bar", "cornerRadiusTopLeft": 4, "cornerRadiusTopRight": 4},
  "encoding": {
    "x": {"field": "band", "type": "nominal", "sort": "<band_order>", "axis": {"labelAngle": 0}},
    "y": {"field": "engineers", "type": "quantitative"},
    "color": {"field": "band", "type": "nominal", "sort": "<band_order>",
              "scale": {"domain": "<band_order>", "range": "<color_range>"}, "legend": null}
  }
}
```

## §3.2 Specific rule — "Are reviews becoming more iterative?" (query #24)

- Binning happens in SQL this time (nested `GROUP BY`), not in application
  code — the distinction from §3.1 is where the bucketing lives, not the
  chart mechanism, which is identical (`bar`, ordinal x = bucket, y =
  count).

SQL:
```sql
SELECT reviews_per_commit, count(*) AS commits
FROM (
  SELECT commit_hash, count(*) AS reviews_per_commit
  FROM reviews
  WHERE org_id = {org_id} AND commit_hash IS NOT NULL
    AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{days} days'
  GROUP BY 1
) t
GROUP BY 1
ORDER BY 1;
```

Vega-Lite spec:
```json
{
  "width": 600, "height": 340,
  "mark": {"type": "bar", "color": "#7c9cff"},
  "encoding": {
    "x": {"field": "reviews_per_commit", "type": "ordinal"},
    "y": {"field": "commits", "type": "quantitative"}
  }
}
```

## §3.3 Exception — "Which engineers are carrying the repository?" (keep every point visible) (query #12)

- **Exception to §3.0's binning default**: when the population is small
  (one repository's contributor list, not the whole org) and outliers
  themselves are the point of the question, do not bin at all — render
  every engineer as a jittered point (`circle` + `yOffset` on a
  `random()` calculate transform) so no individual gets flattened into a
  bucket average.

SQL:
```sql
SELECT r.author_username, count(*) AS reviews, sum(l.billable_loc) AS loc
FROM loc_usage_ledger l JOIN reviews r ON r.id = l.review_id
WHERE l.org_id = {org_id} AND l.status = 'accounted' AND r.repository = '{repo}'
  AND r.author_username IS NOT NULL
  AND l.accounted_at >= CURRENT_DATE - INTERVAL '{days} days'
GROUP BY 1
ORDER BY 3 DESC;
```

Vega-Lite spec:
```json
{
  "width": 600, "height": "<32 * n_engineers, min 200>",
  "transform": [{"calculate": "random()", "as": "jitter"}],
  "mark": {"type": "circle", "opacity": 0.85},
  "encoding": {
    "x": {"field": "loc", "type": "quantitative"},
    "y": {"field": "engineer", "type": "nominal", "sort": "-x"},
    "yOffset": {"field": "jitter", "type": "quantitative"},
    "size": {"field": "reviews", "type": "quantitative", "scale": {"range": [60, 900]}},
    "color": {"field": "loc", "type": "quantitative", "scale": {"scheme": "blues"}, "legend": null}
  }
}
```
