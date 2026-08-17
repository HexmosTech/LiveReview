---
id: chart.concentration
number: 5
title: Concentration (Pareto)
---

# §5 — Concentration / Pareto

## §5.0 General rule

**When the question asks whether a total is dominated by a few
contributors ("who accounts for most of X", "is this concentrated or
broad"), render a sorted bar plus a second `line` layer of cumulative
percent, on an independent right-hand y-scale.** This is the one pattern
that answers "do our top 3 repos/engineers account for 80% of everything"
directly — a plain sorted bar (§4's leaderboard) shows rank but not
concentration.

## §5.1 Specific rule — "Where is organizational velocity concentrated?" (by repository) (query #7)

SQL:
```sql
SELECT r.repository, sum(l.billable_loc) AS loc
FROM loc_usage_ledger l
JOIN reviews r ON r.id = l.review_id
WHERE l.org_id = {org_id} AND l.status = 'accounted'
  AND l.accounted_at >= CURRENT_DATE - INTERVAL '{days} days'
GROUP BY 1
ORDER BY 2 DESC;
```

Vega-Lite spec (bar + cumulative-% line, independent y-scales):
```json
{
  "width": 600, "height": 360,
  "layer": [
    {"mark": {"type": "bar", "color": "#7c9cff"},
     "encoding": {"x": {"field": "repository", "type": "nominal", "sort": "-y"}, "y": {"field": "loc", "type": "quantitative"}}},
    {"mark": {"type": "line", "point": true, "color": "#ff5c7c", "strokeWidth": 2},
     "encoding": {"x": {"field": "repository", "type": "nominal", "sort": "-y"},
                  "y": {"field": "cum_pct", "type": "quantitative", "axis": {"orient": "right"}}}}
  ],
  "resolve": {"scale": {"y": "independent"}}
}
```

## §5.2 Specific rule — "How much of the organization's activity is covered by the top users?" (by engineer) (query #29)

- Identical mechanism to §5.1 — only the grouping column changes
  (`author_username` instead of `repository`). This is the confirmed
  near-duplicate flagged in the chart-idea grouping review: same shape,
  same two-layer spec, only the entity being ranked differs.

SQL:
```sql
SELECT author_username, count(*) AS reviews
FROM reviews
WHERE org_id = {org_id} AND author_username IS NOT NULL
  AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{days} days'
GROUP BY 1
ORDER BY 2 DESC;
```

Vega-Lite spec: identical structure to §5.1, `x.field` = `engineer`,
`y.field` (bar layer) = `reviews`.

**Consolidation note:** §5.1 and §5.2 are the same rule with a
parameterized entity column (`repository` vs `engineer`). A future
revision of this framework should probably merge them into one rule with
an `entity` parameter rather than maintaining two near-identical specific
rules.
