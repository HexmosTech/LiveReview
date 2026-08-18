# §1 — Trend over time

> §0 applies in full. Only deviations from it are stated here.

## §1.0 Governing rule

**When a question asks whether something is changing over time, and the
measure is noisy day to day, never show the raw series alone.** Layer a
smoothing signal — a rolling average, or a percentile band — over it, so
the reader can separate a real trend from daily noise.

The laws below cover specific situations of this kind. Where none of them
matches, this rule alone governs.

---

## §1.1 — Trend of a counted event, org-wide

**Applies when** the question asks whether some countable activity is
rising or falling across the whole organization, with no entity filter and
no second measure.

1. Count the events per day from `reviews` over the window.
2. Compute the rolling average and the period average as window functions
   in the query, not in the chart.
3. Layer three marks: the raw series as a faint area, the rolling average
   as a strong line, the period average as a dashed rule.
4. In the description, state the direction and quote the first and last
   values of the smoothed line.

**Seen as:** query #1 — "Is LiveReview adoption increasing since my team
started using it?"

Vega-Lite spec:
```json
{
  "width": 900, "height": 420,
  "layer": [
    {"mark": {"type": "area", "opacity": 0.25, "color": "#7c9cff", "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "reviews", "type": "quantitative"}}},
    {"mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2.5, "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "rolling_avg_7d", "type": "quantitative"}}},
    {"mark": {"type": "rule", "color": "#ff5c7c", "strokeDash": [6, 4], "strokeWidth": 1.5},
     "data": {"values": [{"period_avg": "<period_avg>"}]},
     "encoding": {"y": {"field": "period_avg", "type": "quantitative"}}}
  ],
  "resolve": {"scale": {"y": "shared"}}
}
```

---

## §1.2 — Trend for one named entity, with a recent interval called out

**Applies when** the question asks what happened to a single named
repository, engineer or team over time — a "what happened to X" question
rather than a whole-org trend.

1. Confirm which entity is meant. If the question implies one but does not
   name it, ask (§0.3).
2. Measure LOC from `loc_usage_ledger` joined to `reviews`, summed per
   day, filtered to that entity. Use LOC, not review count: "velocity"
   means how much code moved, and one large review is not the same event
   as one trivial one.
3. Compute the rolling average in the query, as §1.1.
4. Add a highlight band over the recent interval — two dates you supply,
   not query output.
5. In the description, compare the highlighted interval's average against
   the interval before it. That comparison is the answer to "what
   happened"; the line alone only shows that something did.

**Seen as:** query #10 — "What happened to a repository's velocity?"

Vega-Lite spec:
```json
{
  "width": 800, "height": 340,
  "layer": [
    {"data": {"values": [{"start": "<highlight_start>", "end": "<highlight_end>"}]},
     "mark": {"type": "rect", "color": "#7c9cff", "opacity": 0.12},
     "encoding": {"x": {"field": "start", "type": "temporal"}, "x2": {"field": "end"}}},
    {"mark": {"type": "line", "color": "#3a4358", "strokeWidth": 1},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "loc", "type": "quantitative"}}},
    {"mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2.5},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "rolling_avg", "type": "quantitative"}}}
  ],
  "resolve": {"scale": {"y": "shared"}}
}
```

---

## §1.3 — Two measures over the same period

**Applies when** the question asks whether two different measures are
moving together over time — typically whether more activity also means
more substance behind it.

1. Aggregate each measure separately per day, then join both onto one
   shared date series. Aggregating after a join between them multiplies
   rows and inflates both numbers.
2. Put them on **independent y-scales**, one axis left and one right. They
   are different units; a shared scale flattens the smaller one.
3. Do not add a rolling average. The second line is already the
   comparison, and a third and fourth line make the chart unreadable.
4. In the description, say whether the two moved together or diverged, and
   name the period where they diverged if they did.

**Seen as:** query #23 — "How much engineering work is being covered by
LR?" (LOC against review count)

Vega-Lite spec:
```json
{
  "width": 800, "height": 340,
  "layer": [
    {"mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "loc", "type": "quantitative", "axis": {"titleColor": "#ffb454"}}}},
    {"mark": {"type": "line", "color": "#7c9cff", "strokeWidth": 2},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "reviews", "type": "quantitative", "axis": {"titleColor": "#7c9cff", "orient": "right"}}}}
  ],
  "resolve": {"scale": {"y": "independent"}}
}
```

---

## §1.4 — Exception: when the spread matters, not the average

**Applies when** the question is about how consistent or reliable a
measure is, not where its centre sits — durations, latencies, anything
with a tail. "Are we getting faster" is really "is the slow case getting
worse".

**This overrides §1.0.** A rolling average would hide exactly the signal
being asked about: the median can hold steady while the worst case doubles.

1. Restrict to records that actually completed — a real completion
   timestamp as well as a completed status, since an unfinished item has
   no duration.
2. Derive the duration per record, converted to a unit a person reads
   easily (minutes or hours, not raw seconds).
3. Return three percentiles per bucket — median, low, high — via an
   ordered-set aggregate.
4. **Bucket weekly, not daily.** Percentiles need enough records inside a
   bucket to be stable; a daily p90 over three records is noise wearing a
   statistic's clothing.
5. Layer the band and the median line. In the description, quote the
   median *and* the high percentile — reporting the median alone recreates
   the problem this law exists to avoid.

**Seen as:** query #22 — "Are reviews getting faster?"

Vega-Lite spec:
```json
{
  "width": 700, "height": 340,
  "layer": [
    {"mark": {"type": "errorband", "color": "#7c9cff", "opacity": 0.25},
     "encoding": {"x": {"field": "week", "type": "temporal"}, "y": {"field": "p10", "type": "quantitative"}, "y2": {"field": "p90"}}},
    {"mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2.5, "point": true},
     "encoding": {"x": {"field": "week", "type": "temporal"}, "y": {"field": "p50", "type": "quantitative"}}}
  ]
}
```
