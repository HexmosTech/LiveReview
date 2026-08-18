---
title: "Activity of Many Entities Across Days"
id: livi.charts.rhythm.entities
---

<!-- alaws:commentary -->

**Applies when** the question asks how activity is distributed across both
a set of entities and time at once — which repositories were busy when,
where the bursts and the dead stretches are.

**Seen as:** "What does engineering activity look like across repositories
and days?"

```json
{
  "width": {"step": 14}, "height": {"step": 26},
  "mark": {"type": "rect"},
  "encoding": {
    "x": {"field": "day", "type": "temporal", "axis": {"format": "%b %d", "labelAngle": -40}},
    "y": {"field": "repository", "type": "nominal", "sort": "<repos-by-total-desc>"},
    "color": {"field": "loc", "type": "quantitative", "scale": {"scheme": "blues"}}
  }
}
```

<!-- alaws:laws -->

1. Livi must apply this section where a question asks how activity is distributed across a set of entities and across time at once.

2. Livi must group by the entity and the day together, producing one row per cell of the grid.

3. Livi must keep the horizontal axis a plain temporal day axis in this section rather than the ordinal weekly banding used for habit questions, because this chart compares entities over a continuous window rather than showing a weekly rhythm.

4. Livi must sort entities by their total so the busiest sit together, and must let the chart sort from the data rather than from a fixed list that goes stale when a new entity appears.

5. Livi may leave gaps unfilled in this section, because with many entities the grid is mostly empty by nature and filling every pair inflates the result for little gain.

6. Livi must call out the specific bursts and the entities that went quiet in the description, since a dense grid without them is a picture rather than an answer.

