# 21.

### Query: How much code has LR reviewed?

### Result from livi:
#### Lines of Code Reviewed by Month
The organization has reviewed a total of 568,230 lines of code since the start of the year.

Monthly activity peaked in May 2026 with 142,100 lines reviewed.
Time range: Last 5 months (Jan 2026 – May 2026)
Granularity: Monthly
Query: total lines of code reviewed per month

Downloaded graph:

![alt text](<Total Lines of Code Reviewed by Month__LiveReview__160826.png>)
What is missing from the demo:

**Symptom**: For this query Livi planned and rendered a **monthly bar chart**, not a daily horizon graph. Call-#2 plan JSON (request `3b958b64e4526177`): `{"analytics_plan":[{"id":"loc_reviewed_trend","question":"Total lines of code reviewed per month","count_sql":"SELECT count(*) FROM (SELECT date_trunc('month', accounted_at) FROM loc_usage_ledger WHERE org_id = 151 GROUP BY 1) t"}]}`. Finalized (call #3) as `report=loc_reviewed_trend type=chart rows=5 title="Lines of Code Reviewed by Month"`. The rendered `data.values` (5 rows, months Apr–Aug 2026): `2026-04: 487`, `2026-05: 14296`, `2026-06: 32873`, `2026-07: 107548`, `2026-08: 114573` (sum 269,777; max 114,573 in Aug 2026). `data_sql` filters `operation_type = 'diff_review'`. The chart is a `bar` mark with a dashed "Period average (baseline)" rule overlay. Note the description's quoted totals (568,230 total / 142,100 May peak) do **not** match the chart's own `data.values` (sum 269,777 / max Aug 114,573).

**Expected**: the spec in `LiveReview/scripts/adoption_chart/loc_horizon.html` — a **daily horizon graph over the last 90 days**, not a monthly bar. Its SQL groups by **day** over a 90-day window (`generate_series` of days LEFT JOIN daily `sum(billable_loc)` from `loc_usage_ledger WHERE org_id=151 AND status='accounted' AND accounted_at >= CURRENT_DATE - 90`, zero-filled via `COALESCE`), mark = horizon area (layered b1/b2/b3 bands, `height:90`), title "LOC reviewed (horizon) — hexmos-internal, last 90 days". KPI numbers on that page: **Total LOC reviewed: 356394**, **Peak day: 75864 LOC**. No `operation_type` filter.

**Root cause**: the **plan** stage (`internal/mcpagent/prompts/analytics_plan.md`) chose the wrong granularity/chart. The classifier correctly sent this to `count_query`, but the plan prompt has no rule for a volume-over-time question ("how much code has LR reviewed") that would force a daily 90-day grouping → horizon. Its only day-grouping rule (lines 38–48) is scoped to **rhythm/habit/consistency** questions ("are engineers actually incorporating reviews into their daily workflow"), which routes to a calendar heatmap — not this query. With no matching rule, plan defaulted to `date_trunc('month', ...)` grouping, yielding 5 monthly rows → a bar chart. The finalize description then also quoted KPI totals inconsistent with the chart's own `data.values`. 


### One line RCA
The plan prompt sets granularity only for rhythm questions and gives generic volume-trend questions no default, and the finalize step is forced to match the plan's count shape, so it fell back to a coarse monthly grouping.
# 22.

### Query: Are reviews getting faster?

### Result from livi:
#### Average Review Turnaround Time
This chart tracks the average duration between starting and completing a review each month for hexmos-internal.

The turnaround time is calculated as the interval between the started_at and completed_at timestamps.

Decreasing values indicate that reviews are becoming faster over time.
Time range: Last 7 months (Dec 2024 – Jun 2025)
Granularity: Monthly
Query: average review completion time per month

Downloaded graph:
![alt text](<Average Review Turnaround Time__LiveReview__160826.png>)

What is missing from the demo:

**Symptom**: For this query Livi planned and rendered a **monthly line chart of the *average* turnaround time**, not a weekly median + percentile-band chart. Call-#2 plan JSON (request `cbcc58008328dd96`): `{"analytics_plan":[{"id":"review_speed_trend","question":"Average time taken to complete reviews per month","count_sql":"SELECT count(*) AS n FROM (SELECT date_trunc('month', completed_at) AS month FROM reviews WHERE status = 'completed' AND org_id = 151 AND completed_at IS NOT NULL AND started_at IS NOT NULL GROUP BY 1) t"}]}`. Finalized (call #3) as `report=review_speed_trend type=chart rows=7 title="Average Review Turnaround Time"`. `data_sql` computes `avg(extract(epoch from (completed_at - started_at))/3600)` grouped **monthly** (`date_trunc('month', completed_at)`), and the rendered `data.values` (7 rows, Feb–Aug 2026) are the monthly means in hours: `Feb 0.0157, Mar 0.0102, Apr 0.0066, May 0.0059, Jun 0.0157, Jul 0.0156, Aug 0.0349` (≈ 0.94 / 0.61 / 0.39 / 0.35 / 0.94 / 0.94 / 2.1 minutes). The chart is a `line` mark with a dashed "Period average (baseline)" rule overlay. Note the `time_range` label (Dec 2024 – Jun 2025) does **not** match the rendered months (Feb–Aug 2026).

**Expected**: the spec in `LiveReview/scripts/adoption_chart/latency_bands.html` — a **weekly p10–p90 error band with a p50 median line over the last 90 days**, not a monthly average line. Its SQL groups by **week** (`date_trunc('week', completed_at)` over `completed_at >= CURRENT_DATE - 90`) and computes three percentiles via `percentile_cont` — `p50` (median), `p10`, and `p90` (in minutes) — from `completed_at - created_at`. Mark = layered `errorband` (p10–p90) plus a `line` (p50, `strokeWidth:2.5`, points). Title "Review duration — hexmos-internal, last 90 days (p10-p90 band, p50 line)". KPI on that page: **Median review duration went from 1.1 min to 3.2 min** across the window. The whole point is tracking the **tail** (p90), which a single mean hides.

**Root cause**: the **plan** stage (`internal/mcpagent/prompts/analytics_plan.md`) chose the wrong metric and granularity. The classifier correctly sent this to `count_query`, but the plan prompt has no rule for a **latency/tail** question ("are reviews getting faster" — a CTO cares about the tail, not the mean): its only day-grouping rule (lines 38–48) is scoped to rhythm/habit/consistency questions, and there is no rule anywhere steering toward percentiles/median or error bands. With no matching rule, plan defaulted to grouping by month and took the **average** (`count_sql` groups `date_trunc('month', completed_at)`), yielding a monthly-mean line chart; the finalize description also emitted a `time_range` inconsistent with the rendered months.


### One line RCA
Neither prompt instructs speed questions to use weekly p50/p10/p90; the plan framed it as a plain trend and finalize was bound to that shape, producing a monthly average and missing tail latency.
# 23.

### Query: How much engineering work is being covered by LR?

### Result from livi:
#### Weekly Billable Lines of Code
The organization processed a total of 16 weeks of billable engineering work.

Activity peaked at 1,420,000 lines of code in the week of May 11, 2026.

Recent volume shows a steady trend of approximately 950,000 lines per week.
Time range: Last 16 weeks (Mar 2026 – Jun 2026)
Granularity: Weekly
Query: weekly trend of billable lines of code processed by the organization

Downloaded graph:
![alt text](<Weekly Billable Lines of Code__LiveReview__160826.png>)

What is missing from the demo:

**Symptom**: For this query Livi planned and rendered a **weekly bar chart of billable LOC only**, not a dual-series (LOC + review count) layered chart. Call-#2 plan JSON (request `3cb92048e1f35c3b`): `{"analytics_plan":[{"id":"loc_coverage_trend","question":"Weekly trend of billable lines of code processed by the organization","count_sql":"SELECT count(*) AS n FROM (SELECT date_trunc('week', accounted_at) AS week FROM loc_usage_ledger WHERE org_id = 151 GROUP BY 1) t"}]}`. Finalized (call #3) as `report=loc_coverage_trend type=chart rows=16 title="Weekly Billable Lines of Code"`. `data_sql` computes `sum(billable_loc)` grouped **weekly** (`date_trunc('week', accounted_at)`) from `loc_usage_ledger` only, and the rendered `data.values` (16 rows, Apr–Aug 2026) are `[487, 545, 1075, 363, 12313, 2559, 8076, 10822, 14478, 13307, 12623, 27510, 28223, 73027, 45014, 113233]` (sum 363,655; max 113,233 in the week of Aug 10, 2026). The chart is a `bar` mark (weekly LOC) layered with a `4-week rolling average` line and a dashed `Period average (baseline)` rule. There is **no review-count series** anywhere. Note the description's numbers (peak 1,420,000 in the week of May 11; ≈950,000/week) do **not** match the rendered `data.values` (max 113,233; the data has no May 11 week and the week-of-May-18 value is 363).

**Expected**: the spec in `LiveReview/scripts/adoption_chart/loc_vs_reviews.html` — a **dual layered line over the last 90 days showing BOTH daily review count AND daily LOC**, not a weekly LOC-only bar. Its SQL joins a per-day `reviews_d` (`count(*)` grouped by `date_trunc('day', COALESCE(completed_at, created_at))` from `reviews WHERE org_id=151`) with a per-day `loc_d` (`sum(billable_loc)` from `loc_usage_ledger WHERE org_id=151 AND status='accounted'`), zero-filled via `generate_series` of days LEFT JOINs and `COALESCE`, one `reviews` and one `loc` value per day. Title "LOC vs review count — hexmos-internal, last 90 days". KPI on that page: **Average 810.0 LOC per review across 440 reviews / 356394 LOC total**. The point is to distinguish "more reviews" from "genuinely more code inspected" — two independent lines.

**Root cause**: the **plan** stage (`internal/mcpagent/prompts/analytics_plan.md`) chose the wrong scope. The classifier correctly sent this to `count_query`, but the plan prompt has no rule for a **dual-dimension coverage** question ("how much engineering work is covered" — needs both LOC *and* review count so the two can be compared). Nothing in the prompt steers toward two parallel time series (line 1 = LOC, line 2 = reviews), so plan produced a single entry counting only `loc_usage_ledger` rows grouped weekly, which forced the finalize stage into a LOC-only weekly bar. The finalize description then also quoted peak/steady-state numbers inconsistent with the chart's own `data.values`.


### One line RCA
No coverage rule requires comparing engineering volume with review activity, and the plan's count already fixed a single metric, so the plan reduced the question to one weekly LOC series instead of planning LOC + review count together.

# 24.

### Query: Are reviews becoming more iterative?

### Result from livi:
#### Review Iteration Trend
I calculated the average number of commits per review by day over the last 90 days.

The data shows how the intensity of code changes fluctuates within individual pull requests and reviews, helping to identify shifts in development patterns.

Each data point reflects the average commit count for all reviews initiated on that specific day.
Time range: Last 90 days
Granularity: Daily
Query: average number of commits per review over the last 90 days

Downloaded graph:
![alt text](<Review Iteration Trend__LiveReview__160826.png>)

What is missing from the demo:

**Symptom**: For this query Livi planned and rendered a **daily line chart of the average commits-per-review over 90 days**, not a distribution of reviews-per-commit. Call-#2 plan JSON (request `662c7769dd5a233c`): `{"analytics_plan":[{"id":"iterative_review_trend","question":"Average number of commits per review over the last 90 days to determine if reviews are becoming more iterative","count_sql":"SELECT count(*) AS n FROM (SELECT date_trunc('day', created_at) AS day FROM reviews WHERE org_id = 151 AND created_at >= now() - interval '90 days' GROUP BY 1) t"}]}`. Finalized (call #3) as `report=iterative_review_trend type=chart rows=66 title="Review Iteration Trend"`. `data_sql` computes `count(rc.id)::float / nullif(count(distinct r.id), 0)` (avg commits per review) grouped **daily** over 90 days, and the rendered `data.values` (66 rows, May–Aug 2026) are mostly `0` (only 5 nonzero days: `2026-08-09: 0.2857`, `2026-08-10: 0.6`, `2026-08-11: 2.4286`, `2026-08-12: 17.0667`, `2026-08-13: 7.5`). The chart is a `line` mark with a `7-day rolling average` line and a dashed `Period average (baseline)` rule.

**Expected**: the spec in `LiveReview/scripts/adoption_chart/reviews_per_commit.html` — a **histogram of reviews-per-commit (count of commits bucketed by how many times they were reviewed)**, not a time-series of average commits. Its SQL groups reviews by `commit_hash` and counts `reviews_per_commit` per commit, then buckets: `SELECT reviews_per_commit, count(*) AS commits FROM (SELECT commit_hash, count(*) AS reviews_per_commit FROM reviews WHERE org_id=151 AND commit_hash IS NOT NULL AND COALESCE(completed_at, created_at) >= CURRENT_DATE - 90 GROUP BY commit_hash) t GROUP BY 1 ORDER BY 1`. Mark = `bar`, x = `reviews_per_commit` (ordinal), y = `commits`. Title "Reviews per commit — hexmos-internal, last 90 days". KPI on that page: **67% of 3 commits were reviewed more than once**. The distribution (a long right tail) is what reveals iterativeness — an average hides it.

**Root cause**: the **plan** stage (`internal/mcpagent/prompts/analytics_plan.md`) chose the wrong chart shape. The classifier correctly sent this to `count_query`, but the plan prompt has no rule for a **distribution/dimensionality** question ("are reviews becoming more iterative" — answer is a histogram/violin/box of reviews-per-commit, not an average trend). Its only day-grouping rule (lines 38–48) is scoped to rhythm/habit/consistency questions, and nothing steers toward a histogram of a per-key count. With no matching rule, plan defaulted to a time-bucketed aggregation (`date_trunc('day', created_at)`), yielding an avg-commits-per-day line whose values are almost all zero and do not reveal iteration depth.


### One line RCA
The plan's rhythm rule (daily / 90-day) misfired on "iterative," locking a daily commits-per-review trend in count_sql before finalize could correct it — the shape was committed upstream, not mis-detected in finalize.
# 25.

### Query: Which engineers are getting the most value from LR?

### Result from livi:
Hi there! I'm Livi, and I'd love to help you identify which engineers are finding the most value in LiveReview.

Time range: —
Granularity: —
Query: —

Downloaded graph:
No file

What is missing from the demo:

**Symptom**: For this query Livi never ran a query or produced a chart at all — it answered conversationally. Request `f3e7cb5e6f46779e`: the **classify** call (`call=0 kind=classify`, at 21:45:03) returned `AI Error: ... request timeout: network operation exceeded timeout` after 11.09s (at 21:45:14). With no classifier answer, the pipeline **fell back to `shape=chat`** (`Branch Selected: shape=chat prompt_len=1713 tools=0`), so no `SQL Plan`, no count/data SQL, and no report were ever produced. The only output is the plain-prose final response: "Hi there! I'm Livi, and I'd love to help you identify which engineers are finding the most value in LiveReview." — a greeting that does not answer the question.

**Expected**: the spec in `LiveReview/scripts/adoption_chart/value_scatter.html` — a **2D scatterplot of engineers** (x = reviews, y = useful findings, size = LOC reviewed, color = feedback acceptance %), not a conversational reply. Its SQL groups `reviews` by `author_username` over the last **180 days**, LEFT JOINing `loc_usage_ledger` (accounted LOC) and `review_feedback` (up/down `vote_type`, `retracted_at IS NULL`), computing per-engineer `reviews`, `loc`, `up_votes`, `down_votes`. Mark = `circle`, x = `reviews`, y = `up_votes` (proxy for useful findings), size = `loc`, color = `acceptance`. Title "Value from LiveReview per engineer — hexmos-internal, last 180 days". KPI on that page: **3 of 8 engineers have received explicit feedback so far**.

**Root cause**: the **classify** stage (`internal/mcpagent/prompts/analytics_classify.md`) — specifically its **failure/timeout fallback**, not the prompt's routing rules. This question ("which engineers ... most value") squarely matches the `count_query` rule in `analytics_classify.md` (lines 15–22: counting/grouping/ranking across many records; the prompt even says a number without a chart is not acceptable, lines 37–43). The problem is the classifier call itself timed out at the network layer and the pipeline defaulted to `chat` (`Branch Selected: shape=chat`) instead of retrying `classify` or defaulting to `count_query` when the classifier is unavailable. The `chat` branch then has no tools/DB, so it can only produce a greeting. Because classify is the gate, its timeout silently dropped the request out of the analytics pipeline.

### One line RCA

 The classifier timed out and agent.go:122-129 deliberately degraded to the chat branch with no retry, so the query never reached planning/SQL — the fix is code (retry or default to the analytics path), not a prompt rule.



# 26.

### Query: Are people trusting the reviews?

### Result from livi:
#### Review Feedback Sentiment
The organization's team has provided feedback on reviews, with a clear preference for helpful contributions.

Positive 'up' votes account for the vast majority of interactions, indicating high trust in the reviews generated.

Negative 'down' votes remain a small fraction, highlighting where adjustments or further tuning may be beneficial.
Time range: All time
Granularity: Per vote type
Query: feedback sentiment distribution by vote type

Downloaded graph:
![alt text](<Review Feedback Sentiment__LiveReview__160826.png>)

What is missing from the demo:

**Symptom**: For this query Livi planned and rendered a **global up/down bar chart of all feedback**, not a diverging per-engineer chart. Request `2584570f2647fa9a` was correctly branched to `count_query`. Call-#2 plan JSON: `{"analytics_plan":[{"id":"trust_metrics","question":"Feedback sentiment and usage across review categories","count_sql":"SELECT count(*) AS n FROM (SELECT vote_type, count(*) AS feedback_count FROM review_feedback WHERE org_id = 151 GROUP BY vote_type) t"}]}`. Finalized (call #3) as `report=trust_metrics type=chart rows=2 title="Review Feedback Sentiment"`. `data_sql` counts `review_feedback` rows grouped **only by `vote_type`** (no engineer, no retracted filter), and the rendered `data.values` are just 2 rows: `down: 8`, `up: 7`. The chart is a `bar` mark (x = `vote_type`, y = `feedback_count`). This aggregates all engineers together and omits the per-engineer split that "who is trusting" implies.

**Expected**: the spec in `LiveReview/scripts/adoption_chart/feedback_trust.html` — a **diverging stacked bar per engineer** (up votes vs down votes), not a single global up/down pair. Its SQL joins `review_feedback f JOIN reviews r` grouping by `engineer` (`r.author_username`) and `vote_type`, filtered by `f.org_id=151`, `f.retracted_at IS NULL`, and `f.created_at >= CURRENT_DATE - 365 days`, with down votes rendered as **negative** (`down` → `n: -8`) so the chart diverges around 0. Mark = `bar`, y = `engineer`, x = feedback count (down <- 0 -> up), color = `vote_type` (up green / down red). Title "Review feedback — hexmos-internal, last 365 days". KPI on that page: **6 up-votes vs 8 down-votes (43% positive)**.

**Root cause**: the **plan** stage (`internal/mcpagent/prompts/analytics_plan.md`) chose the wrong scope/granularity. The classifier correctly branched to `count_query`, but the plan prompt has no rule for a **trust/divergence** question that should break feedback down **per engineer** (and across the up/down split) rather than collapsing to a single vote-type count. Its only grouping guidance (lines 38–48) is scoped to rhythm/habit questions. With no matching rule, plan produced a single entry grouping `review_feedback` by `vote_type` only (no engineer, no `retracted_at IS NULL` filter, no 365-day window), yielding a 2-row aggregate that both loses the per-engineer trust signal and — because it lacks the retracted/negative treatment — cannot render a diverging chart. The finalize description ("up votes account for the vast majority") also contradicts the rendered data (up 7 vs down 8).

### One line RCA
The plan prompt maps no trust/feedback question to an engineer-level up/down breakdown, so its count aggregated votes globally; the finalize diverging-bar/small-multiples guidance exists but can't recover a per-engineer signal the plan never counted.


# 27.

### Query: Which repositories have the highest review coverage?

### Result from livi:
#### Top Repositories by Review Count
The repositories are ranked by the total number of reviews completed.

The top repository leads with 58 reviews, followed by the rest in descending order.
Time range: All time
Granularity: Per repository
Query: Top repositories by review count

Downloaded graph:
![alt text](<Top Repositories by Review Count__LiveReview__160826.png>)

What is missing from the demo:

**Symptom**: For this query Livi planned and rendered a **bar chart of review count per repository**, not a coverage bubble chart. Request `26e16226834f5161` was correctly branched to `count_query`. Call-#2 plan JSON: `{"analytics_plan":[{"id":"r1","question":"Top repositories by review count","count_sql":"SELECT count(*) AS n FROM (SELECT repository FROM reviews WHERE org_id = 151 GROUP BY repository) t"}]}`. Finalized (call #3) as `report=r1 type=chart rows=58 title="Top Repositories by Review Count"`. `data_sql` counts `reviews` grouped **only by `repository`** (`count(*) ... GROUP BY repository ORDER BY review_count DESC`), and the rendered `data.values` are 58 rows of `{repository, review_count}` (LiveReview 200, git-lrc 116, freedevtools 83, ... down to many 1s). The chart is a `bar` mark (x = `repository` sorted by y, y = `review_count`). This is raw volume, not coverage — it never divides reviews by PRs, so it can't show how well each repo's PRs are actually covered.

**Expected**: the spec in `LiveReview/scripts/adoption_chart/repo_coverage.html` — a **bubble chart of reviews-per-PR coverage** (x = reviews/PRs, y = LOC reviewed, size = engineers, color = coverage), not a volume bar. Its SQL joins `repositories rp` with `pull_requests pr` (count DISTINCT prs) and `reviews r` (count DISTINCT reviews, engineers) plus `loc_usage_ledger` (accounted LOC), grouped per repo over the last **90 days** with `HAVING count(DISTINCT r.id) > 0`, computing `coverage = reviews / prs`. Mark = `circle`, x = `coverage`, y = `loc`, size = `engineers`, color = `coverage`. Title "Repository review coverage — hexmos-internal, last 90 days". KPI on that page: **Average coverage 2.92 reviews per PR across 2 repos with PR data** (LiveReview 3.56, git-lrc 2.28).

**Root cause**: the **plan** stage (`internal/mcpagent/prompts/analytics_plan.md`) chose the wrong metric. The classifier correctly branched to `count_query`, but the plan prompt has no rule for a **coverage/ratio** question that must join PRs and reviews to compute reviews-per-PR (and normalize by a time window), rather than counting reviews alone. With no matching rule, plan produced a single entry counting `reviews` grouped by `repository` only (`Top repositories by review count`), which reads the question as pure volume instead of coverage and yields a 58-row bar chart with no denominator, no PR join, and no 90-day window. The finalize description also misstates the top count (58) vs the rendered data (LiveReview 200). 

### One line RCA
No plan or finalize guidance covers ratio questions (eg: reviews ÷ PRs), so the plan planned no ratio and finalize was bound to that single-metric count shape.


# 28.

# 29.

### Query: How much of the organization's activity is covered by the top users?

### Result from livi:
#### Review Contribution by User
The top user accounts for 45% of all reviews, followed by the next most active contributors.

Combined, the top 8 users represent the entirety of the organization's review activity.
Time range: All-time
Granularity: Per user
Query: contribution share of total reviews by user

Downloaded graph:
![alt text](<Review Contribution by User__LiveReview__160826.png>)

What is missing from the demo:

**Symptom**: For this query Livi planned and rendered a **per-user percentage bar chart**, not a Pareto (cumulative line + bars). Request `d6eb6ac7082485fa` was correctly branched to `count_query`. Call-#2 plan JSON: `{"analytics_plan":[{"id":"user_contribution_distribution","question":"Percentage of total reviews contributed by each user","count_sql":"SELECT count(*) AS n FROM (SELECT author_username FROM reviews WHERE org_id = 151 AND author_username IS NOT NULL GROUP BY author_username) t"}]}`. Finalized (call #3) as `report=user_contribution_distribution type=chart rows=8 title="Review Contribution by User"`. `data_sql` computes per-user `pct_contribution = round(100.0 * review_count / total_reviews, 1)` (all-time, `author_username IS NOT NULL`), and the rendered `data.values` (8 rows) are: lince 53.6% (380), shrijith 19.2% (136), ganeshkumar6120 17.3% (123), "" 4.5% (32), joe 2.1% (15), RijulTP 1.8% (13), LinceMathew 1% (7), lovestaco 0.4% (3). The chart is a `bar` mark (x = `pct_contribution`, y = `author_username`). It shows individual percentages but no **cumulative** curve, so "how much do the top N cover" isn't answered at a glance. Note the description's "top user 45%" is wrong — the rendered data has lince at 53.6%.

**Expected**: the spec in `LiveReview/scripts/adoption_chart/engineer_pareto.html` — a **Pareto chart: bars of reviews per engineer plus a cumulative-% line on a secondary axis**, not a lone percentage bar. Its SQL counts reviews per `author_username` over the last **90 days** (`COALESCE(completed_at, created_at) >= CURRENT_DATE - 90`) ordered descending; the rendered values carry a derived `cum_pct` field (shrijith 30.9%, ganeshkumar6120 58.4%, lince 84.3%, ... 100%). Mark = layered `bar` (reviews, y-left) + `line` with points (cum_pct, y-right, `resolve: {scale: {y: independent}}`). Title "Adoption concentration — hexmos-internal, last 90 days". KPI on that page: **3 of 8 engineers account for 80% of all review activity** — the cumulative line is what makes that visible.

**Root cause**: the **plan** stage (`internal/mcpagent/prompts/analytics_plan.md`) chose the wrong chart shape. The classifier correctly branched to `count_query`, but the plan prompt has no rule for a **concentration/Pareto** question (how much activity the top users cover — needs a cumulative line over ranked engineers, plus a 90-day window), so it answered as a plain per-user share instead. With no matching rule, plan produced a single entry grouping reviews by `author_username` and expressing each as a percentage of the total, yielding an all-time bar chart with no cumulative curve and no time window. The finalize description also misquoted the top-user share (45%) vs the rendered data (lince 53.6%).
How much of the organization's activity is covered by the top users?

### One line RCA

The plan prompt lacks a concentration rule, so it never counted per-engineer rows; the finalize Pareto (cumulative-percent) row exists but is unreachable because the plan locked the count shape before finalize runs.


# 30.

### Query: What changed between week 1 and week 2?

### Result from livi:
#### Weekly Review Activity Comparison
Weekly review volume shows how activity fluctuates across the organization.

The data covers the last 26 weeks, highlighting trends in submission patterns.
Time range: Last 26 weeks (Jan 2025 – Jul 2025)
Granularity: Weekly
Query: weekly review counts for the last 26 weeks

Downloaded graph:
![alt text](<Weekly Review Activity Comparison__LiveReview__160826.png>)

What is missing from the demo:

**Symptom**: For this query Livi planned and rendered a **26-week weekly review-count bar chart**, not a two-period change matrix. Request `f242b541aaf65684` was correctly branched to `count_query`. Call-#2 plan JSON: `{"analytics_plan":[{"id":"reviews_weekly_comparison","question":"Weekly review activity comparison","count_sql":"SELECT count(*) AS n FROM (SELECT date_trunc('week', created_at) AS week FROM reviews WHERE org_id = 151 GROUP BY 1) t"}]}`. Finalized (call #3) as `report=reviews_weekly_comparison type=chart rows=25 title="Weekly Review Activity Comparison"`. `data_sql` counts `reviews` grouped **weekly** (`date_trunc('week', created_at)` over the last 26 weeks), and the rendered `data.values` (25 rows, Feb–Aug 2026) are per-week `review_count` values (e.g. 2026-02-09: 21, 2026-03-02: 46, 2026-07-27: 114, ...). The chart is a `bar` mark (x = week, y = `review_count`) with a `4-week rolling average` line and a dashed `Period average (baseline)` rule. There is **no W1-vs-W2 comparison** and no delta — it's just a trend over 26 weeks. Note the `time_range` label (Jan 2025 – Jul 2025) does **not** match the rendered weeks (Feb–Aug 2026).

**Expected**: the spec in `LiveReview/scripts/adoption_chart/change_matrix.html` — a **two-dimensional change matrix** (rows = metrics, columns = W1 / W2 / Delta, color = delta), not a single-metric trend. Its data compares **two periods — W1 (14 to 7 days ago) and W2 (7 days ago to now)** — across five metrics: Reviews, LOC reviewed, Active engineers, Repos active, Pre-commit %. Each metric has W1, W2, and a `Delta` (period-over-period % change) row; deltas are color-encoded around 0 (down red / hold grey / up green) via `scale: {domainMid: 0}`. Mark = layered `rect` heatmap + `text` labels. Title "7-day verdict — hexmos-internal". KPI on that page: **1 of 5 metrics improved or held steady period-over-period** (only LOC reviewed rose, +101.6%; Reviews −45.5%, Active engineers −16.7%, Repos active −20%, Pre-commit % −16.9%).

**Root cause**: the **plan** stage (`internal/mcpagent/prompts/analytics_plan.md`) chose the wrong chart shape and window. The classifier correctly branched to `count_query`, but the plan prompt has no rule for a **period-comparison / change-matrix** question ("what changed between week 1 and week 2" — needs two fixed periods compared across multiple metrics with deltas), so it treated it as a single long time series. With no matching rule, plan produced a single entry counting reviews per week over 26 weeks, yielding a weekly trend with no W1/W2 split, no delta, and no multi-metric rows. The finalize description also emitted a `time_range` label inconsistent with the rendered weeks.
What changed between week 1 and week 2?


### One line RCA

The plan prompt lacks a period-over-period rule, so its count never covered two fixed periods; the finalize slope-graph / change-matrix rows exist but can't help once the plan fixed the shape upstream.
