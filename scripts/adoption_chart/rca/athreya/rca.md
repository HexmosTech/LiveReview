# 1.

Working

# 2.

Working

# 3.

### Query: How broadly has the organization adopted LiveReview?

### Result from livi:

Daily Review Completion Volume

[](Daily Review Completion Volume**LiveReview**160826.png)

The organization has completed reviews across 111 distinct days.

Activity peaked with the highest daily volume of reviews recorded during the tracked period.

This trend illustrates the consistent integration of LiveReview into developer workflows.

Time range: Entire history (inception to date)

Granularity: Daily

Query: daily count of reviews completed by the organization

---

Review Adoption over Time

[rendered graph](Review Adoption over Time**LiveReview**160826.png)

The organization has seen a total of 7 unique contributors complete at least one review.

This count represents the cumulative number of individual engineers who have successfully utilized LiveReview.

Time range: Last 7 months (Dec 2024 – Jun 2025)

Granularity: Monthly

Query: count of unique users who have completed at least one review, by month

### Downloaded graph:

### What is missing from the demo:

**Symptom** — For the query "How broadly has the organization adopted LiveReview?", Livi planned and rendered two time-series charts (chat_debug.log, request dd8a06177ca3916e):

- `Daily Review Adoption` — bar chart, reviews completed per day, 111 rows
- `Review Adoption over Time` — line chart, *cumulative* unique contributors per month, 7 rows

Both charts answer "how much / how many over time". Neither answers "how broadly" — i.e. whether usage is spread across the whole org or concentrated in a few engineers. (The call #2 plan JSON was `adoption_trend` + `unique_users` — both grouped by time.)

**Expected** (see adoption_breadth.html): ONE histogram + KPI overlay —

- SQL: completed reviews per engineer over the last 90 days, grouped by `author_username`, then bucketed into bands: `1-4 (light)`, `5-19 (regular)`, `20+ (heavy)`.
- Chart: `bar` mark; x = band (ordinal, fixed sort order), y = engineer count, color = band.
- Stats in description: engineers active (8), median reviews/engineer (31), top contributor's share (shrijith, 136 of 440 = 31%).

**Root cause** — the wrong decision is made in the *planning* call (call #2), not in chart rendering. `internal/mcpagent/prompts/analytics_plan.md` has a special routing rule for rhythm/habit/consistency questions ("group by day") but no rule for breadth/distribution/concentration questions. Faced with "how broadly adopted", the planner fell back to its default group-by-time bias and emitted two time-series reports. The finalize call (#3) does know the histogram shape ("distributed across many rows, as an aggregate shape → bar over SQL-computed bins" in `internal/mcpagent/prompts/analytics_finalize.md`), but it is contractually bound to the planned sub-question ("This report answers: ..."), so it never gets to choose a distribution.

# 4.

# 5.

# 6.

# 7.

# 8.

# 9.

# 10.
