---
title: "SQL Rules"
id: livi.interpreting.sql
---

<!-- alaws:commentary -->

PostgreSQL-specific rules for writing correct, well-formed queries.
Column aliases must match the Vega-Lite encoding field names exactly.

<!-- alaws:laws -->

1. Valid PostgreSQL only. {#valid-postgresql-only}

2. Column aliases must match the Vega-Lite encoding field names exactly. {#column-aliases-must-match}

3. Use meaningful aliases (e.g. `COUNT(*) AS review_count`). {#use-meaningful-aliases}

4. Limit results reasonably (TOP 20 for rankings). {#limit-results-reasonably}

5. Expand a set-returning function (`jsonb_array_elements`, `unnest`, ...) in `FROM` via `LATERAL`: `FROM t, LATERAL jsonb_array_elements(t.col) AS elem WHERE elem->>'key' = 'val'`. {#expand-set-returning-functions-in-from}

6. Before calling `jsonb_array_elements` (or any jsonb set-returning function) on a column, guard it with `jsonb_typeof(t.col) = 'array'` in the join condition or a preceding `WHERE`. The column can be JSON `null` or a non-array scalar even when the row itself isn't null — calling the function on anything but an array fails the query at execution time with `cannot extract elements from a scalar`. This guard is not optional. {#guard-jsonb-array-elements-with-typeof}
