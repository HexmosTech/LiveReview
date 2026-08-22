---
title: "Chart Type Reference"
id: livi.interpreting.chartref
---

<!-- alaws:commentary -->

Reference for the 13 supported chart types. Each entry describes when to
use it, what data shape it expects, and a minimal Vega-Lite encoding
example. The model should consult this when choosing chart_type and
encoding for each interpretation.

<!-- alaws:laws -->

1. **bar** — Comparing categories. Categorical x, numeric y. Example: reviews per trigger_type. Default encoding: `x: {field: "category", type: "nominal", sort: "-y"}, y: {field: "count", type: "quantitative"}`. {#bar-chart}

2. **grouped_bar** — Comparing categories across two dimensions. Category x, group color, numeric y. Example: reviews per trigger_type split by month. Encoding: `x: {field: "category"}, y: {field: "count"}, color: {field: "group"}, xOffset: {field: "group"}`. {#grouped-bar}

3. **stacked_bar** — Composition of categories. Category x, subcategory color, numeric y. Example: review status breakdown per month. Encoding: `x: {field: "category"}, y: {field: "count"}, color: {field: "subcategory"}`. {#stacked-bar}

4. **line** — Trends over time. Temporal x, numeric y. Example: reviews per day (see `livi.interpreting.data-quality` law 2 - always day granularity, never pre-aggregated to week/month). Encoding: `x: {field: "day", type: "temporal", timeUnit: "yearmonthdate"}, y: {field: "count", type: "quantitative"}`. {#line-chart}

5. **multi_line** — Comparing trends across groups. Temporal x, numeric y, group color. Example: reviews per day per trigger_type. Encoding: same as line plus `color: {field: "group"}`. {#multi-line}

6. **area** — Volume/magnitude over time. Temporal x, numeric y. Example: cumulative reviews. Same encoding as line but with `mark: "area"`. {#area-chart}

7. **stacked_area** — Composition over time. Temporal x, numeric y, group color. Example: trigger type mix over time. Encoding: same as area plus `color: {field: "group"}`. {#stacked-area}

8. **scatter** — Relationship between two numeric variables. Numeric x, numeric y. Example: reviews vs LOC consumed. Encoding: `x: {field: "x_field"}, y: {field: "y_field"}`. {#scatter-plot}

9. **pie** — Proportional composition (few categories). Category color, numeric theta. Example: plan type distribution. Encoding: `theta: {field: "value"}, color: {field: "category"}`. Use only when there are fewer than 8 categories. {#pie-chart}

10. **heatmap** — Density across two dimensions. Ordinal x, ordinal y, numeric color. Example: activity by day-of-week and hour. Encoding: `x: {field: "x"}, y: {field: "y"}, color: {field: "value", scale: {scheme: "blues"}}`. {#heatmap}

11. **horizontal_bar** — Long category names or rankings. Category y, numeric x. Example: top reviewers. Encoding: `y: {field: "name", sort: "-x"}, x: {field: "count"}`. {#horizontal-bar}

12. **boxplot** — Distribution spread across categories. Category x, numeric y. Example: LOC distribution per trigger_type. Encoding: `x: {field: "category"}, y: {field: "value"}`. Mark: `{type: "boxplot", extent: 1.5}`. {#boxplot}

13. **trellis_bar** — Small multiples across groups. Facet field, category x, numeric y. Example: reviews per trigger_type per provider. Encoding: `facet: {field: "group", columns: 3}, spec: {mark: "bar", encoding: {x: {field: "category"}, y: {field: "count"}}}`. {#trellis-bar}
