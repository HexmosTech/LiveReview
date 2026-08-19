---
title: "Counting the Answer"
id: livi.planning.counting
---

<!-- alaws:commentary -->

`count_sql` answers one question: **how many rows will the answer have?**
It is not the query that produces the answer — that comes later.

So `count_sql` always returns exactly one number: one row, one column.
What must not be 1 is the *value* it returns, because an answer of one row
is a bare number with nothing to compare it against. A grouped answer
therefore wraps its grouping in an outer count:

```sql
SELECT count(*) AS n FROM (
  SELECT date_trunc('day', COALESCE(completed_at, created_at)) AS day
  FROM reviews
  WHERE org_id = 42 AND status = 'completed'
  GROUP BY 1
) t
```

That counts days, not reviews — which is what the chart will plot one
point per. Returning the grouped rows themselves is the most common
mistake here and fails immediately: the count phase expects a single
number and rejects anything else.

<!-- alaws:laws -->

1. Generate the count query against the tables and columns dbctx supplied for the question, not tables or columns you were not given.

2. Write `count_sql` so that it returns exactly one row and exactly one column — a single number. A query that returns grouped rows is not a count query and will be rejected.

3. Count the same groups the answer will plot, one for one. If the answer is one point per month, count months; if one bar per repository, count repositories. Counting the underlying records instead is the most damaging mistake at this stage: `SELECT count(*) AS n FROM reviews WHERE org_id = 42` counts reviews, and a number in the hundreds pushes a five-point answer out of a chart and into a file export.

4. Count the rows the answer will have, not the rows scanned. Where the answer groups, wrap the grouping in an outer `SELECT count(*)` and count its output rows, as in `SELECT count(*) AS n FROM (SELECT date_trunc('day', completed_at) AS day FROM reviews WHERE org_id = 42 GROUP BY 1) t`, which counts days rather than reviews.

5. Plan a grouped answer even where the question reads as a single total, so that the number `count_sql` returns is greater than 1 — an answer of one row is a bare number with nothing to judge it against, which is never the right shape unless the user asked for one fixed value.
