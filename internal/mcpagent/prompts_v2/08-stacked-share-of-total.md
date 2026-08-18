# §8 — Share of total over time

> §0 applies in full. Only deviations from it are stated here.

## §8.0 General rule

**When the question asks how the mix is shifting — which category's share
is growing — stack it and normalise to 100%.** A raw stacked count
conflates "this category grew" with "everything grew"; normalising
isolates the shift, which is what is being asked about.

Return raw counts from the query. The chart normalises; dividing in the
query as well normalises twice and flattens everything to 100%.

Bucket weekly: daily is too jittery to read a mix shift from, monthly
hides the transition.

## §8.1 "Where are reviews happening?" (query #14)

Discrete periods compared side by side — more useful than a pie chart,
which can only show one moment.

Data: `reviews` grouped by week and by the trigger-type column.

Vega-Lite spec:
```json
{
  "width": 700, "height": 340,
  "mark": {"type": "bar"},
  "encoding": {
    "x": {"field": "week", "type": "temporal"},
    "y": {"field": "n", "type": "quantitative", "stack": "normalize", "axis": {"format": "%"}},
    "color": {"field": "trigger_type", "type": "nominal"}
  }
}
```

## §8.2 "Are we moving review earlier in the development lifecycle?" (query #15)

Same data as §8.1; only the mark changes to `area`, because this question
is about a continuous transition rather than a period-by-period
comparison.

Vega-Lite spec:
```json
{
  "width": 700, "height": 340,
  "mark": {"type": "area", "interpolate": "monotone"},
  "encoding": {
    "x": {"field": "week", "type": "temporal"},
    "y": {"field": "n", "type": "quantitative", "stack": "normalize", "axis": {"format": "%"}},
    "color": {"field": "trigger_type", "type": "nominal"}
  }
}
```
