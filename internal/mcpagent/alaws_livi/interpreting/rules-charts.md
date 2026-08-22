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

8. A question about *concentration* — "where is X concentrated", "who accounts for most of the total", "is this the same few people doing everything" — is never answered by a plain sorted bar alone; a sorted bar shows rank but not how much of the whole the top few actually hold. One interpretation MUST use the `pareto` chart type: `bar` for the raw per-entity value plus a `line` layer of the running cumulative percent, both against the same entity ordering (sorted descending by value), matching the `pareto` entry in the chart types reference exactly. Compute the cumulative percent in SQL with a window function - `sum(value) OVER (ORDER BY value DESC) / sum(value) OVER () * 100 AS cum_pct` - never in the chart spec. {#concentration-question-needs-pareto}

<<<<<<< HEAD
9. A dbctx sample showing one dominant value for a field (e.g. `Category` sampling as just `review`) doesn't mean the field is low-cardinality — it means the sample missed the rest; that field can hold dozens of real values (`Security`, `Correctness`, ...). Group by it directly instead of avoiding it or filtering on a guessed value. {#thin-sample-is-not-low-cardinality}

10. When the user specifies which dimension belongs on which axis (e.g. "x axis = users, y axis = reviews"), honor that exactly in the `vega_lite_spec` encoding — even if it means swapping the template's default x/y mapping. {#user-axis-preference-overrides-defaults}
=======
9. **AXIS OVERRIDE — HARD RULE:** When the user explicitly specifies which dimension belongs on which axis (e.g. "users on x axis, reviews on y axis"), you MUST honor that specification exactly. Do NOT copy a chart_types.json template verbatim if its axes conflict with the user's request. Specifically: if the user says "x axis = category, y axis = numeric", your `vega_lite_spec` encoding MUST have `x` as `nominal`/`ordinal` and `y` as `quantitative` — even if you selected `horizontal_bar` as the chart type. Swap the template's x and y encodings to match the user's stated preference. The user's explicit axis instruction always wins over a template's default orientation. {#user-axis-preference-overrides-defaults}
>>>>>>> dd9632cd (fix(livi): make axis override rules more explicit and forceful)
