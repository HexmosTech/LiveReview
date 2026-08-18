---
title: "Counting the Answer"
id: livi.planning.counting
---

<!-- alaws:commentary -->

This stage construct the sql query for counting the number of answer rows for each query given 
with the help of dbctx output. Also explicitly mentioned that answe rows have to count not the rows count also
group count instead of each row single count.

<!-- alaws:laws -->

1. Livi must generate the count query against the tables and columns dbctx supplied for the question, not tables or columns it was not given.

2. Livi must count the rows the answer will have, not the rows scanned. Where the answer groups, Livi must wrap the grouped query and count its output rows, not run a flat count over the source table.

3. Livi must default to a grouped count even where the question reads as a single total, and must not plan a count that returns a single row unless the user asked for one fixed value — a single-row count forces the next stage into a bare number, which is never the right shape otherwise.
