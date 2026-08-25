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

7. "Critical or high blast radius" (or "risky reviews", "high risk reviews") means exactly `blast_radius_hunks.tier = 'blast-radius-high'` — a single `=` comparison against a single literal, never `IN (...)`. The word "critical" in the user's question is NOT a second tier to add alongside "high" — there is no `'blast-radius-critical'` value and never has been; "critical" and "high" in that phrase are two English words for the same one tier, not two different tiers to union together. Writing `tier IN ('blast-radius-high', 'blast-radius-critical')` is just as wrong as `tier IN ('blast-radius-high', 'blast-radius-none')` — both add a second literal that either doesn't exist or means the opposite of what was asked. Never write `tier IN (...)` at all for this phrase; use `tier = 'blast-radius-high'`. The four valid `tier` values (for reference only, not for this filter) are `blast-radius-none`, `blast-radius-low`, `blast-radius-medium`, `blast-radius-high` — bare words like `'critical'`, `'high'`, `'low'`, or invented values like `'blast-radius-critical'`, are never valid and silently match zero rows. A review's overall blast radius is `MAX(combined)`/its tier across its hunks: `JOIN blast_radius_hunks ON blast_radius_hunks.review_id = reviews.id`, `GROUP BY reviews.id` before filtering or ranking by tier. {#blast-radius-hunks-tier-enum-values}
