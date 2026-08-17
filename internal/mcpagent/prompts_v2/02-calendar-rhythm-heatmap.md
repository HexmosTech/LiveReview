---
id: chart.rhythm
number: 2
title: Calendar / Activity-Rhythm Heatmap
---

# §2 — Calendar / activity-rhythm heatmap

## §2.0 General rule

**When the question asks about a pattern across calendar days — habit,
rhythm, consistency, gaps, bursts — not "who did the most," render a
GitHub-style calendar grid, not a time-series line.** Data must be
daily-granularity and zero-filled for missing days (a missing row renders
as nothing, not a colored "zero" cell — see §2.1's zero-fill CTE). This is
the one pattern `internal/mcpagent/prompts/analytics_plan.md` already has
an explicit exception for (its rhythm/habit/consistency rule) — see
`PROMPT_LOGIC.md` §2.3.

## §2.1 Specific rule — "Are engineers actually incorporating reviews into their daily workflow?" (query #2)

- Axes: x = week (ordinal, not temporal — see note in spec), y = day-of-week (Mon/Wed/Fri labeled only), color = review count, GitHub-green threshold scale, no legend.

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
  -- one row per calendar day, count of reviews that day
  SELECT date_trunc('day', COALESCE(completed_at, created_at))::date AS day, count(*) AS value
  FROM reviews
  WHERE org_id = {org_id}
    AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{days} days'
  GROUP BY 1
)
SELECT d.day, COALESCE(daily.value, 0) AS value
FROM days d
LEFT JOIN daily ON daily.day = d.day
ORDER BY d.day;
```

Vega-Lite spec:
```json
{
  "width": {"step": "<cell_step>"}, "height": {"step": "<cell_step>"},
  "mark": {"type": "rect", "cornerRadius": 2},
  "encoding": {
    "x": {"field": "day", "type": "ordinal", "timeUnit": "yearweek",
          "scale": {"paddingInner": 0.15},
          "axis": {"format": "%b", "labelExpr": "date(datum.value) <= 7 ? timeFormat(datum.value, '%b') : ''", "labelAngle": 0}},
    "y": {"field": "day", "type": "ordinal", "timeUnit": "day",
          "sort": ["Sun","Mon","Tue","Wed","Thu","Fri","Sat"],
          "scale": {"paddingInner": 0.15},
          "axis": {"values": ["Mon","Wed","Fri"]}},
    "color": {"field": "value", "type": "quantitative",
              "scale": {"type": "threshold", "domain": [1, 3, 6, 10], "range": "<github_greens>"},
              "legend": null}
  }
}
```

**Why ordinal, not temporal (a rule inside this rule):** a temporal x-scale
here puts week columns on a continuous time scale whose band-width math
for a `rect` mark collapses/overlaps adjacent weeks. Ordinal is mandatory
for both x and y in this pattern.

## §2.2 Specific rule — "What does engineering activity look like across repositories and days?" (query #9)

- Refines §2.0: same `rect` + `color` mechanism, but the y-axis is
  **repository** (sorted by total LOC) instead of day-of-week, and x stays
  a plain temporal day axis rather than §2.1's ordinal year-week banding —
  because this chart is not trying to show a Mon–Sun weekly rhythm, it's
  comparing repos against each other over a continuous window.

SQL:
```sql
SELECT r.repository, l.accounted_at::date AS day, sum(l.billable_loc) AS loc
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
  "width": {"step": 14}, "height": {"step": 26},
  "mark": {"type": "rect"},
  "encoding": {
    "x": {"field": "day", "type": "temporal", "axis": {"format": "%b %d", "labelAngle": -40}},
    "y": {"field": "repository", "type": "nominal", "sort": "<repos-by-loc-desc>"},
    "color": {"field": "loc", "type": "quantitative", "scale": {"scheme": "blues"}}
  }
}
```
