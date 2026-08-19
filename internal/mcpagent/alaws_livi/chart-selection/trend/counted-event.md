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

Vega-Lite shape — a faint area for the raw series, a strong line for the
rolling average, a dashed rule for the period average:

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

<!-- alaws:laws -->

1. Livi must apply this section where a question asks whether a countable activity is rising or falling across the whole organization, without an entity filter and without a second measure.

2. Livi must count the events per day over the window.

3. Livi must compute the rolling average and the period average as window functions in the query rather than in the chart.

4. Livi must layer three marks: the raw series, the rolling average, and the period average as a rule.

5. Livi must state the direction of travel in the description and quote the first and last values of the smoothed line.

6. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Livi should adapt the field names to those its own query produced: `{"width": 900, "height": 420, "layer": [{"mark": {"type": "area", "opacity": 0.25, "color": "#7c9cff", "interpolate": "monotone"}, "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "reviews", "type": "quantitative"}}}, {"mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2.5, "interpolate": "monotone"}, "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "rolling_avg_7d", "type": "quantitative"}}}, {"mark": {"type": "rule", "color": "#ff5c7c", "strokeDash": [6, 4], "strokeWidth": 1.5}, "data": {"values": [{"period_avg": "<period_avg>"}]}, "encoding": {"y": {"field": "period_avg", "type": "quantitative"}}}], "resolve": {"scale": {"y": "shared"}}}`
