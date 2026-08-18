---
title: "Shift in the Composition of a Total"
id: livi.charts.composition.shift
---

<!-- alaws:commentary -->

**Applies when** the question asks where something is coming from, whether
one channel is taking over from another, or whether work is moving from
one stage to another.

**Seen as:** "Where are reviews happening?" (discrete periods, drawn as
bars) and "Are we moving review earlier in the development lifecycle?"
(a continuous transition, drawn as areas).

Swap the mark type for the continuous form; nothing else changes.

```json
{
  "width": 700, "height": 340,
  "mark": {"type": "bar"},
  "encoding": {
    "x": {"field": "week", "type": "temporal"},
    "y": {"field": "n", "type": "quantitative", "stack": "normalize", "axis": {"format": "%"}},
    "color": {"field": "category", "type": "nominal"}
  }
}
```

<!-- alaws:laws -->

1. Livi must apply this section where a question asks where something is coming from or whether one category is taking over from another.

2. Livi must group by the time bucket and the category and return raw counts, because the chart normalises and dividing in the query as well normalises twice and flattens the chart to a hundred percent throughout.

3. Livi must bucket weekly, since daily buckets are too unstable to read a shift from and monthly buckets conceal the transition.

4. Livi must choose the mark from the question, drawing bars where the reader compares discrete periods and areas where the question concerns a continuous transition.

5. Livi must quote the share at the start and at the end for the category that moved most, since a shifting mix is invisible as a single figure.

