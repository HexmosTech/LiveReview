# §4 — Ranked leaderboard

> §0 applies in full. Only deviations from it are stated here.

## §4.0 Governing rule

**When a question ranks named entities against each other, render ONE
sorted horizontal bar.** Never split "most" and "least" into two charts —
they are the two ends of one ranking, not two questions. Add a dashed
target rule when a threshold is meaningful, and colour by band when a
tiered read adds more than a plain sort does.

---

## §4.1 — Ranking members against each other and against a target

**Applies when** the question asks who is doing the most or least of
something, who is behind, or who needs a nudge. Compound phrasings — "most
and least", "who is and isn't" — are still one ranking.

1. Group by the member and rank descending. Choose the measure to suit the
   question: a count answers "who is using it", a volume measure answers
   "who is putting real work through it".
2. **Handle the absent members.** Grouping the activity table alone can
   only list members who did something — anyone at zero is missing from
   the result and therefore missing from the chart, which is precisely who
   "who hasn't" is asking about. Start from the roster of members and fill
   zero where there is no match.
3. If no roster is reachable, say in the description that the chart shows
   only members with at least one event. Silence implies everyone is on
   it, which is the false claim this step exists to prevent.
4. Add the target as a rule layer. It is a constant you supply, not a
   queried value.
5. Band the bars by tier using §3.1's thresholds.
6. In the description, quote how many are below the target out of how
   many total.

**Seen as:** query #4 — "Who has adopted LiveReview — and who hasn't?"

Vega-Lite spec:
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
