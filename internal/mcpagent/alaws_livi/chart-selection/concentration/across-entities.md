---
title: "Concentration of a Total Across Entities"
id: livi.charts.concentration.entities
---

<!-- alaws:commentary -->

**Applies when** the question asks where something is concentrated,
whether a few account for most of the total, or whether the organization
carries a risk because too much sits in too few places. The entity may be
a repository, an engineer or a team — the section governs all of them.

**Seen as:** "Where is organizational velocity concentrated?" (by
repository, on lines of code) and "How much of the organization's activity
is covered by the top users?" (by engineer, on review count).

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

<!-- alaws:laws -->

1. Livi must apply this section where a question asks where something is concentrated or whether a few contributors account for most of a total, whatever the entity.

2. Livi must group by the entity, aggregate the measure, and sort descending, because the cumulative line is meaningless unless the rows are in descending order.

3. Livi must compute the cumulative percentage in the query as a running total over the sorted rows divided by the grand total.

4. Livi must place the cumulative line on an independent scale, or the percentages will flatten against the units of the bars.

5. Livi must retain the long tail of near-zero entries, since that tail carries the message that most are dormant.

6. Livi must quote the share held by the leading few as a single sentence, because that is the figure the reader will repeat and it should not require reading the chart.

7. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Livi should adapt the field names to those its own query produced: `{"width": 600, "height": 360, "layer": [{"mark": {"type": "bar", "color": "#7c9cff"}, "encoding": {"x": {"field": "entity", "type": "nominal", "sort": "-y"}, "y": {"field": "value", "type": "quantitative"}}}, {"mark": {"type": "line", "point": true, "color": "#ff5c7c", "strokeWidth": 2}, "encoding": {"x": {"field": "entity", "type": "nominal", "sort": "-y"}, "y": {"field": "cum_pct", "type": "quantitative", "axis": {"orient": "right"}}}}], "resolve": {"scale": {"y": "independent"}}}`
