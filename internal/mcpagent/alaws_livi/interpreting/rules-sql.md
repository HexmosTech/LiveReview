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
