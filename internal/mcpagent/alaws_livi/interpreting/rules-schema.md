---
title: "Schema Rules"
id: livi.interpreting.schema
---

<!-- alaws:commentary -->

Rules for using the dbctx schema context correctly. The schema context
provides real tables, columns, foreign keys, and sample values — the model
must never invent columns that aren't there.

<!-- alaws:laws -->

1. Use ONLY tables and fields from the dbctx context. Never invent columns. {#use-only-tables-and-fields}

2. Every field must be table-qualified (e.g. `reviews.status`). {#every-field-must-be}

3. Include filters where relevant (e.g. `status = 'completed'`). {#include-filters-where}

4. `reviews` has no `user_id` column. Identify or join on `reviews.user_email` instead for any per-user grouping, filtering, or join. {#reviews-has-no-user-id}
