---
title: "Data Handling"
id: livi.general.data
---

<!-- alaws:commentary -->

These laws govern every query Livi writes, in every chapter. Most of them
exist because breaking them produces a chart that looks right and is
wrong — the most expensive kind of error in this system.

Rule 7 uses pg_trgm's `similarity()` alongside `ILIKE` for fuzzy repository-name matching; `0.3` is pg_trgm's own default similarity threshold.

<!-- alaws:laws -->

1. Every query you write must be scoped to organization {{org_id}}. {#every-query-you-write-must}

2. Write PostgreSQL. No other dialect is available. {#write-postgresql-no-other-dialect}

3. Read a record's completion timestamp where present and fall back to its creation timestamp otherwise, since not every record completes. `COALESCE(completed_at, created_at)` is the general activity time. {#read-record-completion-timestamp-where}

4. Take lines-of-code figures only from settled ledger rows (`loc_usage_ledger.status = 'accounted'`). Counting provisional rows lets unaccounted numbers leak into history. `loc_usage_ledger.operation_type` is always `diff_review`; filtering on `'review'` silently produces zero rows. {#take-lines-of-code-figures}

5. Exclude records with no author from any count of people. Such records are automation, and counting them invents a colleague who does not exist. {#exclude-records-with-no-author}

6. Use `reviews.repository` (a plain name column) directly — never join `reviews` to `repositories` through `pull_requests`, because `reviews.pull_request_id` is unpopulated for most reviews and that join silently drops rows. {#use-reviews-repository-plain-name}

7. Filter a named repository with `<table>.<column> ILIKE '%<name>%' OR similarity(<table>.<column>, '<name>') > 0.3`, whichever table's repository-name column the query is already using (`reviews.repository`, `repositories.name`, `repositories.full_name`, ...), ranked by `ORDER BY similarity(<table>.<column>, '<name>') DESC` inside that same row-level step (a CTE or subquery, before any `GROUP BY`). {#filter-to-a-named-repository}

8. Do not join `reviews` to `users` through a foreign key — the schema has none. Join them yourself on `users.email = reviews.user_email`. {#do-not-join-reviews-to}

9. Alias every selected expression with a unique name. `count(*) AS n`, not bare `count(*)`. Duplicate or unnamed columns are rejected. {#alias-every-selected-expression-with}

10. Bucket timestamps with `date_trunc('day' | 'week' | 'month' | 'quarter', created_at)` and do not use any other bucketing function. {#bucket-timestamps-with-date-trunc}

11. Use a single `SELECT` statement. No `INSERT`, `UPDATE`, `DELETE`, `WITH RECURSIVE`, `FOR UPDATE`, `SELECT *`, or bind parameters. {#use-single-select-statement-no}

12. Do not name a CTE after one of the tables in the schema reference. {#do-not-name-cte-after}

13. List columns by name — never `SELECT *` or `table.*`. {#list-columns-by-name-never}

14. Available SQL functions: `count sum avg min max stddev variance percentile_cont bool_or bool_and rank dense_rank row_number lag lead first_value last_value ntile round abs ceil floor trunc mod power sqrt coalesce nullif greatest least date_trunc date_part extract to_char to_date to_timestamp age now date_bin lower upper initcap trim btrim ltrim rtrim concat concat_ws substring length split_part replace left right lpad rpad jsonb_array_length jsonb_typeof generate_series unnest`. Nothing else. {#available-sql-functions-count-sum}

15. Compute period-over-period change in SQL with `lag()`, never by subtracting two numbers itself. {#compute-period-over-period-change}

16. `reviews.status` is one of `created`, `in_progress`, `completed`, `failed`. A question about work that actually finished means `status = 'completed'`. {#reviews-status-is-one-of}

17. `pull_requests.state` is one of `open`, `closed`, `merged`. {#pull-requests-state-is-one}

18. `review_feedback.vote_type` is one of `up`, `down`. {#review-feedback-vote-type-is}

19. `loc_usage_ledger.status` is one of `accounted`, `ignored`. `actor_kind` is one of `member`, `system`, `unknown`. {#loc-usage-ledger-status-is}

20. There is no single "how was this review triggered" column. `reviews.trigger_type` (`webhook` = PR/MR, `cli_diff` = pre-commit, `mcp` = MCP) and `loc_usage_ledger.trigger_source` (`api`, ...) are two different columns on two different tables. Check which table the metric lives on before picking which trigger column to group by. {#there-is-no-single-how}

21. When counting per day, fill empty days with zero. A missing row draws nothing, so a quiet week closes up and the trend flatters the team. {#when-counting-per-day-fill}

22. Account for one-to-many joins. Two such joins in one query multiply rows and inflate every count; measures must be counted distinctly or aggregated separately and joined afterwards. {#account-for-one-to-many}

23. Compute rolling averages, cumulative percentages, running totals and deltas in the query, so the chart plots columns that already exist. Round every such derived figure to two decimal places with `round()` — an unrounded float repeating to fifteen digits is noise in a tooltip and in the description. {#compute-rolling-averages-cumulative-percentages}

24. Keep presentation out of the query — normalising to a hundred percent, negating a value to sit below a zero line, and highlight bands are applied to the chart, not the data. {#keep-presentation-out-of-the}

25. `users` has no `org_id` column and no `is_active` column — `is_active` exists on `orgs` and on `auth_tokens`, not on `users`. To scope the `users` table to one org (e.g. to list every member, including those with zero reviews, for a "who has/hasn't adopted" question), join `user_roles` instead: `JOIN user_roles ON user_roles.user_id = users.id AND user_roles.org_id = {{org_id}}`. Filtering on `users.org_id` or `users.is_active` is rejected outright — the columns do not exist. {#users-has-no-org-id}

26. Filter a named person with `concat_ws(' ', users.first_name, users.last_name) ILIKE '%<name>%' OR similarity(concat_ws(' ', users.first_name, users.last_name), '<name>') > 0.3`, or match `users.email` directly when the name given looks like an email or username, ranked by `ORDER BY similarity(concat_ws(' ', users.first_name, users.last_name), '<name>') DESC` inside that same row-level step (a CTE or subquery, before any `GROUP BY`). `users` has no `name` or `full_name` column — only `first_name` and `last_name`. Join to `reviews` with `reviews.user_email = users.email` — there is no foreign key between them. {#filter-to-a-named-person}
