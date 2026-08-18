---
id: chart.oneoff
number: 10
title: One-off Shapes (No Shared Mechanism)
---

# §10 — One-off shapes

## §10.0 General rule

**These queries don't share a chart mechanism with each other or with
§1–§9 — each gets its own specific rule directly, with no general rule
above it beyond "match the named technique to the question, don't default
to `bar`."** Grouping them here is deliberate: it stops future additions
from being forced into an ill-fitting §1–§9 bucket just to avoid an
"uncategorized" pile — a one-off is a legitimate outcome, not a filing
failure.

## §10.1 Specific rule — "Is adoption becoming broader over time?" (query #5)

- Shape: stacked `area` (true stack, not normalized like §8) of active
  engineer counts, colored by adoption tier (same bands as §3.1/§4.1).
  Closest relative is §8, but the question is "how many people at each
  depth" (absolute headcount by tier), not "what fraction of total volume"
  — so it stays unnormalized.

Where the data lives:

- **Table:** `reviews`, grouped by **week and author together** — you need
  each person's activity level within each week before you can say which
  tier they were in that week.
- **Then bucket, then count people.** Two steps: assign each
  engineer-week to a tier using the same thresholds as §3.1 and §4.1, then
  count how many engineers fall in each tier per week. The chart plots
  people, not reviews.
- Skipping the per-author grouping and going straight to distinct-user
  counts per week is the failure this rule exists to prevent: it produces
  a headcount line that cannot show depth at all.
- Trailing ~180 days, since this is a question about a slow-moving shift.

Vega-Lite spec:
```json
{
  "width": 800, "height": 380,
  "mark": {"type": "area", "interpolate": "monotone", "line": {"strokeWidth": 1.5}},
  "encoding": {
    "x": {"field": "week", "type": "temporal"},
    "y": {"field": "engineers", "type": "quantitative", "stack": true},
    "color": {"field": "band", "type": "nominal", "sort": "<band_order>",
              "scale": {"domain": "<band_order>", "range": "<color_range>"}},
    "order": {"field": "band", "sort": "ascending"}
  }
}
```

## §10.2 Specific rule — "What does each engineer actually spend their review activity on?" (query #13)

- Shape: stacked `bar`, x = engineer, color = repository — shows whether
  an engineer concentrates on one repo or spreads across many.

Where the data lives:

- **Table:** `reviews`, grouped by author and repository together — one
  row per person per repo they touched.
- Authored reviews only; trailing 90 days.
- **Watch the repository count.** An org with dozens of repos produces an
  unreadable stack of colours. Keep the top few per engineer and roll the
  rest into an "other" bucket rather than emitting every one.

Vega-Lite spec:
```json
{
  "width": 700, "height": 340,
  "mark": {"type": "bar"},
  "encoding": {
    "x": {"field": "engineer", "type": "nominal", "sort": "-y"},
    "y": {"field": "reviews", "type": "quantitative", "stack": true},
    "color": {"field": "repository", "type": "nominal"}
  }
}
```

## §10.3 Specific rule — "How much does LiveReview save versus alternatives?" (query #20)

- Shape: waterfall — stacked `bar` with a computed invisible base (`y`)
  and visible delta (`y2`) per step, color = positive/negative.

Where the data lives:

- **Table:** `loc_usage_ledger` — it carries both the review volume and
  the actual LLM cost per settled row. Two totals for the window is all
  the database needs to give you.
- **Everything else on this chart is an assumption, not data.** The
  "alternative cost" you are comparing against comes from a stated
  assumption (engineer-hours saved × a rate), and the subscription cost is
  a known constant. Neither is in the schema.
- **Because of that, state your assumptions in the description.** A
  savings figure whose inputs are invisible is not persuasive to the
  person who has to defend it — quote the rate and the hours you assumed
  so they can argue with the number rather than distrust it.
- The invisible base segment that positions each waterfall bar is
  arithmetic on those totals, done before the chart.

Vega-Lite spec:
```json
{
  "width": 600, "height": 380,
  "mark": {"type": "bar", "size": 60},
  "encoding": {
    "x": {"field": "label", "type": "nominal", "sort": null, "axis": {"labelAngle": -20}},
    "y": {"field": "base", "type": "quantitative"},
    "y2": {"field": "top"},
    "color": {"field": "color", "type": "nominal", "legend": null,
              "scale": {"domain": ["positive", "negative"], "range": ["#39d353", "#ff5c7c"]}}
  }
}
```

## §10.4 Specific rule — "How much code has LR reviewed?" (long-span, compact view) (query #21)

- Shape: horizon graph — 2-3 `area` layers of the SAME field, each clamped
  to a band via SQL/app logic, stacked with rising opacity so intensity
  reads at a glance without a tall, sparse line. Reach for this only when
  the span is long (30-90+ points) and vertical space is scarce — §1's
  plain line+rolling-average is the default for shorter spans.

Where the data lives:

- **Table:** `loc_usage_ledger` alone — no join needed, since the org
  filter and the LOC figure both live on it.
- Daily sums, settled rows only, zero-filled across the full date series
  exactly as in §1.1.
- **The band-splitting is arithmetic on top of the daily value**, done
  after the query: pick a band height, then derive one column per band
  holding that band's share of the day's value. Three columns, three area
  layers. The query itself stays a plain daily series.

Vega-Lite spec (3 area layers, same field, rising opacity, band-clamped y-domain):
```json
{
  "width": 800, "height": 90,
  "layer": [
    {"mark": {"type": "area", "color": "#7c9cff", "opacity": 0.35, "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "b1", "type": "quantitative", "scale": {"domain": [0, "<band>"]}}}},
    {"mark": {"type": "area", "color": "#7c9cff", "opacity": 0.6, "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "b2", "type": "quantitative", "scale": {"domain": [0, "<band>"]}}}},
    {"mark": {"type": "area", "color": "#7c9cff", "opacity": 1.0, "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "b3", "type": "quantitative", "scale": {"domain": [0, "<band>"]}}}}
  ],
  "resolve": {"scale": {"y": "shared"}}
}
```

## §10.5 Specific rule — "Are people trusting the reviews?" (query #26)

- Shape: diverging bar — up-votes positive, down-votes negative, sharing
  one categorical (engineer) axis and a zero line. Grounded directly in
  `review_feedback`'s `vote_type` column.

Where the data lives:

- **Table:** `review_feedback` joined to `reviews` for the author name.
- **Start from the feedback table, not from reviews** — every row you want
  is a vote, and reviews with no feedback have nothing to contribute to
  this chart.
- **Exclude retracted votes**, and filter on the feedback's own timestamp
  rather than the review's; a vote cast this week on an old review is
  this week's signal.
- **Group by author and vote type**, returning both as positive counts.
  The negation that puts down-votes below the zero line is a presentation
  step applied afterwards, not something the query should bake in.
- Feedback is sparse in most orgs. If the totals are tiny, say the actual
  counts in the description — "4 up, 1 down" is honest where a chart alone
  implies a trend that five data points cannot support.

Vega-Lite spec:
```json
{
  "width": 600, "height": 320,
  "mark": {"type": "bar"},
  "encoding": {
    "y": {"field": "engineer", "type": "nominal"},
    "x": {"field": "n", "type": "quantitative"},
    "color": {"field": "vote_type", "type": "nominal",
              "scale": {"domain": ["up", "down"], "range": ["#39d353", "#ff5c7c"]}}
  }
}
```

## §10.6 Exception — "What kinds of engineering problems is LiveReview finding?" (treemap requested, bar delivered) (query #17)

- Vega-Lite has no native treemap or Sankey mark. Per the standing rule in
  `internal/mcpagent/prompts/analytics_plan.md`: do not attempt to fake
  one — answer with a sorted `bar` (count per category, optionally colored
  by severity) instead. That is a faithful, honest answer to the same
  question, just not the requested picture.
- Also blocked on the same `ai_comments.content` JSON-payload issue noted
  in §9.1 — category/severity extraction needs to be reliable first.

## §10.7 Exception — "What does a healthy engineering-review workflow look like?" (connected scatterplot, not built) (query #28)

- Would be: a `line` layer (points ordered by period via `"order"`, not
  SQL row order) plus a `point` layer at the same x/y, tracing successive
  weekly states so the trajectory itself becomes meaningful. Vega-Lite
  supports this pattern explicitly; it just hasn't been built against real
  data yet.
