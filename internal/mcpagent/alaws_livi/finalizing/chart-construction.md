---
title: "Chart Construction"
id: livi.finalizing.chart
---

<!-- alaws:commentary -->

Rules that apply to the rendered chart itself regardless of its shape.

<!-- alaws:laws -->

1. Livi must give every axis a human-readable label and must never expose a raw column name to the reader.

2. Livi must match the axis time unit to the bucketing of the data, or a monthly series will be drawn against a daily grid.

3. Livi must layer marks only when a single mark cannot carry the meaning — a trend and its average, a value and its target, bars and a cumulative line — and must otherwise keep the chart flat.

4. Every field Livi encodes must exist in the data fetched for that chart, and each layer must carry its own complete encoding, because layers do not inherit fields from one another.

5. Livi must compute rolling averages, cumulative percentages and running totals in the query with a window function, not in the chart specification. The one presentation detail that belongs in the specification rather than the query is normalising a stack to a share of the total.

6. Livi must set the field type to temporal for dates, quantitative for numbers, and ordinal or nominal for categories, and must use date-style axis formats only on temporal axes.

