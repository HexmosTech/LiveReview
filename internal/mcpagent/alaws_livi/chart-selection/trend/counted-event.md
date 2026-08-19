---
title: "Trend of a Counted Event"
id: livi.charts.trend.counted_event
---

<!-- alaws:commentary -->

**Applies when** the question asks whether some countable activity is
rising or falling across the whole organization, with no entity filter and
no second measure.

**Seen as:** "Is LiveReview adoption increasing since my team started
using it?"

<!-- alaws:laws -->

1. Apply this section where a question asks whether a countable activity is rising or falling across the whole organization, without an entity filter and without a second measure.

2. Count the events per day over the window.

3. Compute the rolling average and the period average as window functions in the query rather than in the chart.

4. Layer three marks: the raw series, the rolling average, and the period average as a rule.

5. State the direction of travel in the description and quote the first and last values of the smoothed line.

6. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Adapt the field names to those its own query produced:

```json
{
  "width": 900, "height": 420,
  "layer": [
    {"mark": {"type": "area", "opacity": 0.25, "color": "#7c9cff", "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "reviews", "type": "quantitative"}}},
    {"mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2.5, "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "rolling_avg_7d", "type": "quantitative"}}},
    {"mark": {"type": "rule", "color": "#ff5c7c", "strokeDash": [6, 4], "strokeWidth": 1.5},
     "data": {"values": [{"period_avg": "<period_avg>"}]},
     "encoding": {"y": {"field": "period_avg", "type": "quantitative"}}}
  ],
  "resolve": {"scale": {"y": "shared"}}
}
```
