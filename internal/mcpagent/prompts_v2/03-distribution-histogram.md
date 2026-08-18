# §3 — Distribution across a population

> §0 applies in full. Only deviations from it are stated here.

## §3.0 General rule

**When the question asks how spread out a metric is across many rows — not
the total, not the ranking, but the shape of the spread — bucket it and
render a histogram.** If the population is small enough to show everyone
individually and the outliers are the point, show every point instead of
bucketing.

## §3.1 "How broadly has the organization adopted LiveReview?" (query #3)

Data: `reviews` grouped by author, one row per engineer. Bucket into
light / regular / heavy — in the query or after it, either is fine, but
the thresholds must match §4's, or "heavy user" quietly means two
different things on two charts in the same conversation.

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

## §3.2 "Are reviews becoming more iterative?" (query #24)

Data: `reviews` keyed on the commit identifier. **This is a count of
counts — two aggregation passes.** First how many reviews each commit
received, then how many commits received each of those numbers. Skipping
the second pass gives a list of commits, which is data, not a
distribution. Exclude rows with no commit, or they collapse into one fake
mega-commit.

The x values are small integers, so they are already their own buckets.

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

## §3.3 Exception — "Which engineers are carrying the repository?" (query #12)

**Exception to §3.0's bucketing default.** The population is one repo's
contributor list and the outliers are the point of the question, so do not
bin — show every engineer as a jittered point, and nobody gets flattened
into a bucket average.

Data: `loc_usage_ledger` joined to `reviews`, filtered to one repository.
Two measures per engineer: LOC drives position, review count drives dot
size — so a person who did many small reviews reads differently from one
who did a few enormous ones.

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
