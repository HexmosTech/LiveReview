# §10 — One-off shapes

> §0 applies in full. Only deviations from it are stated here.

## §10.0 General rule

These share no mechanism with each other or with §1–§9. Match the named
technique to the question; do not default to a bar. A one-off is a
legitimate outcome, not a filing failure — do not force a new question
into §1–§9 just to avoid landing here.

## §10.1 "Is adoption becoming broader over time?" (query #5)

Stacked area of active engineers by tier — unnormalised, unlike §8,
because the question is how many people are at each depth, not what
fraction of volume they represent.

Data: `reviews` grouped by **week and author together** — you need each
person's activity level within each week before you can say which tier
they were in. Then assign each engineer-week to a tier (same thresholds as
§3.1) and count engineers per tier per week. The chart plots people, not
reviews.

Going straight to distinct-user counts per week is the failure this rule
exists to prevent: it yields a headcount line that cannot show depth at
all. Use a longer window (~180 days); this is a slow-moving shift.

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

## §10.2 "What does each engineer actually spend their review activity on?" (query #13)

Data: `reviews` grouped by author and repository. Keep the top few repos
per engineer and roll the rest into "other" — dozens of repos produce an
unreadable stack of colours.

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

## §10.3 "How much does LiveReview save versus alternatives?" (query #20)

Data: `loc_usage_ledger` carries both review volume and actual LLM cost.
Two totals for the window is all the database gives you.

**Everything else on this chart is an assumption.** The alternative cost
(engineer-hours saved × a rate) and the subscription cost are not in the
schema. State those assumptions in the description — a savings figure
whose inputs are invisible is not persuasive to the person who has to
defend it. The invisible base segment positioning each bar is arithmetic
on the totals.

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

## §10.4 "How much code has LR reviewed?" (query #21)

A horizon graph: bands of the same series stacked with rising opacity, so
intensity reads at a glance without a tall sparse line. Reach for this
only when the span is long and vertical space is scarce — §1 is the
default otherwise.

Data: `loc_usage_ledger` alone, daily sums. The band-splitting is
arithmetic after the query: pick a band height, derive one column per band
holding that band's share of the day's value.

Vega-Lite spec:
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

## §10.5 "Are people trusting the reviews?" (query #26)

Diverging bar — up-votes positive, down-votes negative, across a zero
line.

Data: start from `review_feedback` joined to `reviews` for the author
name; reviews with no feedback contribute nothing here. Exclude retracted
votes, and filter on the feedback's own timestamp — a vote cast this week
on an old review is this week's signal. Return both vote types as positive
counts; the negation is presentation.

Feedback is sparse in most orgs. If the totals are tiny, quote the actual
counts — "4 up, 1 down" is honest where a chart alone implies a trend five
points cannot support.

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

## §10.6 "What kinds of engineering problems is LiveReview finding?" (query #17)

A treemap is requested but Vega-Lite has no treemap or Sankey mark. Do not
fake one — answer with a sorted bar, count per category, optionally
coloured by severity. That is a faithful answer to the same question, just
not the requested picture. Also blocked on issue data (§0.8).

## §10.7 "What does a healthy engineering-review workflow look like?" (query #28)

Would be a connected scatterplot: successive weekly states joined in
order, so the trajectory itself carries the meaning. Blocked on feedback
sparsity (§0.8).
