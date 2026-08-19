---
title: "Counting the Answer"
id: livi.planning.counting
---

<!-- alaws:commentary -->

**Why this stage exists at all.** A chart with 500 points on it is not a
chart anyone can read — it's a wall of pixels. Past that point the answer
should be a downloadable file instead, and the code enforces this as a
hard rule: if the count you write here comes back above 500, the finalize
stage's chart choice is overridden to CSV, no matter what it decides. That
happens in code, not by asking the model to judge "how complicated the
graph might look" — the number is real and fixed, currently 500
(`maxChartRows` in `internal/mcpagent/analytics.go`).

So `count_sql` has one job: predict, before any chart is drawn, how many
points the eventual chart or file will have. Get that number right and
the finalize stage automatically gets routed to the correct output.

**Worked example.** "Reviews per month, this year" will have around 12
points — one per month. The right `count_sql` is a query that returns the
number 12, not the number of individual reviews (which could be in the
hundreds or thousands):

```sql
SELECT count(*) AS n FROM (
  SELECT date_trunc('month', completed_at) AS month
  FROM reviews
  WHERE org_id = 42 AND status = 'completed'
  GROUP BY 1
) t
```

Get this wrong — count reviews instead of months — and a five-point
monthly trend can come back as "706", which is above the 500-row line and
silently downgrades a chart-shaped question into a CSV export nobody
asked for.

<!-- alaws:laws -->

1. Select the measure from the question's intent: review count for how often the tool is used, lines of code for how much work passes through it, people for reach and depth, duration for speed, feedback for trust. The word "velocity" means lines of code.

2. Generate the count query against the tables and columns dbctx supplied for the question, not tables or columns you were not given.

3. Write `count_sql` so that it returns exactly one row and exactly one column — a single number. A query that returns grouped rows is not a count query and will be rejected.

4. Count the same groups the answer will plot, one for one. If the answer is one point per month, count months; if one bar per repository, count repositories. Counting the underlying records instead is the most damaging mistake at this stage: `SELECT count(*) AS n FROM reviews WHERE org_id = 42` counts reviews, and a number in the hundreds pushes a five-point answer out of a chart and into a file export.

5. Count the rows the answer will have, not the rows scanned. Where the answer groups, wrap the grouping in an outer `SELECT count(*)` and count its output rows, as in `SELECT count(*) AS n FROM (SELECT date_trunc('day', completed_at) AS day FROM reviews WHERE org_id = 42 GROUP BY 1) t`, which counts days rather than reviews.

6. Plan a grouped answer even where the question reads as a single total, so that the number `count_sql` returns is greater than 1 — an answer of one row is a bare number with nothing to judge it against, which is never the right shape unless the user asked for one fixed value.
