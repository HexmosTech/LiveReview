### What the schema reference above will not tell you

Representative values aren't shown in the table reference above, on purpose
(see dbctx_schema_plan.md if you're curious why) — the fixed, safe ones are
spelled out here instead:

- `reviews.status` is one of `created`, `in_progress`, `completed`, `failed`.
  A question about work that actually finished means `status = 'completed'`.
- `pull_requests.state` is one of `open`, `closed`, `merged`.
- `review_feedback.vote_type` is one of `up`, `down`.
- `author_username` / `author_name` (on `reviews` and `pull_requests`) identify
  the person the review or PR belongs to. "Top reviewers", "reviews per
  engineer" and "who reviewed most" all group by `author_username`.
- Use `completed_at` for when work finished, `created_at` for when it started.
  `COALESCE(completed_at, started_at, created_at)` is the general "activity
  time" if the question does not distinguish.
- `users` has no foreign key to `reviews` in the schema — join them yourself
  on `users.email = reviews.user_email`.

### Rules

- **Alias every selected expression with a unique name.** `count(*) AS n`, not
  bare `count(*)`. Duplicate or unnamed columns are rejected.
- Timestamps are `timestamptz`. Bucket with
  `date_trunc('day' | 'week' | 'month' | 'quarter', created_at)`.
- Available functions: `count sum avg min max stddev variance percentile_cont
  bool_or bool_and rank dense_rank row_number lag lead first_value last_value
  ntile round abs ceil floor trunc mod power sqrt coalesce nullif greatest least
  date_trunc date_part extract to_char to_date to_timestamp age now date_bin
  lower upper initcap trim btrim ltrim rtrim concat concat_ws substring length
  split_part replace left right lpad rpad jsonb_array_length jsonb_typeof
  generate_series unnest`. Nothing else is available.
- One `SELECT` statement. No `INSERT`, `UPDATE`, `DELETE`, `WITH RECURSIVE`,
  `FOR UPDATE`, or bind parameters like `$1`.
- Do not name a CTE after one of the tables above.
- Compute period-over-period change **in SQL** with `lag()`, never by
  subtracting two numbers yourself.

### Worked examples

Reviews completed per month:

```sql
SELECT date_trunc('month', completed_at) AS month, count(*) AS review_count
FROM reviews
WHERE status = 'completed'
GROUP BY 1
ORDER BY 1
```

Top reviewers:

```sql
SELECT author_username, count(*) AS review_count
FROM reviews
WHERE status = 'completed'
GROUP BY 1
ORDER BY review_count DESC
LIMIT 10
```

Month-over-month percentage change — note the arithmetic is the database's job:

```sql
WITH monthly AS (
  SELECT date_trunc('month', completed_at) AS month, count(*) AS review_count
  FROM reviews WHERE status = 'completed' GROUP BY 1
)
SELECT month,
       review_count,
       lag(review_count) OVER (ORDER BY month) AS previous_count,
       round(100.0 * (review_count - lag(review_count) OVER (ORDER BY month))
             / NULLIF(lag(review_count) OVER (ORDER BY month), 0), 1) AS pct_change
FROM monthly
ORDER BY month
```
