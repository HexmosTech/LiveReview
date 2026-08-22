---
title: "Chart Selection Rules"
id: livi.interpreting.chart-rules
---

<!-- alaws:commentary -->

Rules for choosing chart types and building Vega-Lite specs. Each
interpretation carries its own chart type — the model must vary types
across interpretations and never repeat the same type.

<!-- alaws:laws -->

1. Pick the chart type whose `use_when` best matches the data shape. {#pick-the-chart-type}

2. Vary chart types across interpretations — never use the same chart type twice in one response. {#vary-chart-types-across}

3. The `vega_lite_spec` must be a complete valid Vega-Lite spec. {#vega-lite-spec-must-be}

4. Use `DATA_PLACEHOLDER` as the value of `data.values`. {#use-data-placeholder}

5. Field names in encoding must match SQL column aliases exactly. {#field-names-in-encoding}

6. For temporal fields, valid composite `timeUnit`s are: `yearmonthdate`, `yearmonth`, `yearweek`, `yearquarter`, `year`. {#valid-composite-time-units}

7. A question about how _broadly_ or _widely_ something has been adopted ("how broadly has the org adopted LiveReview", "is usage broad-based or is it three people doing everything") is a question about people too, one of the interpretation MUST be a per-engineer breadth histogram, bucketing each engineer into an adoption band (`0 reviews`, `1-10 (light)`, `11-20 (regular)`, `21+ (heavy)`) and charting engineer count per band (`bar`, x = band nominal sorted by the band order given here, y = engineer count, color = band, matching field to field so the color legend and x-axis agree). Compute the per-engineer count in a subquery, then bucket and count engineers in the outer query — a single flat `GROUP BY` cannot bucket by a value it is itself still aggregating. {#breadth-question-is-about-people}
