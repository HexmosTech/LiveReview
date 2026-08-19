---
title: "Building Up to a Net Figure"
id: livi.charts.oneoff.net_figure
---

<!-- alaws:commentary -->

**Applies when** the question asks what something is worth, what it saved,
or what it cost overall — an answer assembled from parts that add and
subtract.

**Seen as:** "How much does LiveReview save versus alternatives?"

```json
{
  "width": 600, "height": 380,
  "mark": {"type": "bar", "size": 60},
  "encoding": {
    "x": {"field": "label", "type": "nominal", "sort": null, "axis": {"labelAngle": -20}},
    "y": {"field": "base", "type": "quantitative"},
    "y2": {"field": "top"},
    "color": {"field": "direction", "type": "nominal", "legend": null,
              "scale": {"domain": ["positive", "negative"], "range": ["#39d353", "#ff5c7c"]}}
  }
}
```

<!-- alaws:laws -->

1. Apply this section where a question asks what something is worth, saved or cost overall.

2. Take from the database only what it records, which is ordinarily a volume and a real cost.

3. Treat every other input as an assumption and name it as one.

4. State each assumption in the description, because a figure whose inputs are invisible cannot be defended by the person who has to repeat it.

5. Compute the invisible base of each bar before rendering, since the mark only draws between the two values it is given.

6. Colour additions and subtractions differently and let the final bar carry the net figure.

7. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Adapt the field names to those its own query produced:

```json
{
  "width": 600, "height": 380,
  "mark": {"type": "bar", "size": 60},
  "encoding": {
    "x": {"field": "label", "type": "nominal", "sort": null, "axis": {"labelAngle": -20}},
    "y": {"field": "base", "type": "quantitative"},
    "y2": {"field": "top"},
    "color": {"field": "direction", "type": "nominal", "legend": null,
              "scale": {"domain": ["positive", "negative"], "range": ["#39d353", "#ff5c7c"]}}
  }
}
```
