---
title: "Output Format"
id: livi.interpreting.output
---

<!-- alaws:commentary -->

The response is parsed by code, not read by a person. The shape below is
exact: a `query` string, an `interpretation` summary, and an
`interpretations` array holding up to five entries — nothing else in the
object.

Each interpretation carries its own SQL, chart type, title, description,
and optional Vega-Lite encoding overrides. The Go code executes the SQL,
builds the chart from the type + encoding, and presents all non-empty
results to the user.

<!-- alaws:laws -->

1. Reply with a single JSON object holding `query`, `interpretation`, and the `interpretations` array, and nothing else — no tool call, no markdown fence, no prose before or after. {#reply-with-single-json-object}

2. Give every interpretation a `sql`, `chart_type`, `title`, and `description`. The `encoding` field is optional — omit it to use the chart type's default encoding. {#give-every-interpretation}

3. The `sql` field must be a valid PostgreSQL SELECT statement. It must NOT contain INSERT, UPDATE, DELETE, DROP, ALTER, CREATE, TRUNCATE, or any other DDL/DML. {#sql-must-be-select}

4. The `chart_type` must be one of: bar, grouped_bar, stacked_bar, line, multi_line, area, stacked_area, scatter, pie, heatmap, horizontal_bar, boxplot, trellis_bar. {#chart-type-must-be}

5. The `title` should be a short, human-readable label for the chart (e.g. "Reviews per Month", "Top Contributors by LOC"). {#title-should-be}

6. The `description` should be 1-2 sentences explaining what the chart shows and any notable patterns. {#description-should-be}

7. When providing `encoding`, use Vega-Lite encoding channel names: `x`, `y`, `color`, `theta`, `size`, `tooltip`, etc. Each channel should have `field` (column name from SQL) and `type` (nominal, ordinal, quantitative, temporal). {#when-providing-encoding}

8. The `interpretation` field is a 1-2 sentence restatement of what the overall answer covers. It is not shown to the user. {#interpretation-field}

9. Begin your reply with the `{` character and end it with the matching `}`, with no text of any kind before or after. {#begin-reply-with-brace}
