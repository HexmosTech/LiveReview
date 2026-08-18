# §5 — Concentration / Pareto

> §0 applies in full. Only deviations from it are stated here.

## §5.0 General rule

**When the question asks whether a total is dominated by a few
contributors, render a sorted bar plus a cumulative-percentage line on an
independent right-hand scale.** This is the only shape that answers "do
our top three account for 80% of everything" — a plain sorted bar (§4)
shows rank but not concentration.

The descending sort is not cosmetic: the cumulative line is meaningless
without it. Compute the cumulative percentage in the query — a running
total over the sorted rows, divided by the grand total.

Expect a long tail of near-zero entries. That tail is part of the message,
so do not filter it away — but quote the top-N share in the description,
because that is the number someone will repeat in a meeting.

## §5.1 "Where is organizational velocity concentrated?" (query #7)

Data: `loc_usage_ledger` joined to `reviews`, summed LOC per repository,
sorted descending.

Vega-Lite spec:
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

## §5.2 "How much of the organization's activity is covered by the top users?" (query #29)

Same mechanism as §5.1, grouped on the engineer instead of the repository
and counting reviews instead of summing LOC. The shape is the same because
the worry is the same: whether the org is genuinely using LiveReview or
three people are carrying the numbers.

Vega-Lite spec: as §5.1 with `x.field` = `engineer` and the bar layer's `y.field` =
`reviews`.
