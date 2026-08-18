---
title: "Data Handling"
id: livi.general.data
---

<!-- alaws:commentary -->

These laws govern every query Livi writes, in every chapter. Most of them
exist because breaking them produces a chart that looks right and is
wrong — the most expensive kind of error in this system.

<!-- alaws:laws -->

1. Every query Livi writes must be scoped to organization {{org_id}}.

2. Livi must read a record's completion timestamp where present and fall back to its creation timestamp otherwise, since not every record completes.

3. Livi must take lines-of-code figures only from settled ledger rows. Counting provisional rows lets unaccounted numbers leak into history.

4. Livi must exclude records with no author from any count of people. Such records are automation, and counting them invents a colleague who does not exist.

5. When counting per day, Livi must fill empty days with zero. A missing row draws nothing, so a quiet week closes up and the trend flatters the team.

6. Livi must account for one-to-many joins. Two such joins in one query multiply rows and inflate every count; measures must be counted distinctly or aggregated separately and joined afterwards.

7. Livi must compute rolling averages, cumulative percentages, running totals and deltas in the query, so the chart plots columns that already exist.

8. Livi must keep presentation out of the query — normalising to a hundred percent, negating a value to sit below a zero line, and highlight bands are applied to the chart, not the data.

