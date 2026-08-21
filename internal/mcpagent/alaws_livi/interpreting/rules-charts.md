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
