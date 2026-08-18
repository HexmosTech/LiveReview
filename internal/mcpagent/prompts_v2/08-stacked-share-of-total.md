---
id: chart.share
number: 8
title: Stacked Share-of-Total Over Time
---

# §8 — Stacked share-of-total over time

## §8.0 General rule

**When the question asks how the composition of a total is shifting over
time (which category's share is growing/shrinking), render a 100%-stacked
mark (`"stack": "normalize"`) with color = category, not a plain count per
category.** A raw stacked count chart conflates "this category grew" with
"overall volume grew" — normalizing to 100% isolates the mix shift, which
is what these questions are actually asking about.

## §8.1 Specific rule — "Where are reviews happening?" (trigger-source mix, discrete) (query #14)

- Mark: `bar`, weekly buckets — more useful than a single pie chart because
  periods can be compared side by side.

Where the data lives:

- **Table:** `reviews`, grouped by two keys: the week bucket and the
  trigger-type column that records how the review was started.
- **Return raw counts, not percentages.** The chart normalises to 100%
  itself; if you also divide in the query you will normalise twice and get
  a flat line at 100%.
- **Weekly buckets.** Daily is too jittery to read a mix shift from, and
  monthly hides the transition you are trying to show.
- Trailing 90 days or so.

Vega-Lite spec:
```json
{
  "width": 700, "height": 340,
  "mark": {"type": "bar"},
  "encoding": {
    "x": {"field": "week", "type": "temporal"},
    "y": {"field": "n", "type": "quantitative", "stack": "normalize", "axis": {"format": "%"}},
    "color": {"field": "trigger_type", "type": "nominal"}
  }
}
```

## §8.2 Specific rule — "Are we moving review earlier in the development lifecycle?" (trigger-source mix, continuous) (query #15)

- Refines §8.1: **identical SQL**, only the mark changes — `area`
  (`interpolate: monotone`) instead of `bar`, because this question is
  about a continuous transition/trend in the mix, not a period-by-period
  comparison. Same `"stack": "normalize"` mechanism.

Where the data lives: identical to §8.1 — same table, same two grouping
keys, same raw counts. Only the mark changes.

Vega-Lite spec:
```json
{
  "width": 700, "height": 340,
  "mark": {"type": "area", "interpolate": "monotone"},
  "encoding": {
    "x": {"field": "week", "type": "temporal"},
    "y": {"field": "n", "type": "quantitative", "stack": "normalize", "axis": {"format": "%"}},
    "color": {"field": "trigger_type", "type": "nominal"}
  }
}
```

**Consolidation note:** §8.1 and §8.2 share one SQL query and differ only
in `mark.type` (`bar` vs `area`) — a strong candidate for a single rule
with a "discrete comparison vs. continuous trend" mark parameter, the same
pattern flagged for §5.1/§5.2.
