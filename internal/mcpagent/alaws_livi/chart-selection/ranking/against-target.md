---
title: "Ranking Members Against Each Other and a Target"
id: livi.charts.ranking.against_target
---

<!-- alaws:commentary -->

**Applies when** the question asks who is doing the most or least of
something, who is behind, or who needs a nudge.

**Seen as:** "Who has adopted LiveReview — and who hasn't?"

```json
{
  "width": 700, "height": "<max(200, 28 * n_members)>",
  "layer": [
    {"mark": {"type": "bar", "cornerRadiusTopRight": 3, "cornerRadiusBottomRight": 3},
     "encoding": {
       "y": {"field": "member", "type": "nominal", "sort": "-x"},
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

<!-- alaws:laws -->

1. Livi must apply this section where a question asks who is doing the most or least of something, including where the question names both ends at once.

2. Livi must group by the member and rank descending, choosing a count where the question asks who is using the tool and a volume measure where it asks who is putting real work through it.

3. Livi must account for members with no activity by starting from the roster of members and filling zero where there is no match, because grouping the activity table alone can only list members who did something and omits precisely those the question asks about.

4. Where no roster is reachable, Livi must state in the description that the chart shows only members with at least one event, since silence implies that everyone appears.

5. Livi must draw the target as a separate rule layer from a value it supplies, not from the query.

6. Livi must band the bars by tier using the same thresholds as the Distribution chapter.

7. Livi must quote how many members fall below the target, out of how many in total.

