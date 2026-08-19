---
title: "Chart Construction"
id: livi.finalizing.chart
---

<!-- alaws:commentary -->

Rules that apply to the rendered chart itself regardless of its shape.

<!-- alaws:laws -->

1. Give every axis a human-readable label and never expose a raw column name to the reader.

2. Match the axis time unit to the bucketing of the data, or a monthly series will be drawn against a daily grid.

3. Layer marks only when a single mark cannot carry the meaning — a trend and its average, a value and its target, bars and a cumulative line — and otherwise keep the chart flat.

4. Every field you encode must exist in the data fetched for that chart, and each layer must carry its own complete encoding, because layers do not inherit fields from one another.

5. Compute rolling averages, cumulative percentages and running totals in the query with a window function, not in the chart specification. The one presentation detail that belongs in the specification rather than the query is normalising a stack to a share of the total.

6. Set the field type to temporal for dates, quantitative for numbers, and ordinal or nominal for categories, and use date-style axis formats only on temporal axes.

