# §7 — Two measures against each other

> §0 applies in full. Only deviations from it are stated here.

## §7.0 General rule

**When the question asks how two measures relate — not one measure ranked,
but whether high X goes with high Y — render a scatter: one dot per thing,
a measure on each axis, size for a third.** A bar chart cannot show this
even with the right metric, because it only has one axis.

## §7.1 "Which repositories are unusually active or inactive?" (query #8)

LOC against review count separates high-volume/high-frequency repos from
large-diff/low-frequency ones; dot size is the number of active engineers,
which separates one person's private repo from the whole team's.

Data: `reviews` as the base with `loc_usage_ledger` **left** joined —
left, not inner, because a repo with reviews but no settled ledger rows
still belongs on the chart at zero LOC, and an inner join would delete
exactly the inactive repos being asked about.

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

## §7.2 "Which engineers are getting the most value from LR?" (query #25)

Reviews against up-voted feedback, separating heavy users from productive
ones — the two are not the same thing. Up-votes are a **proxy** for useful
findings; say so rather than presenting it as measured fact.

Data: `reviews` grouped by author, left joined to `loc_usage_ledger` and
`review_feedback`. Ignore retracted feedback — someone who took their vote
back did not vote. Count up and down votes as separate filtered aggregates
in one pass. Watch the double-join fan-out (§0.5).

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

## §7.3 "Which repositories have the highest review coverage?" (query #27)

Close to §7.1 mechanically, but the ratio is what makes it a different
question: §7.1 asks how busy a repo is, this asks what fraction of its
work went through review at all. A repo can be busy and badly covered.

Data: **start from `repositories`, not `reviews`** — coverage is a ratio
against everything that could have been reviewed, so the denominator comes
from the repository and pull-request side. Left join `pull_requests` (the
denominator), `reviews` (the numerator), and `loc_usage_ledger`.

Two traps: reviews reference a repository by **name** while pull requests
use an **id**, so join each on its own key. And put the date filters
*inside* the joins — filtering a left-joined table in a trailing WHERE
turns it into an inner join and drops the zero rows you preserved.

Compute the coverage ratio once as a column rather than making the chart
divide.

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

## §7.4 "What is the blast radius of issues being caught?" (query #19)

Would follow §7.0 — findings per review against files affected, sized by
severity. Blocked on issue data (§0.8).
