---
title: "Output Format"
id: livi.interpreting.output
---

<!-- alaws:commentary -->

The response is parsed by code, not read by a person. The shape below is
exact: a `query` string, an `interpretation` summary, and an
`interpretations` array holding up to five entries — nothing else in the
object.

Each interpretation carries its own SQL, chart type, name, description,
Vega-Lite spec, and applied laws list. The Go code executes the SQL,
builds the chart from the spec, and presents all non-empty results to
the user.

<!-- alaws:laws -->

1. Reply with ONLY valid JSON, no markdown, no code fences. {#reply-with-only-valid-json}

2. Top-level schema: `query` (original user query string), `interpretation` (1-2 sentence restatement of what the overall answer covers), `interpretations` array. Nothing else in the object. {#top-level-schema-query-interpretation}

3. Each interpretation must have: `name` (short label), `description` (what this shows), `chart_type` (chart type ID from the reference), `sql` (PostgreSQL query), `vega_lite_spec` (complete Vega-Lite spec), `applied_laws` (array of canonical law numbers). {#each-interpretation-must-have}

4. The `vega_lite_spec` must use `DATA_PLACEHOLDER` as the value of `data.values`. The Go code replaces it with real query results. {#vega-lite-spec-data-placeholder}

5. The `applied_laws` array lists the canonical number of every law in this chapter that this interpretation actually used. Do not pad with laws that did not affect the reply. {#applied-laws-array}

6. The `sql` field must be a valid PostgreSQL SELECT statement. It must NOT contain INSERT, UPDATE, DELETE, DROP, ALTER, CREATE, TRUNCATE, or any other DDL/DML. {#sql-must-be-select-only}

7. Begin your reply with the `{` character and end it with the matching `}`, with no text of any kind before or after. {#begin-reply-with-brace}
