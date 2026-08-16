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

### What is missing from the demo:

**Symptom** — For the query "How broadly has the organization adopted LiveReview?", Livi planned and rendered two time-series charts (chat_debug.log, request dd8a06177ca3916e):

- `Daily Review Adoption` — bar chart, reviews completed per day, 111 rows
- `Review Adoption over Time` — line chart, _cumulative_ unique contributors per month, 7 rows

Both charts answer "how much / how many over time". Neither answers "how broadly" — i.e. whether usage is spread across the whole org or concentrated in a few engineers. (The call #2 plan JSON was `adoption_trend` + `unique_users` — both grouped by time.)

**Expected** (see adoption_breadth.html): ONE histogram + KPI overlay —

- SQL: completed reviews per engineer over the last 90 days, grouped by `author_username`, then bucketed into bands: `1-4 (light)`, `5-19 (regular)`, `20+ (heavy)`.
- Chart: `bar` mark; x = band (ordinal, fixed sort order), y = engineer count, color = band.
- Stats in description: engineers active (8), median reviews/engineer (31), top contributor's share (shrijith, 136 of 440 = 31%).

**Root cause** — the wrong decision is made in the _planning_ call (call #2), not in chart rendering. `internal/mcpagent/prompts/analytics_plan.md` has a special routing rule for rhythm/habit/consistency questions ("group by day") but no rule for breadth/distribution/concentration questions. Faced with "how broadly adopted", the planner fell back to its default group-by-time bias and emitted two time-series reports. The finalize call (#3) does know the histogram shape ("distributed across many rows, as an aggregate shape → bar over SQL-computed bins" in `internal/mcpagent/prompts/analytics_finalize.md`), but it is contractually bound to the planned sub-question ("This report answers: ..."), so it never gets to choose a distribution.

# 4.

### Query: Who has adopted LiveReview—and who hasn't?

### Result from livi:

Reviewer Activity Rankings

[](Reviewer Activity Rankings**LiveReview**160826.png)

The organization's top contributor has completed 42 reviews, while the least active reviewer has completed 1.

Activity is measured by the total count of completed reviews per author.

Time range: All time

Granularity: Per contributor

Query: who has reviewed the most pull requests

---

Review Productivity per Contributor

[](Review Productivity per Contributor**LiveReview**160826.png)

The chart displays the number of completed reviews by each contributor in hexmos-internal.

Athreya leads with 15 reviews, while the least active contributors have completed 1 review each.

Time range: All time

Granularity: Per contributor

Query: count of completed reviews per contributor in hexmos-internal

### What is missing from the demo:

**Symptom** — For "Who has adopted LiveReview the most and least?", Livi planned and rendered two separate bar charts (chat_debug.log, request d6cb8598fb73d768, call #2 plan at line 787):

```
{"analytics_plan": [
  {"id": "most_active_reviewers", "question": "Who has reviewed the most pull requests?",
   "count_sql": "SELECT count(*) AS n FROM (SELECT author_username FROM reviews WHERE org_id = 151 AND status = 'completed' GROUP BY author_username) t"},
  {"id": "least_active_reviewers", "question": "Who has reviewed the fewest pull requests?",
   "count_sql": "SELECT count(*) AS n FROM (SELECT author_username FROM reviews WHERE org_id = 151 AND status = 'completed' GROUP BY author_username) t"}
]}
```

- `Reviewer Activity Rankings` — `bar` mark, `most_active_reviewers`, data_sql `ORDER BY review_count DESC`, 8 rows (line 809/811/812).
- `Review Productivity per Contributor` — `bar` mark, `least_active_reviewers`, data_sql `ORDER BY review_count ASC`, 8 rows (line 840/842/843).

Both charts' Vega-Lite encoding hardcodes `"sort": "-x"` on the y-field (line 844), so despite the opposite `ORDER BY` in their SQL, they render as the same descending ranking of the same 8 rows (`lince` 334, `shrijith` 127, `ganeshkumar6120` 121, `""` 10, `joe` 15, `RijulTP` 8, `LinceMathew` 7, `lovestaco` 3) — visually duplicate charts, distinguished only by title/description text. Neither query includes engineers with zero reviews (both `GROUP BY author_username` on `reviews` alone, no join against a full roster), so the "who hasn't" half of the question has no data behind it at all.

**Expected** (see adoption_leaderboard.html) — ONE sorted horizontal bar with adoption-tier coloring and a target rule:

- SQL: `SELECT author_username, count(*) AS value FROM reviews WHERE org_id = 151 AND author_username IS NOT NULL AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '90 days' GROUP BY 1 ORDER BY 2 DESC` — a 90-day window, unlike the log's all-time/no-window query.
- Bands: `0 reviews` (#3a4358), `1-4 (light)` (#7c9cff), `5-19 (regular)` (#ffb454), `20+ (heavy)` (#39d353).
- Chart: two-layer spec — `bar` mark (y = engineer sorted `-x`, x = value, color = band) + a dashed `rule` layer at x = 5 (the target).
- KPI text: "1 of 8 engineers are below the target of 5 reviews."

**Root cause** — the wrong decision is made in the planning call (call #2), not in chart rendering. `internal/mcpagent/prompts/analytics_plan.md` instructs the model: _"One entry per distinct thing the user asked for. 'Show me reviews per month and my top reviewers' is **two** entries."_ That rule is correct for genuinely independent asks, but the planner over-applies it here: "most and least" is one ranking question with two ends, not two independent sub-questions, yet the plan produced two mirrored `PlanEntry` objects (`most_active_reviewers` / `least_active_reviewers`) with near-identical `count_sql`. Because finalize (call #3) is "contractually bound to the planned sub-question" per report (same mechanism noted in section 3's root cause), each finalize call only ever sees one half of the ranking and has no opportunity to produce the single banded-and-targeted chart `analytics_finalize.md`'s own chart-shape table already documents for this pattern ("comparing named categories against each other → sorted bar ... add a rule layer for a fixed target threshold") — that row exists but nothing in `analytics_plan.md` routes a compound most/least question to it as one report instead of two.

# 5.

### Query: Is adoption becoming broader over time?

### Result from livi:

Weekly Unique Reviewers

[](Weekly Unique Reviewers**LiveReview**160826.png)

The number of unique users performing reviews has grown from 2 in early 2026 to 12 in the most recent week.

Activity across the organization shows a consistent upward trend in participation over the last 6 months.

Time range: Last 6 months (Jan 2026 – Jun 2026)

Granularity: Weekly

Query: unique users performing reviews per week

### What is missing from the demo:

**Symptom** — For "Is adoption becoming broader over time?", Livi planned and rendered one report (chat_debug.log, request 864afedfb14e75b0, call #2 plan at line 947):

```
{"analytics_plan": [
  {"id": "adoption_trend",
   "question": "Number of unique users performing reviews per week over the last 6 months",
   "count_sql": "SELECT count(*) FROM (SELECT date_trunc('week', created_at) AS week FROM reviews WHERE org_id = 151 AND status = 'completed' GROUP BY 1) t"}
]}
```

`Weekly Unique Reviewers` — 3-layer `line` chart (raw count + 4-week rolling average + period-average baseline, the standard auto-injected rolling-average pattern for weekly data), data_sql `count(DISTINCT user_email)` grouped by `date_trunc('week', created_at)`, 26 rows (line 966/968/969). This measures **headcount** — how many distinct people touched a review each week — not **breadth of usage across activity tiers**, so it cannot show whether growth is many people doing a little or the same few people doing more. It also doesn't answer "broader" in the sense of light/regular/heavy segmentation at all: there is no tier field anywhere in this report.

The rendered description is also inconsistent with its own `data.values` (line 970): it claims growth "from 2 in early 2026 to 12 in the most recent week," but the actual series starts at `unique_users: 1` (2026-02-02) and the last point is `unique_users: 5` (2026-08-10) — the true max across all 26 weeks is 5, not 12, and never reaches 2 until the week of 2026-05-25.

**Expected** (see adoption_growth.html) — a stacked area chart of active-engineer counts broken out by usage tier, so growth in headcount and growth in engagement depth are visually separable:

- SQL: `SELECT date_trunc('week', COALESCE(completed_at, created_at))::date AS week, author_username, count(*) AS n FROM reviews WHERE org_id = 151 AND author_username IS NOT NULL AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '180 days'` — per-engineer weekly counts, then bucketed into tiers in a second pass (not a single flat GROUP BY).
- Bands: `1-4 (light)` (#7c9cff), `5-19 (regular)` (#ffb454), `20+ (heavy)` (#39d353).
- Chart: `area` mark (`interpolate: monotone`), x = week, y = engineer count (`stack: true`), color = tier.
- Stats: "Total active engineers/week went from 1 to 5. Heavy users (20+ reviews/week) went from 0 to 0."

**Root cause** — the wrong decision is made in the planning call (call #2), not in chart rendering. `internal/mcpagent/prompts/analytics_plan.md` has a rule for rhythm/habit questions ("group by day, not by author") but no equivalent rule distinguishing a _breadth_ question ("is adoption spreading across more people at more depth") from a plain headcount trend. The only applicable rule here is the generic "Default to a grouped answer... Write count_sql as if the answer will be grouped by time (or by whatever dimension makes the comparison)" — the planner picked time (week) as the sole grouping dimension and `count(DISTINCT user_email)` as the metric, which satisfies "grouped by time" literally but drops the per-engineer tier dimension entirely. Because finalize (call #3) only ever sees this report's own question ("Number of unique users performing reviews per week"), it has no basis to introduce tiers that were never in the plan — it just applies the standard time-trend chart-shape rule (line + rolling average) from `analytics_finalize.md`, which is the correct rule for the plan it was given, not for the question that was actually asked.

# 6.

# 7.

# 8.

# 9.

# 10.
