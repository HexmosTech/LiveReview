---
id: chart.relationship
number: 7
title: Relationship Between Two Numeric Measures (Scatter / Bubble)
---

# §7 — Relationship between two numeric measures

## §7.0 General rule

**When the question compares two (or three) independent numeric measures
against each other per entity — not one measure ranked, but "how do X and
Y relate" — render a scatter/bubble chart: `circle` mark, x/y = the two
measures, `size` for a third measure if one exists, `color` for the
entity or a fourth dimension.** A single-metric bar chart cannot show this
relationship even if it uses the "right" metric, because it only has one
axis.

## §7.1 Specific rule — "Which repositories are unusually active or inactive?" (query #8)

- Measures: LOC reviewed (x) vs. review count (y) — separates
  high-volume/high-frequency repos from large-diff/low-frequency ones.
  `size` = active engineers.

SQL:
```sql
SELECT r.repository,
       count(*) AS reviews,
       count(DISTINCT r.author_username) AS engineers,
       coalesce(sum(l.billable_loc), 0) AS loc
FROM reviews r
LEFT JOIN loc_usage_ledger l ON l.review_id = r.id AND l.status = 'accounted'
WHERE r.org_id = {org_id}
  AND COALESCE(r.completed_at, r.created_at) >= CURRENT_DATE - INTERVAL '{days} days'
GROUP BY 1
ORDER BY reviews DESC;
```

Vega-Lite spec:
```json
{
  "width": 600, "height": 380,
  "mark": {"type": "circle", "opacity": 0.85},
  "encoding": {
    "x": {"field": "loc", "type": "quantitative"},
    "y": {"field": "reviews", "type": "quantitative"},
    "size": {"field": "engineers", "type": "quantitative", "scale": {"range": [80, 1200]}},
    "color": {"field": "repository", "type": "nominal", "legend": null}
  }
}
```

## §7.2 Specific rule — "Which engineers are getting the most value from LR?" (query #25)

- Measures: reviews (x) vs. up-voted feedback as a proxy for useful
  findings (y). `size` = LOC, `color` = acceptance %. Separates "heavy
  users" from "productive users" — the two are not the same thing.

SQL:
```sql
SELECT r.author_username AS engineer,
       count(DISTINCT r.id) AS reviews,
       coalesce(sum(l.billable_loc), 0) AS loc,
       count(*) FILTER (WHERE f.vote_type = 'up') AS up_votes,
       count(*) FILTER (WHERE f.vote_type = 'down') AS down_votes
FROM reviews r
LEFT JOIN loc_usage_ledger l ON l.review_id = r.id AND l.status = 'accounted'
LEFT JOIN review_feedback f ON f.review_id = r.id AND f.org_id = {org_id} AND f.retracted_at IS NULL
WHERE r.org_id = {org_id} AND r.author_username IS NOT NULL
  AND COALESCE(r.completed_at, r.created_at) >= CURRENT_DATE - INTERVAL '{days} days'
GROUP BY 1;
```

Vega-Lite spec:
```json
{
  "width": 600, "height": 380,
  "mark": {"type": "circle", "opacity": 0.85},
  "encoding": {
    "x": {"field": "reviews", "type": "quantitative"},
    "y": {"field": "useful_findings", "type": "quantitative"},
    "size": {"field": "loc", "type": "quantitative", "scale": {"range": [60, 900]}},
    "color": {"field": "acceptance", "type": "quantitative", "scale": {"scheme": "greens"}}
  }
}
```

## §7.3 Specific rule — "Which repositories have the highest review coverage?" (query #27)

- Measures: reviews/PRs ratio (x, "coverage") vs. LOC reviewed (y). Same
  mechanism as §7.1 — near-duplicate flagged in the chart-idea grouping
  review, both are "LOC/volume vs. a second repo-level measure, sized by
  engineers." The distinguishing value here is the `coverage` ratio
  itself, joined against `pull_requests`, which §7.1 does not compute.

SQL:
```sql
SELECT rp.name AS repository,
       count(DISTINCT pr.id) AS prs,
       count(DISTINCT r.id) AS reviews,
       count(DISTINCT r.author_username) AS engineers,
       coalesce(sum(l.billable_loc), 0) AS loc
FROM repositories rp
LEFT JOIN pull_requests pr ON pr.repository_id = rp.id
  AND pr.provider_created_at >= CURRENT_DATE - INTERVAL '{days} days'
LEFT JOIN reviews r ON r.repository = rp.name AND r.org_id = {org_id}
  AND COALESCE(r.completed_at, r.created_at) >= CURRENT_DATE - INTERVAL '{days} days'
LEFT JOIN loc_usage_ledger l ON l.review_id = r.id AND l.status = 'accounted'
WHERE rp.org_id = {org_id}
GROUP BY 1
HAVING count(DISTINCT r.id) > 0
ORDER BY 3 DESC;
```

Vega-Lite spec:
```json
{
  "width": 600, "height": 380,
  "mark": {"type": "circle", "opacity": 0.85},
  "encoding": {
    "x": {"field": "coverage", "type": "quantitative"},
    "y": {"field": "loc", "type": "quantitative"},
    "size": {"field": "engineers", "type": "quantitative", "scale": {"range": [60, 900]}},
    "color": {"field": "coverage", "type": "quantitative", "scale": {"scheme": "blues"}}
  }
}
```

## §7.4 Exception — "What is the blast radius of issues being caught?" (not built) (query #19)

- Needs `ai_comments.file_path`-level joins that
  haven't been validated against real data; file-level blast radius is
  possible, dependency-level would need metadata that doesn't exist today.
- Would follow §7.0's general mechanism (bubble: findings per review ×
  files affected, size = severity/LOC) once the data is confirmed
  reliable — documented as a pending instance of this rule, not a new one.
