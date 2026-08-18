# §6 — Period-over-period comparison

> §0 applies in full. Only deviations from it are stated here.

## §6.0 General rule

**When the question asks whether something is gaining or losing, went up
or down, or changed between two points in time — collapse the window into
exactly two buckets and compare them directly.** A multi-month trend
answers a different question ("how has this moved continuously") and
forces the reader to eyeball a direction the chart should be stating
outright.

Label each row Previous or Current with a conditional on its date, then
group by that label alongside the entity. Split the window down the
middle so both halves are the same length and the comparison is fair.

## §6.1 "Which repositories are gaining or losing engineering velocity?" (query #6)

Data: `loc_usage_ledger` joined to `reviews`, grouped by repository and
period. Derive a gain/loss/flat label per repository by comparing its two
values — that label is what turns a tangle of lines into an answer.

Vega-Lite spec — one line per repository between two x points, coloured by direction:
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

## §6.2 "What changed between week 1 and week 2?" (query #30)

Several metrics at once rather than one metric across many entities.

Data: **several small queries, not one big one.** Review counts from
`reviews`, LOC from `loc_usage_ledger`, active engineers as a distinct
author count, repositories touched — there is no single join that produces
all of them sensibly. Run each for both halves, then assemble one small
table of metric / period / value rows, with the delta as a third period
column. Keep the metric list short: a verdict with fifteen rows is a
spreadsheet.

Vega-Lite spec — delta-coloured cells with the numbers overlaid:
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

## §6.3 Exception — "Why did this repository's velocity change?" (query #11)

**Exception to §6.1.** "Why" is not answered by showing *that* velocity
changed — §6.1 already does that. It needs the change decomposed per
contributor.

Data: same two-period split, but grouped by author and filtered to one
repository, then subtracted — one row per engineer carrying their delta
and its direction. Sort by delta so the biggest movers sit at the ends.

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
