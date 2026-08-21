---
title: "Interpretation Rules"
id: livi.interpreting.rules
---

<!-- alaws:commentary -->

Core rules for generating multiple interpretations of a single analytics
question. Each interpretation is an independent SQL query plus chart spec —
no dependencies between them, no shared state.

<!-- alaws:laws -->

1. Produce between 1 and 5 interpretations of the user's question. More interpretations are better when the question is broad or ambiguous; a narrow, specific question may warrant fewer. {#produce-between-1-and-5}

2. Every interpretation MUST include `org_id = {{org_id}}` in its WHERE clause (or join through a table that has org_id). A query without it will be rejected. {#every-interpretation-must}

3. Vary chart types across interpretations. If the first interpretation uses a bar chart, the second should use a line, pie, heatmap, or another type that fits the data shape. Never return two identical chart types. {#vary-chart-types-across}

4. Never return a single-number result. If the question seems to ask for one total, group it by something meaningful (month, repository, trigger_type) so the result has context. A number with nothing to compare it against is never the right shape unless the user explicitly asked for a single value. {#never-return-single-number}

5. For questions about counts or small sets (e.g. "how many repositories?"), return the actual items with their names, not just the count number. Use the name/title column, not just IDs. {#return-actual-items}

6. Select the time granularity that fits the question's implied window: day for the last month or less, week for 1-3 months, month for 3-12 months, year for multi-year. When uncertain, prefer day — downstream aggregation can coarsen it. {#select-time-granularity}

7. Use `date_trunc` for time grouping, not `DATE()` or `to_char()`. The correct Postgres form is `date_trunc('month', created_at)`. {#use-date-trunc}

8. Order results by a meaningful column — typically the numeric measure descending, or the time column ascending — so the chart's default sort is useful. {#order-results-by}
