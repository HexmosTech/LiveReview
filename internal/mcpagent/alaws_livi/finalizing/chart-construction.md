---
title: "Chart Construction"
id: livi.finalizing.chart
---

<!-- alaws:commentary -->

Rules that apply to the rendered chart itself regardless of its shape.
Field-type assignment (`temporal`/`quantitative`/`ordinal`/`nominal`) and
the allowed marks live in `livi.charts.basics` (Chart Grammar), not here —
this section covers what to do with a field once its type is already
fixed.

<!-- alaws:laws -->

1. Encode the measure the question's intent calls for: review count for how often the tool is used, lines of code for how much work passes through it, people for reach and depth, duration for speed, feedback for trust. The word "velocity" means lines of code — do not substitute review count for it. {#encode-the-measure-the-question}

2. Give every axis a human-readable label and never expose a raw column name to the reader. {#give-every-axis-human-readable}

3. Match the axis time unit to the bucketing of the data, or a monthly series will be drawn against a daily grid. {#match-the-axis-time-unit}

4. Layer marks only when a single mark cannot carry the meaning — a trend and its average, a value and its target, bars and a cumulative line — and otherwise keep the chart flat. {#layer-marks-only-when-single}

5. Every field you encode must exist in the data fetched for that chart, and each layer must carry its own complete encoding, because layers do not inherit fields from one another. {#every-field-you-encode-must}

6. Compute rolling averages, cumulative percentages and running totals in the query with a SQL window function, not with Vega-Lite's own window transform in the chart specification. The one presentation detail that belongs in the specification rather than the query is normalising a stack to a share of the total — the same rule `composition/shift.md` relies on for its `"stack": "normalize"` encoding. {#compute-rolling-averages-cumulative-percentages}

7. Use date-style axis formats only on temporal axes. {#use-date-style-axis-formats}
