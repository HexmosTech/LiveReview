---
id: chart.oneoff
number: 10
title: One-off Shapes (No Shared Mechanism)
---

# §10 — One-off shapes

## §10.0 General rule

**These queries don't share a chart mechanism with each other or with
§1–§9 — each gets its own specific rule directly, with no general rule
above it beyond "match the named technique to the question, don't default
to `bar`."** Grouping them here is deliberate: it stops future additions
from being forced into an ill-fitting §1–§9 bucket just to avoid an
"uncategorized" pile — a one-off is a legitimate outcome, not a filing
failure.

## §10.1 Specific rule — "Is adoption becoming broader over time?" (query #5)

- Shape: stacked `area` (true stack, not normalized like §8) of active
  engineer counts, colored by adoption tier (same bands as §3.1/§4.1).
  Closest relative is §8, but the question is "how many people at each
  depth" (absolute headcount by tier), not "what fraction of total volume"
  — so it stays unnormalized.

SQL:
```sql
SELECT date_trunc('week', COALESCE(completed_at, created_at))::date AS week,
       author_username, count(*) AS n
FROM reviews
WHERE org_id = {org_id}
  AND author_username IS NOT NULL
  AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{days} days'
GROUP BY 1, 2
ORDER BY 1;
```

Vega-Lite spec:
```json
{
  "width": 800, "height": 380,
  "mark": {"type": "area", "interpolate": "monotone", "line": {"strokeWidth": 1.5}},
  "encoding": {
    "x": {"field": "week", "type": "temporal"},
    "y": {"field": "engineers", "type": "quantitative", "stack": true},
    "color": {"field": "band", "type": "nominal", "sort": "<band_order>",
              "scale": {"domain": "<band_order>", "range": "<color_range>"}},
    "order": {"field": "band", "sort": "ascending"}
  }
}
```

## §10.2 Specific rule — "What does each engineer actually spend their review activity on?" (query #13)

- Shape: stacked `bar`, x = engineer, color = repository — shows whether
  an engineer concentrates on one repo or spreads across many.

SQL:
```sql
SELECT author_username, repository, count(*) AS reviews
FROM reviews
WHERE org_id = {org_id} AND author_username IS NOT NULL
  AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{days} days'
GROUP BY 1, 2
ORDER BY 1, 3 DESC;
```

Vega-Lite spec:
```json
{
  "width": 700, "height": 340,
  "mark": {"type": "bar"},
  "encoding": {
    "x": {"field": "engineer", "type": "nominal", "sort": "-y"},
    "y": {"field": "reviews", "type": "quantitative", "stack": true},
    "color": {"field": "repository", "type": "nominal"}
  }
}
```

## §10.3 Specific rule — "How much does LiveReview save versus alternatives?" (query #20)

- Shape: waterfall — stacked `bar` with a computed invisible base (`y`)
  and visible delta (`y2`) per step, color = positive/negative.

SQL:
```sql
SELECT count(*) AS reviews, coalesce(sum(llm_cost_usd), 0) AS llm_cost
FROM loc_usage_ledger
WHERE org_id = {org_id} AND status = 'accounted'
  AND accounted_at >= CURRENT_DATE - INTERVAL '{days} days';
```

Vega-Lite spec:
```json
{
  "width": 600, "height": 380,
  "mark": {"type": "bar", "size": 60},
  "encoding": {
    "x": {"field": "label", "type": "nominal", "sort": null, "axis": {"labelAngle": -20}},
    "y": {"field": "base", "type": "quantitative"},
    "y2": {"field": "top"},
    "color": {"field": "color", "type": "nominal", "legend": null,
              "scale": {"domain": ["positive", "negative"], "range": ["#39d353", "#ff5c7c"]}}
  }
}
```

## §10.4 Specific rule — "How much code has LR reviewed?" (long-span, compact view) (query #21)

- Shape: horizon graph — 2-3 `area` layers of the SAME field, each clamped
  to a band via SQL/app logic, stacked with rising opacity so intensity
  reads at a glance without a tall, sparse line. Reach for this only when
  the span is long (30-90+ points) and vertical space is scarce — §1's
  plain line+rolling-average is the default for shorter spans.

SQL:
```sql
WITH days AS (
  SELECT generate_series((CURRENT_DATE - INTERVAL '{days} days')::date, CURRENT_DATE::date, '1 day')::date AS day
),
daily AS (
  SELECT accounted_at::date AS day, sum(billable_loc) AS loc
  FROM loc_usage_ledger
  WHERE org_id = {org_id} AND status = 'accounted'
    AND accounted_at >= CURRENT_DATE - INTERVAL '{days} days'
  GROUP BY 1
)
SELECT d.day, COALESCE(daily.loc, 0) AS loc
FROM days d LEFT JOIN daily ON daily.day = d.day
ORDER BY d.day;
```

Vega-Lite spec (3 area layers, same field, rising opacity, band-clamped y-domain):
```json
{
  "width": 800, "height": 90,
  "layer": [
    {"mark": {"type": "area", "color": "#7c9cff", "opacity": 0.35, "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "b1", "type": "quantitative", "scale": {"domain": [0, "<band>"]}}}},
    {"mark": {"type": "area", "color": "#7c9cff", "opacity": 0.6, "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "b2", "type": "quantitative", "scale": {"domain": [0, "<band>"]}}}},
    {"mark": {"type": "area", "color": "#7c9cff", "opacity": 1.0, "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "b3", "type": "quantitative", "scale": {"domain": [0, "<band>"]}}}}
  ],
  "resolve": {"scale": {"y": "shared"}}
}
```

## §10.5 Specific rule — "Are people trusting the reviews?" (query #26)

- Shape: diverging bar — up-votes positive, down-votes negative, sharing
  one categorical (engineer) axis and a zero line. Grounded directly in
  `review_feedback`'s `vote_type` column.

SQL:
```sql
SELECT r.author_username AS engineer, f.vote_type, count(*) AS n
FROM review_feedback f
JOIN reviews r ON r.id = f.review_id
WHERE f.org_id = {org_id} AND r.author_username IS NOT NULL
  AND f.retracted_at IS NULL
  AND f.created_at >= CURRENT_DATE - INTERVAL '{days} days'
GROUP BY 1, 2;
```

Vega-Lite spec:
```json
{
  "width": 600, "height": 320,
  "mark": {"type": "bar"},
  "encoding": {
    "y": {"field": "engineer", "type": "nominal"},
    "x": {"field": "n", "type": "quantitative"},
    "color": {"field": "vote_type", "type": "nominal",
              "scale": {"domain": ["up", "down"], "range": ["#39d353", "#ff5c7c"]}}
  }
}
```

## §10.6 Exception — "What kinds of engineering problems is LiveReview finding?" (treemap requested, bar delivered) (query #17)

- Vega-Lite has no native treemap or Sankey mark. Per the standing rule in
  `internal/mcpagent/prompts/analytics_plan.md`: do not attempt to fake
  one — answer with a sorted `bar` (count per category, optionally colored
  by severity) instead. That is a faithful, honest answer to the same
  question, just not the requested picture.
- Also blocked on the same `ai_comments.content` JSON-payload issue noted
  in §9.1 — category/severity extraction needs to be reliable first.

## §10.7 Exception — "What does a healthy engineering-review workflow look like?" (connected scatterplot, not built) (query #28)

- Would be: a `line` layer (points ordered by period via `"order"`, not
  SQL row order) plus a `point` layer at the same x/y, tracing successive
  weekly states so the trajectory itself becomes meaningful. Vega-Lite
  supports this pattern explicitly; it just hasn't been built against real
  data yet.
