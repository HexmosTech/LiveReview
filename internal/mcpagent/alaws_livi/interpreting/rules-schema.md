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

5. `loc_usage_ledger.status` is one of `accounted` or `ignored` only — never `'processed'`, `'complete'`, or any other value, all of which silently match zero rows. Take lines-of-code figures only from `status = 'accounted'` rows. `loc_usage_ledger.actor_kind` is one of `member`, `system`, or `unknown` only — never `'user'`. `loc_usage_ledger.operation_type` is always `diff_review`; filtering on `'review'` also silently produces zero rows. {#loc-usage-ledger-enum-values}

6. `users` has no `org_id` column and no `is_active` column. To scope `users` to one org, join `user_roles` instead: `JOIN user_roles ON user_roles.user_id = users.id AND user_roles.org_id = {{org_id}}`. {#users-has-no-org-id}
