---
title: "Exception: When Individuals Matter More Than the Shape"
id: livi.charts.distribution.individuals
---

<!-- alaws:commentary -->

**Applies when** the population is small — one repository's contributors,
one team — and the question is about *who* stands out rather than what the
overall spread looks like.

**This section overrides the chapter's bucketing rule.** Bucketing averages
people away, and here the outliers are the answer.

**Seen as:** "Which engineers are carrying the repository?"

```json
{
  "width": 600, "height": "<32 * n_members, min 200>",
  "transform": [{"calculate": "random()", "as": "jitter"}],
  "mark": {"type": "circle", "opacity": 0.85},
  "encoding": {
    "x": {"field": "loc", "type": "quantitative"},
    "y": {"field": "member", "type": "nominal", "sort": "-x"},
    "yOffset": {"field": "jitter", "type": "quantitative"},
    "size": {"field": "reviews", "type": "quantitative", "scale": {"range": [60, 900]}},
    "color": {"field": "loc", "type": "quantitative", "scale": {"scheme": "blues"}, "legend": null}
  }
}
```

<!-- alaws:laws -->

1. Livi must apply this section, in place of the chapter's bucketing rule, where the population is small and the question concerns who stands out rather than the overall spread.

2. Livi must return one row per individual and must not bin them.

3. Livi must encode two measures, one driving position along the axis and the other the size of the mark, so that a member who did much small work reads differently from one who did a little large work.

4. Livi must jitter the marks so that overlapping individuals remain visible.

5. Livi must sort by the positional measure so the heaviest contributors sit together.

6. Livi must name the individuals who stand out in the description, since the marks are evidence and the naming is the answer.

7. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Livi should adapt the field names to those its own query produced:

```json
{
  "width": 600, "height": "<32 * n_members, min 200>",
  "transform": [{"calculate": "random()", "as": "jitter"}],
  "mark": {"type": "circle", "opacity": 0.85},
  "encoding": {
    "x": {"field": "loc", "type": "quantitative"},
    "y": {"field": "member", "type": "nominal", "sort": "-x"},
    "yOffset": {"field": "jitter", "type": "quantitative"},
    "size": {"field": "reviews", "type": "quantitative", "scale": {"range": [60, 900]}},
    "color": {"field": "loc", "type": "quantitative", "scale": {"scheme": "blues"}, "legend": null}
  }
}
```
