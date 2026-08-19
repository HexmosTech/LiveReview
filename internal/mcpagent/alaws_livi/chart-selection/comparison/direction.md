---
title: "Direction of Change Across Many Entities"
id: livi.charts.comparison.direction
---

<!-- alaws:commentary -->

**Applies when** the question asks which entities are rising or falling —
which repositories are speeding up, which teams are slowing down.

**Seen as:** "Which repositories are gaining or losing engineering
velocity?"

<!-- alaws:laws -->

1. Apply this section where a question asks which entities are rising or falling.

2. Group by the entity and the period together.

3. Derive a direction for each entity — gain, loss or flat — by comparing its two values, since that derivation is what turns a tangle of lines into an answer.

4. Draw one line per entity between the two points and colour it by that direction.

5. Count in the description how many entities gained and how many lost, out of how many tracked, and name the largest movers.

6. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Adapt the field names to those its own query produced:

```json
{
  "width": 500, "height": 380,
  "mark": {"type": "line", "point": true, "strokeWidth": 2.5},
  "encoding": {
    "x": {"field": "period", "type": "nominal", "sort": ["Previous", "Current"]},
    "y": {"field": "value", "type": "quantitative"},
    "color": {"field": "trend", "type": "nominal",
              "scale": {"domain": ["gain", "flat", "loss"], "range": ["#39d353", "#8b949e", "#ff5c7c"]}},
    "detail": {"field": "entity", "type": "nominal"}
  }
}
```
