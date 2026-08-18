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

Where the data lives:

- **Ranking by review count:** `reviews` grouped by author. **Ranking by
  LOC:** `loc_usage_ledger` joined to `reviews` for the author name,
  settled rows only. Either is defensible — count answers "who is using
  it", LOC answers "who is putting real work through it".
- **Drop rows with no author** for the same reason as §3.1.
- **Trailing 90 days**, not all time.
- **The "who hasn't" half of the question is the hard half.** Grouping the
  reviews table alone can only ever list people who *did* review — anyone
  at zero is absent from the result and therefore absent from the chart,
  which is precisely the person the question was asked about. To answer it
  honestly you need the org's member roster as the left side of the join,
  with review counts filled in as zero where there is no match. If no
  roster is reachable, say plainly in the description that the chart shows
  only engineers with at least one review, rather than letting silence
  imply everyone is on it.
- **The target line is a constant you supply**, not a queried value.

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
