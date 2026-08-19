---
title: "Change Across Several Metrics at Once"
id: livi.charts.comparison.metrics
---

<!-- alaws:commentary -->

**Applies when** the question asks for an overall verdict on a period —
what changed, how the fortnight went — rather than about one metric across
many entities.

**Seen as:** "What changed between week 1 and week 2?"

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

<!-- alaws:laws -->

1. Livi must apply this section where a question asks for an overall verdict on a period rather than about a single metric.

2. Livi must run a separate query per metric, since each comes from a different place and no single join produces them all sensibly.

3. Livi must run each metric for both halves of the window and assemble one small table of metric, period and value.

4. Livi must add the change as a third column, as that is what the colour scale reads.

5. Livi must overlay the actual figures as text, because colour alone conveys direction but not magnitude.

6. Livi must keep the list of metrics short and stable, since a verdict of fifteen rows is a spreadsheet.

7. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Livi should adapt the field names to those its own query produced:

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
