---
title: "Two Measures Over the Same Period"
id: livi.charts.trend.two_measures
---

<!-- alaws:commentary -->

**Applies when** the question asks whether two different measures are
moving together over time — typically whether more activity also means
more substance behind it.

**Seen as:** "How much engineering work is being covered by LiveReview?"
(lines of code against review count)

```json
{
  "width": 800, "height": 340,
  "layer": [
    {"mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "loc", "type": "quantitative", "axis": {"titleColor": "#ffb454"}}}},
    {"mark": {"type": "line", "color": "#7c9cff", "strokeWidth": 2},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "reviews", "type": "quantitative", "axis": {"titleColor": "#7c9cff", "orient": "right"}}}}
  ],
  "resolve": {"scale": {"y": "independent"}}
}
```

<!-- alaws:laws -->

1. Livi must apply this section where a question asks whether two measures are moving together over time.

2. Livi must aggregate each measure separately per day and then join both onto one shared date series, because aggregating after joining them multiplies rows and inflates both figures.

3. Livi must place the two measures on independent scales with one axis on each side, since they carry different units and a shared scale flattens the smaller one.

4. Livi must not add a rolling average here, because the second line already supplies the comparison and further lines make the chart unreadable.

5. Livi must state in the description whether the two measures moved together or diverged, and must name the period of divergence where one occurred.

6. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Livi should adapt the field names to those its own query produced: `{"width": 800, "height": 340, "layer": [{"mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2}, "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "loc", "type": "quantitative", "axis": {"titleColor": "#ffb454"}}}}, {"mark": {"type": "line", "color": "#7c9cff", "strokeWidth": 2}, "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "reviews", "type": "quantitative", "axis": {"titleColor": "#7c9cff", "orient": "right"}}}}], "resolve": {"scale": {"y": "independent"}}}`
