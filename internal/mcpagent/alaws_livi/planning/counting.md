---
title: "Counting the Answer"
id: livi.planning.counting
---

<!-- alaws:commentary -->

This stage construct the sql query for counting the number of answer rows for each query given 
with the help of dbctx output. Also explicitly mentioned that answe rows have to count not the rows count also
group count instead of each row single count.

<!-- alaws:laws -->

1. Generate the count query against the tables and columns dbctx supplied for the question, not tables or columns it was not given.

2. Write `count_sql` so that it returns exactly one row and exactly one column — a single number. A query that returns grouped rows is not a count query and will be rejected.

3. Count the rows the answer will have, not the rows scanned. Where the answer groups, wrap the grouping in an outer `SELECT count(*)` and count its output rows, as in `SELECT count(*) AS n FROM (SELECT date_trunc('day', completed_at) AS day FROM reviews WHERE org_id = 42 GROUP BY 1) t`, which counts days rather than reviews.

4. Plan a grouped answer even where the question reads as a single total, so that the number `count_sql` returns is greater than 1 — an answer of one row is a bare number with nothing to judge it against, which is never the right shape unless the user asked for one fixed value.
