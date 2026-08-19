---
title: "Composition at a Single Moment"
id: livi.charts.composition.moment
---

<!-- alaws:commentary -->

**Applies when** the question asks what a total is made of right now,
with no comparison over time — the split of one whole into its parts.

This is the narrow case where a pie or donut earns its place. It reads
well only for a handful of slices; beyond that the eye cannot compare
angles and a sorted bar communicates the same data better. Any question
that asks whether the mix is *changing* belongs to the shift section
instead, because a pie can only show one moment.

```json
{
  "width": 400, "height": 400,
  "mark": {"type": "arc", "innerRadius": 60},
  "encoding": {
    "theta": {"field": "n", "type": "quantitative"},
    "color": {"field": "category", "type": "nominal"}
  }
}
```

<!-- alaws:laws -->

1. Apply this section where a question asks what a total is composed of at a single moment, with no comparison across periods.

2. Use the `arc` mark only where there are six categories or fewer, and use a sorted bar instead where there are more, since angles beyond a handful of slices cannot be compared by eye.

3. Route a question about whether the mix is changing to the shift section instead, because a single arc chart cannot show a change over time.

4. Quote the largest share as a percentage in the description, so the headline does not depend on reading the chart.

5. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Adapt the field names to those its own query produced:

```json
{
  "width": 400, "height": 400,
  "mark": {"type": "arc", "innerRadius": 60},
  "encoding": {
    "theta": {"field": "n", "type": "quantitative"},
    "color": {"field": "category", "type": "nominal"}
  }
}
```
