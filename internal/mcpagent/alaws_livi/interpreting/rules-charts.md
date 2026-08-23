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

8. A question about _concentration_ — "where is X concentrated", "who accounts for most of the total", "is this the same few people doing everything" — is never answered by a plain sorted bar alone; a sorted bar shows rank but not how much of the whole the top few actually hold. One interpretation MUST use the `pareto` chart type: `bar` for the raw per-entity value plus a `line` layer of the running cumulative percent, both against the same entity ordering (sorted descending by value), matching the `pareto` entry in the chart types reference exactly. The `sql` field itself MUST select the cumulative-percent column with a window function - `sum(value) OVER (ORDER BY value DESC) / sum(value) OVER () * 100 AS cum_pct` - never compute it only in the chart spec. If the `sql` you write doesn't return `cum_pct` (or whatever you name it), the `line` layer has nothing to plot and the chart silently renders as a bare bar chart with no cumulative line - so before finalizing a `pareto` interpretation, check that its `sql`'s `SELECT` list actually contains the window-function column the `vega_lite_spec`'s line layer references by name. Because that SQL already multiplies by 100, `cum_pct` is a 0-100 number, not a 0-1 fraction - reference it in any `tooltip`/axis with a plain numeric `format` (e.g. `".1f"`) and put the `%` sign in the field's `title` instead (`"Cumulative %"`). Do NOT format it with `".1%"` - that format type itself multiplies by 100 again, turning 44.8 into "4480.9%". `.1%` is only correct for a field that is genuinely a 0-1 fraction, e.g. law 11's `pct_share` below. {#concentration-question-needs-pareto}

9. A dbctx sample showing one dominant value for a field (e.g. `Category` sampling as just `review`) doesn't mean the field is low-cardinality — it means the sample missed the rest; that field can hold dozens of real values (`Security`, `Correctness`, ...). Group by it directly instead of avoiding it or filtering on a guessed value. {#thin-sample-is-not-low-cardinality}

10. When the user specifies which dimension belongs on which axis (e.g. "x axis = users, y axis = reviews"), honor that exactly in the `vega_lite_spec` encoding — even if it means swapping the template's default x/y mapping. {#user-axis-preference-overrides-defaults}

11. While showing a share of the total in a tooltip, add a `transform` to compute it: `{"joinaggregate": [{"op": "sum", "field": "<value>", "as": "total"}]}` then `{"calculate": "datum.<value> / datum.total", "as": "pct_share"}`, and reference `pct_share` in the tooltip with `format: ".1%"`. `pct_share` built this way is a 0-1 fraction, which is exactly what `.1%` expects (it multiplies by 100 itself) - do not also multiply by 100 in the `calculate` expression, and never apply `.1%` to a percent SQL already computed as 0-100 (see law 8's `cum_pct`). {#compute-tooltip-share-with-transform}

12. When the user has NOT specified axis assignments, choose the orientation that maximizes readability. Put the category field on the y-axis (horizontal bars) whenever the labels are longer text — names, emails, IDs, addresses, or any string longer than ~8 characters. Short labels (month names, status codes, single words) can go on either axis. The goal: labels should be readable without angled rotation or truncation. {#readability-default-orientation}
