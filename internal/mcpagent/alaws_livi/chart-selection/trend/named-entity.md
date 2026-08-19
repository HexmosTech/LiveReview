---
title: "Trend for One Named Entity"
id: livi.charts.trend.named_entity
---

<!-- alaws:commentary -->

**Applies when** the question asks what happened to a single named
repository, engineer or team over time — a "what happened to X" question
rather than a whole-organization trend.

**Seen as:** "What happened to a repository's velocity?"

Vega-Lite shape — a highlight rectangle over the recent interval, a thin
raw line, a heavier rolling-average line:

```json
{
  "width": 800, "height": 340,
  "layer": [
    {"data": {"values": [{"start": "<highlight_start>", "end": "<highlight_end>"}]},
     "mark": {"type": "rect", "color": "#7c9cff", "opacity": 0.12},
     "encoding": {"x": {"field": "start", "type": "temporal"}, "x2": {"field": "end"}}},
    {"mark": {"type": "line", "color": "#3a4358", "strokeWidth": 1},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "loc", "type": "quantitative"}}},
    {"mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2.5},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "rolling_avg", "type": "quantitative"}}}
  ],
  "resolve": {"scale": {"y": "shared"}}
}
```

<!-- alaws:laws -->

1. Apply this section where a question asks what happened to a single named repository, engineer or team over time.

2. Confirm which entity is meant before querying, and must ask where the question implies one without naming it.

3. Measure lines of code summed per day for that entity rather than a record count, because velocity concerns how much code moved and one large review is not the same event as one trivial one.

4. Compute the rolling average in the query.

5. Mark the recent interval with a highlight band supplied as two dates, not drawn from the query.

6. Compare the highlighted interval against the interval preceding it in the description, because the line alone shows only that something happened and not what.

7. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Adapt the field names to those its own query produced:

```json
{
  "width": 800, "height": 340,
  "layer": [
    {"data": {"values": [{"start": "<highlight_start>", "end": "<highlight_end>"}]},
     "mark": {"type": "rect", "color": "#7c9cff", "opacity": 0.12},
     "encoding": {"x": {"field": "start", "type": "temporal"}, "x2": {"field": "end"}}},
    {"mark": {"type": "line", "color": "#3a4358", "strokeWidth": 1},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "loc", "type": "quantitative"}}},
    {"mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2.5},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "rolling_avg", "type": "quantitative"}}}
  ],
  "resolve": {"scale": {"y": "shared"}}
}
```
