# §6 — Period-over-period comparison

> §0 applies in full. Only deviations from it are stated here.

## §6.0 Governing rule

**When a question asks whether something is gaining or losing, went up or
down, or changed between two points in time — collapse the window into
exactly two buckets and compare them directly.** A multi-month trend
answers a different question and forces the reader to eyeball a direction
the chart should state outright.

Label each row Previous or Current with a conditional on its date, then
group by that label alongside whatever is being compared. Split the window
down the middle so both halves are the same length and the comparison is
fair.

---

## §6.1 — Direction of change across many entities

**Applies when** the question asks which entities are rising or falling —
which repos are speeding up, which teams are slowing down.

1. Apply §6.0's two-bucket split, grouped by entity and period.
2. **Derive a direction label per entity** — gain, loss, or flat — by
   comparing its two values. This is the step that turns a tangle of lines
   into an answer.
3. Draw one line per entity between the two x points, coloured by that
   direction.
4. In the description, count them: how many gained, how many lost, out of
   how many tracked. Name the biggest movers.

**Seen as:** query #6 — "Which repositories are gaining or losing
engineering velocity?"

Vega-Lite spec:
```json
{
  "width": 500, "height": 380,
  "mark": {"type": "line", "point": true, "strokeWidth": 2.5},
  "encoding": {
    "x": {"field": "period", "type": "nominal", "sort": ["Previous", "Current"]},
    "y": {"field": "loc", "type": "quantitative"},
    "color": {"field": "trend", "type": "nominal",
              "scale": {"domain": ["gain", "flat", "loss"], "range": ["#39d353", "#8b949e", "#ff5c7c"]}},
    "detail": {"field": "repository", "type": "nominal"}
  }
}
```

---

## §6.2 — Change across several metrics at once

**Applies when** the question asks for an overall verdict on a period —
what changed, how did the fortnight go — rather than about one metric
across many entities.

1. **Run several small queries, not one.** Each metric comes from a
   different place, and there is no single join that produces all of them
   sensibly.
2. Run each metric for both halves of the window, then assemble one small
   table of metric, period and value rows.
3. Add the delta as a third period column. That is what the colour scale
   reads.
4. Overlay the actual numbers as text. A colour alone tells the reader the
   direction but not the size.
5. Keep the metric list short and stable. A verdict with fifteen rows is a
   spreadsheet.

**Seen as:** query #30 — "What changed between week 1 and week 2?"

Vega-Lite spec:
```json
{
  "width": 300, "height": 220,
  "layer": [
    {"mark": {"type": "rect"},
     "encoding": {
       "x": {"field": "period", "type": "nominal", "sort": ["W1", "W2", "Delta"]},
       "y": {"field": "metric", "type": "nominal", "sort": "<metric_order>"},
       "color": {"condition": {"test": "datum.is_delta", "field": "value", "type": "quantitative",
                                "scale": {"domainMid": 0, "range": ["#ff5c7c", "#232a3d", "#39d353"]}},
                 "value": "#161b22"}
     }},
    {"mark": {"type": "text", "color": "#e6ebf5", "fontSize": 12},
     "encoding": {
       "x": {"field": "period", "type": "nominal", "sort": ["W1", "W2", "Delta"]},
       "y": {"field": "metric", "type": "nominal", "sort": "<metric_order>"},
       "text": {"field": "value", "type": "quantitative", "format": ".1f"}
     }}
  ]
}
```

---

## §6.3 — Exception: explaining a change rather than showing it

**Applies when** the question asks *why* something changed, not whether it
did.

**This overrides §6.1.** Showing that velocity moved does not explain it —
§6.1 already established the movement. The answer is the change broken
down by whoever caused it.

1. Apply §6.0's two-bucket split, but grouped by contributor and filtered
   to the single entity in question.
2. **Subtract**: one row per contributor carrying their delta and its
   direction. The chart plots the change, not the two raw values.
3. Sort by the delta so the biggest movers sit at the ends, where the eye
   goes first.
4. In the description, turn the aggregate into attribution — "the drop is
   almost entirely one person" is the answer the question wanted.

**Seen as:** query #11 — "Why did this repository's velocity change?"

Vega-Lite spec:
```json
{
  "width": 600, "height": "<max(200, 30 * n_engineers)>",
  "mark": {"type": "bar"},
  "encoding": {
    "y": {"field": "engineer", "type": "nominal", "sort": "x"},
    "x": {"field": "delta", "type": "quantitative"},
    "color": {"field": "direction", "type": "nominal", "legend": null,
              "scale": {"domain": ["up", "down"], "range": ["#39d353", "#ff5c7c"]}}
  }
}
```
