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

6. **Hard rule, no exceptions: bucket every time-series query by day** — `date_trunc('day', ...)` — regardless of how long the window is. Never write `date_trunc('week' | 'month' | 'quarter' | 'year', ...)` for a trend/time-series interpretation, even for a 6-month or multi-year window. The frontend's own Day/Week/Month toggle re-buckets a daily series client-side; a query that pre-aggregates to week or month throws away the daily rows that toggle needs, and it can never recover them afterward. {#select-time-granularity}

7. Use `date_trunc` for time grouping, not `DATE()` or `to_char()`. The correct Postgres form is `date_trunc('day', created_at)` — see law 6: day is the only granularity a time-series query should ever bucket by. {#use-date-trunc}

8. Order results by a meaningful column — typically the numeric measure descending, or the time column ascending — so the chart's default sort is useful. {#order-results-by}

9. Never produce two interpretations of the same trend that differ only in time granularity — "Daily Review Volume Over Time" next to "Weekly Review Volume Trend" is the same chart twice, not two interpretations. The UI already carries its own Day/Week/Month toggle on a trend chart, so a second interpretation only earns its place by answering a genuinely different question (broken out by repository, by trigger type, as a rolling average, as a comparison against a target) — not by re-bucketing the same series law 3's "vary chart types" rule still applies underneath this: a second trend interpretation must differ in subject or measure, never only in bucket size. {#never-vary-granularity-only}
