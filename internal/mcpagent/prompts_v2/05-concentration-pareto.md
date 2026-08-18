# §5 — Concentration

> §0 applies in full. Only deviations from it are stated here.

## §5.0 Governing rule

**When a question asks whether a total is dominated by a few of its
contributors, render a sorted bar plus a cumulative-percentage line on an
independent right-hand scale.** This is the only shape that answers "do
our top three account for most of it" — a plain sorted bar (§4) shows rank
but not concentration.

---

## §5.1 — Concentration of a total across entities

**Applies when** the question asks where something is concentrated,
whether a few account for most of the total, or whether the org is
carrying a risk because too much sits in too few places. The entity can be
a repository, an engineer, a team — the law is the same.

1. Group by the entity, aggregate the measure, and **sort descending**.
   The sort is not cosmetic: the cumulative line is meaningless without
   it.
2. Compute the cumulative percentage in the query — a running total over
   the sorted rows divided by the grand total.
3. Put the cumulative line on an independent right-hand scale, or the
   percentages flatten against the bars' units.
4. Keep the long tail of near-zero entries. That tail is part of the
   message — most of these are dormant — so do not filter it away.
5. In the description, quote the top-N share as a single sentence. That is
   the number someone will repeat in a meeting, and it should not require
   reading the chart.

**Seen as:** query #7 — "Where is organizational velocity concentrated?"
(by repository, on LOC) and query #29 — "How much of the organization's
activity is covered by the top users?" (by engineer, on review count).
Same law, different entity and measure.

Vega-Lite spec:
```json
{
  "width": 600, "height": 360,
  "layer": [
    {"mark": {"type": "bar", "color": "#7c9cff"},
     "encoding": {"x": {"field": "entity", "type": "nominal", "sort": "-y"}, "y": {"field": "value", "type": "quantitative"}}},
    {"mark": {"type": "line", "point": true, "color": "#ff5c7c", "strokeWidth": 2},
     "encoding": {"x": {"field": "entity", "type": "nominal", "sort": "-y"},
                  "y": {"field": "cum_pct", "type": "quantitative", "axis": {"orient": "right"}}}}
  ],
  "resolve": {"scale": {"y": "independent"}}
}
```
