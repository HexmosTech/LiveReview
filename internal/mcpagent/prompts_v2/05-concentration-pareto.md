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

Where the data lives:

- **Tables:** `loc_usage_ledger` joined to `reviews` for the repository
  name; settled rows only; trailing 90 days.
- **Measure:** summed LOC per repository, sorted descending. The sort is
  not cosmetic — a Pareto curve is meaningless unless the bars are in
  descending order, because the cumulative line assumes it.
- **The cumulative percentage is a query column, not a chart feature.**
  Compute a running total over the sorted rows and divide by the grand
  total. Two window functions over the same ordering.
- Expect a long tail of near-zero repositories. That tail is part of the
  message ("most of these are dormant"), so do not filter it away — but do
  quote the top-N share in the description, because that is the number
  someone will repeat in a meeting.

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

Where the data lives: identical in shape to §5.1 — same descending sort,
same running-total-over-grand-total for the cumulative line — but grouped
on the author column of `reviews` instead of the repository, and counting
reviews rather than summing LOC.

Vega-Lite spec: identical structure to §5.1, `x.field` = `engineer`,
`y.field` (bar layer) = `reviews`.

**Consolidation note:** §5.1 and §5.2 are the same rule with a
parameterized entity column (`repository` vs `engineer`). A future
revision of this framework should probably merge them into one rule with
an `entity` parameter rather than maintaining two near-identical specific
rules.
