---
title: "Data Handling"
id: livi.general.data
---

<!-- alaws:commentary -->

These laws govern every query Livi writes, in every chapter. Most of them
exist because breaking them produces a chart that looks right and is
wrong — the most expensive kind of error in this system.

<!-- alaws:laws -->

1. Every query you write must be scoped to organization {{org_id}}.

2. Write PostgreSQL. No other dialect is available.

3. Read a record's completion timestamp where present and fall back to its creation timestamp otherwise, since not every record completes. `COALESCE(completed_at, created_at)` is the general activity time.

4. Take lines-of-code figures only from settled ledger rows (`loc_usage_ledger.status = 'accounted'`). Counting provisional rows lets unaccounted numbers leak into history. `loc_usage_ledger.operation_type` is always `diff_review`; filtering on `'review'` silently produces zero rows.

5. Exclude records with no author from any count of people. Such records are automation, and counting them invents a colleague who does not exist.

6. Use `reviews.repository` (a plain name column) directly — never join `reviews` to `repositories` through `pull_requests`, because `reviews.pull_request_id` is unpopulated for most reviews and that join silently drops rows.

7. Do not join `reviews` to `users` through a foreign key — the schema has none. Join them yourself on `users.email = reviews.user_email`.

8. Alias every selected expression with a unique name. `count(*) AS n`, not bare `count(*)`. Duplicate or unnamed columns are rejected.

9. Bucket timestamps with `date_trunc('day' | 'week' | 'month' | 'quarter', created_at)` and do not use any other bucketing function.

10. Use a single `SELECT` statement. No `INSERT`, `UPDATE`, `DELETE`, `WITH RECURSIVE`, `FOR UPDATE`, `SELECT *`, or bind parameters.

11. Do not name a CTE after one of the tables in the schema reference.

12. List columns by name — never `SELECT *` or `table.*`.

13. Available SQL functions: `count sum avg min max stddev variance percentile_cont bool_or bool_and rank dense_rank row_number lag lead first_value last_value ntile round abs ceil floor trunc mod power sqrt coalesce nullif greatest least date_trunc date_part extract to_char to_date to_timestamp age now date_bin lower upper initcap trim btrim ltrim rtrim concat concat_ws substring length split_part replace left right lpad rpad jsonb_array_length jsonb_typeof generate_series unnest`. Nothing else.

14. Compute period-over-period change in SQL with `lag()`, never by subtracting two numbers itself.

15. `reviews.status` is one of `created`, `in_progress`, `completed`, `failed`. A question about work that actually finished means `status = 'completed'`.

16. `pull_requests.state` is one of `open`, `closed`, `merged`.

17. `review_feedback.vote_type` is one of `up`, `down`.

18. `loc_usage_ledger.status` is one of `accounted`, `ignored`. `actor_kind` is one of `member`, `system`, `unknown`.

19. There is no single "how was this review triggered" column. `reviews.trigger_type` (`webhook` = PR/MR, `cli_diff` = pre-commit, `mcp` = MCP) and `loc_usage_ledger.trigger_source` (`api`, ...) are two different columns on two different tables. Check which table the metric lives on before picking which trigger column to group by.

20. When counting per day, fill empty days with zero. A missing row draws nothing, so a quiet week closes up and the trend flatters the team.

21. Account for one-to-many joins. Two such joins in one query multiply rows and inflate every count; measures must be counted distinctly or aggregated separately and joined afterwards.

22. Compute rolling averages, cumulative percentages, running totals and deltas in the query, so the chart plots columns that already exist.

23. Keep presentation out of the query — normalising to a hundred percent, negating a value to sit below a zero line, and highlight bands are applied to the chart, not the data.

