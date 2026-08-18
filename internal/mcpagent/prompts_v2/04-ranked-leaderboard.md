# §4 — Ranked leaderboard

> §0 applies in full. Only deviations from it are stated here.

## §4.0 General rule

**When the question ranks named entities against each other, render ONE
sorted horizontal bar.** Never split "most" and "least" into two charts —
they are the two ends of one ranking, not two questions. Add a dashed
target rule when a threshold is meaningful, and colour by tier when a
banded read adds more than a plain sort does.

## §4.1 "Who has adopted LiveReview the most and least?" (query #4)

Data: `reviews` grouped by author for a review-count ranking, or
`loc_usage_ledger` joined to `reviews` for a LOC ranking. Either is
defensible — count answers "who is using it", LOC answers "who is putting
real work through it". Use the same tier thresholds as §3.1.

**The "who hasn't" half is the hard half.** Grouping the reviews table
alone can only list people who *did* review — anyone at zero is absent
from the result and so absent from the chart, which is exactly the person
being asked about. To answer honestly, start from the org's member roster
and fill in zero where there is no match. If no roster is reachable, say
in the description that the chart shows only engineers with at least one
review, rather than letting silence imply everyone is on it.

The target line is a constant you supply, not a queried value.

Vega-Lite spec — sorted bar coloured by tier, plus dashed target rule:
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
