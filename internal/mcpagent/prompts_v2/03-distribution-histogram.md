# §3 — Distribution across a population

> §0 applies in full. Only deviations from it are stated here.

## §3.0 Governing rule

**When a question asks how spread out a measure is across many members of
a population — not the total, not the ranking, but the shape of the
spread — bucket it and show the shape.** If the population is small enough
to show everyone individually and the outliers are the point, show every
member instead of bucketing.

---

## §3.1 — Spread of a measure across a population

**Applies when** the question asks how widely or evenly something is
distributed across a group — whether use is broad or concentrated in a
few, how many are light versus heavy users.

1. Group by the member to get one row each, then bucket those rows into
   bands.
2. Use the **same band thresholds everywhere in the conversation** (§4.1
   uses them too). If "heavy user" means one thing on this chart and
   another on the next, both charts become untrustworthy.
3. Plot band on the x-axis in fixed order, member count on the y-axis, and
   colour by band. Do not sort by height — the bands have a natural order
   and reordering them destroys the shape being asked about.
4. In the description, quote how many fall in each band and name the
   headline: broad, or concentrated in a few.

**Seen as:** query #3 — "How broadly has the organization adopted
LiveReview?"

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

---

## §3.2 — Distribution of a per-item count

**Applies when** the question asks how often something happens *per
item* — reviews per commit, comments per review, retries per job — and
whether that number is growing.

1. **Two aggregation passes.** First count the events per item, then count
   how many items had each count. Stopping after the first pass gives a
   list of items, which is data, not a distribution.
2. Exclude items with no identifier, or they collapse into one fake
   mega-item that dominates the chart.
3. Treat the resulting counts as ordinal buckets — they are small integers
   and already their own bands, so no thresholds are needed.
4. In the description, say what the tail means. A long right tail is the
   finding: some items are being worked repeatedly.

**Seen as:** query #24 — "Are reviews becoming more iterative?"

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

---

## §3.3 — Exception: when individuals matter more than the shape

**Applies when** the population is small — one repository's contributors,
one team — and the question is about *who* stands out rather than what the
overall spread looks like.

**This overrides §3.0's bucketing.** Bucketing averages people away, and
here the outliers are the answer.

1. Return one row per individual; do not bin.
2. Encode two measures: one drives position along the axis, the other
   drives dot size. A person who did many small pieces of work then reads
   differently from one who did a few large ones.
3. Jitter the points so overlapping individuals stay visible.
4. Sort by the positional measure so the heaviest contributors sit
   together.
5. Name the standouts in the description. "Three people account for most
   of it" is the answer; the dots are the evidence.

**Seen as:** query #12 — "Which engineers are carrying the repository?"

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
