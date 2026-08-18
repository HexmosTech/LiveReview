---
id: chart.relationship
number: 7
title: Relationship Between Two Numeric Measures (Scatter / Bubble)
---

# §7 — Relationship between two numeric measures

## §7.0 General rule

**When the question compares two (or three) independent numeric measures
against each other per entity — not one measure ranked, but "how do X and
Y relate" — render a scatter/bubble chart: `circle` mark, x/y = the two
measures, `size` for a third measure if one exists, `color` for the
entity or a fourth dimension.** A single-metric bar chart cannot show this
relationship even if it uses the "right" metric, because it only has one
axis.

## §7.1 Specific rule — "Which repositories are unusually active or inactive?" (query #8)

- Measures: LOC reviewed (x) vs. review count (y) — separates
  high-volume/high-frequency repos from large-diff/low-frequency ones.
  `size` = active engineers.

Where the data lives:

- **Tables:** `reviews` as the base, with `loc_usage_ledger` **left**
  joined onto it. Left, not inner: a repository with reviews but no
  settled ledger rows still belongs on the chart at zero LOC, and an inner
  join would silently delete exactly the "unusually inactive" repos the
  question asks about.
- **Three measures per repository**, all from the same grouped query:
  review count, distinct author count, and summed LOC (treat a null sum as
  zero).
- Distinct-counting the authors is what makes the bubble size meaningful —
  it separates "one person's private repo" from "the whole team's".
- Trailing 90 days.

Vega-Lite spec:
```json
{
  "width": 600, "height": 380,
  "mark": {"type": "circle", "opacity": 0.85},
  "encoding": {
    "x": {"field": "loc", "type": "quantitative"},
    "y": {"field": "reviews", "type": "quantitative"},
    "size": {"field": "engineers", "type": "quantitative", "scale": {"range": [80, 1200]}},
    "color": {"field": "repository", "type": "nominal", "legend": null}
  }
}
```

## §7.2 Specific rule — "Which engineers are getting the most value from LR?" (query #25)

- Measures: reviews (x) vs. up-voted feedback as a proxy for useful
  findings (y). `size` = LOC, `color` = acceptance %. Separates "heavy
  users" from "productive users" — the two are not the same thing.

Where the data lives:

- **Tables:** `reviews` grouped by author, left joined to both
  `loc_usage_ledger` (settled rows) and `review_feedback`.
- **Ignore retracted feedback** — someone who took their vote back did not
  vote.
- **Count up-votes and down-votes as separate filtered aggregates** in one
  pass, rather than running two queries and stitching them together.
- **Watch the double-join.** Joining two one-to-many tables onto `reviews`
  in the same query multiplies rows: each review appears once per ledger
  row *per* feedback row. Count reviews distinctly, and know that the LOC
  sum inflates the same way. If in doubt, aggregate each measure on its
  own and join those results together.
- Up-votes here are a **proxy** for "useful findings", not a measurement
  of it. Say so in the description rather than presenting it as fact.

Vega-Lite spec:
```json
{
  "width": 600, "height": 380,
  "mark": {"type": "circle", "opacity": 0.85},
  "encoding": {
    "x": {"field": "reviews", "type": "quantitative"},
    "y": {"field": "useful_findings", "type": "quantitative"},
    "size": {"field": "loc", "type": "quantitative", "scale": {"range": [60, 900]}},
    "color": {"field": "acceptance", "type": "quantitative", "scale": {"scheme": "greens"}}
  }
}
```

## §7.3 Specific rule — "Which repositories have the highest review coverage?" (query #27)

- Measures: reviews/PRs ratio (x, "coverage") vs. LOC reviewed (y). Same
  mechanism as §7.1 — near-duplicate flagged in the chart-idea grouping
  review, both are "LOC/volume vs. a second repo-level measure, sized by
  engineers." The distinguishing value here is the `coverage` ratio
  itself, joined against `pull_requests`, which §7.1 does not compute.

Where the data lives:

- **Start from `repositories`, not from `reviews`.** Coverage is a ratio
  against everything that *could* have been reviewed, so the denominator
  has to come from the repository and pull-request side of the schema.
- **Tables:** `repositories` left joined to `pull_requests` (the
  denominator), to `reviews` (the numerator), and through those to
  `loc_usage_ledger`.
- **Note the join key mismatch:** reviews reference a repository by
  **name**, while pull requests reference it by **id**. Join each on its
  own key rather than assuming one identifier works for both.
- **Count everything distinctly.** Three left joins fan out badly, and a
  plain count here will report numbers several times too large.
- **Apply the date filters inside the joins, not in a trailing WHERE.**
  Filtering a left-joined table afterwards quietly turns it into an inner
  join and drops the zero rows you went to the trouble of preserving.
- The coverage ratio itself (reviews ÷ PRs) is a derived column — compute
  it once and encode it, rather than making the chart divide.

Vega-Lite spec:
```json
{
  "width": 600, "height": 380,
  "mark": {"type": "circle", "opacity": 0.85},
  "encoding": {
    "x": {"field": "coverage", "type": "quantitative"},
    "y": {"field": "loc", "type": "quantitative"},
    "size": {"field": "engineers", "type": "quantitative", "scale": {"range": [60, 900]}},
    "color": {"field": "coverage", "type": "quantitative", "scale": {"scheme": "blues"}}
  }
}
```

## §7.4 Exception — "What is the blast radius of issues being caught?" (not built) (query #19)

- Needs `ai_comments.file_path`-level joins that
  haven't been validated against real data; file-level blast radius is
  possible, dependency-level would need metadata that doesn't exist today.
- Would follow §7.0's general mechanism (bubble: findings per review ×
  files affected, size = severity/LOC) once the data is confirmed
  reliable — documented as a pending instance of this rule, not a new one.
