---
title: "Data Quality Rules"
id: livi.interpreting.data-quality
---

<!-- alaws:commentary -->

Critical rules for producing useful, comparable data. A single-number
answer is never acceptable — every query must return enough rows for the
user to see patterns, trends, or comparisons.

<!-- alaws:laws -->

1. NEVER return single-number results. Every query must return multiple rows with a dimension (time, category, name) for comparison. A query returning 1 row is a failure. {#never-return-single-number}

2. For time series: always fetch at DAY granularity — `DATE_TRUNC('day', created_at) AS day` — with no exceptions, regardless of how long the window is. The frontend chart carries its own Day/Week/Month toggle that re-buckets a daily series client-side; the backend does not re-aggregate it for you. Do NOT aggregate by month, week, or quarter yourself, even for a multi-month or multi-year window. {#for-time-series-always}

3. For small result sets (under 10 items), return the actual items (names, labels, details) not just counts. Example: instead of `COUNT(repositories) = 2`, return each repository's `name, provider, created_at`. {#for-small-result-sets}

4. Prefer queries that reveal patterns: rankings, trends, distributions, comparisons. Avoid flat counts. {#prefer-queries-that}

5. A question that only asks to list or enumerate entities ("what repositories exist", "list our users") has no quantitative measure to chart at all. Pick a real measure that makes the list comparable instead of listing entities bare - e.g. "repositories in the org" becomes reviews (or LOC) per repository, "our users" becomes reviews per user. Never encode the same field as both `x` and `y` to force a name-only list into a bar chart - a chart with `y: {field: "repository_name", type: "quantitative"}` over a text column is nonsense, not a chart, since that field holds no number to plot. {#listing-question-has-no-measure}
