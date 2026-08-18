---
id: chart.distribution
number: 3
title: Distribution Across a Population
---

# §3 — Distribution across a population

## §3.0 General rule

**When the question asks how spread out a metric is across many rows —
not the total, not the ranking, but the shape of the spread — bucket the
metric and render a histogram (`bar` over SQL-computed bins), or keep
every point visible with a jittered strip/beeswarm plot if the population
is small enough to show individually (roughly under ~30 points).**

## §3.1 Specific rule — "How broadly has the organization adopted LiveReview?" (query #3)

- Bucketing happens in Python (`band_for()` in `generate_breadth.py`,
  shared with §4/§1's leaderboard/growth charts), not in SQL — bands are
  `1-4 (light)` / `5-19 (regular)` / `20+ (heavy)`.

Where the data lives:

- **Table:** `reviews`, grouped by author to get one row per engineer with
  their review count.
- **Skip rows with no author.** Automated and system-triggered reviews
  have no person attached, and counting them as an anonymous "engineer"
  invents a teammate who does not exist.
- **Window:** a trailing 90 days. Adoption questions are about the current
  state of the team, and all-time totals let someone who left last year
  keep looking active.
- **Bucketing into light / regular / heavy can happen in the query or
  after it** — either is fine, but the band thresholds must be the same
  ones §4 uses, or "heavy user" quietly means two different things on two
  charts in the same conversation.

Vega-Lite spec:
```json
{
  "width": 700, "height": 340,
  "mark": {"type": "bar", "cornerRadiusTopLeft": 4, "cornerRadiusTopRight": 4},
  "encoding": {
    "x": {"field": "band", "type": "nominal", "sort": "<band_order>", "axis": {"labelAngle": 0}},
    "y": {"field": "engineers", "type": "quantitative"},
    "color": {"field": "band", "type": "nominal", "sort": "<band_order>",
              "scale": {"domain": "<band_order>", "range": "<color_range>"}, "legend": null}
  }
}
```

## §3.2 Specific rule — "Are reviews becoming more iterative?" (query #24)

- Binning happens in SQL this time (nested `GROUP BY`), not in application
  code — the distinction from §3.1 is where the bucketing lives, not the
  chart mechanism, which is identical (`bar`, ordinal x = bucket, y =
  count).

Where the data lives:

- **Table:** `reviews`, keyed on the commit identifier.
- **This is a count of counts — two aggregation passes.** First: how many
  reviews each commit received. Then: how many commits received each of
  those numbers. Skipping the second pass gives you a list of commits,
  which is data, not a distribution.
- **Exclude rows with no commit recorded**, otherwise they collapse into
  one fake mega-commit.
- The x-axis values here are small integers (1, 2, 3 reviews), so they are
  already their own buckets — no band thresholds needed.

Vega-Lite spec:
```json
{
  "width": 600, "height": 340,
  "mark": {"type": "bar", "color": "#7c9cff"},
  "encoding": {
    "x": {"field": "reviews_per_commit", "type": "ordinal"},
    "y": {"field": "commits", "type": "quantitative"}
  }
}
```

## §3.3 Exception — "Which engineers are carrying the repository?" (keep every point visible) (query #12)

- **Exception to §3.0's binning default**: when the population is small
  (one repository's contributor list, not the whole org) and outliers
  themselves are the point of the question, do not bin at all — render
  every engineer as a jittered point (`circle` + `yOffset` on a
  `random()` calculate transform) so no individual gets flattened into a
  bucket average.

Where the data lives:

- **Tables:** `loc_usage_ledger` joined to `reviews`, filtered to one
  repository and to authored (non-null) reviews.
- **Two measures per engineer:** LOC reviewed and review count. LOC drives
  the position on the axis; the count drives the dot size, so a person who
  did a lot of small reviews reads differently from one who did a few
  enormous ones.
- **No bucketing.** One row per engineer is the point — see the exception
  note above.
- Sort by the larger measure so the heaviest contributors are adjacent.

Vega-Lite spec:
```json
{
  "width": 600, "height": "<32 * n_engineers, min 200>",
  "transform": [{"calculate": "random()", "as": "jitter"}],
  "mark": {"type": "circle", "opacity": 0.85},
  "encoding": {
    "x": {"field": "loc", "type": "quantitative"},
    "y": {"field": "engineer", "type": "nominal", "sort": "-x"},
    "yOffset": {"field": "jitter", "type": "quantitative"},
    "size": {"field": "reviews", "type": "quantitative", "scale": {"range": [60, 900]}},
    "color": {"field": "loc", "type": "quantitative", "scale": {"scheme": "blues"}, "legend": null}
  }
}
```
