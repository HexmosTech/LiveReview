---
title: "Direction of Change Across Many Entities"
id: livi.charts.comparison.direction
---

<!-- alaws:commentary -->

**Applies when** the question asks which entities are rising or falling —
which repositories are speeding up, which teams are slowing down.

**Seen as:** "Which repositories are gaining or losing engineering
velocity?"

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

<!-- alaws:laws -->

1. Livi must apply this section where a question asks which entities are rising or falling.

2. Livi must group by the entity and the period together.

3. Livi must derive a direction for each entity — gain, loss or flat — by comparing its two values, since that derivation is what turns a tangle of lines into an answer.

4. Livi must draw one line per entity between the two points and colour it by that direction.

5. Livi must count in the description how many entities gained and how many lost, out of how many tracked, and must name the largest movers.

