# §10 — One-off shapes

> §0 applies in full. Only deviations from it are stated here.

## §10.0 Governing rule

These share no mechanism with each other or with §1–§9. Match the named
technique to the question; do not default to a bar. Landing here is a
legitimate outcome — do not force a new question into §1–§9 to avoid it.

---

## §10.1 — Growth in depth, not just headcount

**Applies when** the question asks whether something is becoming *broader*
or *deeper* over time — whether the org is moving from a few enthusiasts
to habitual use, rather than simply adding names.

1. Group by the time bucket **and the member together.** You need each
   person's activity level within each bucket before you can say which
   tier they were in.
2. Assign each member-bucket to a tier using §3.1's thresholds, then count
   members per tier per bucket. The chart plots people, not events.
3. **Stack unnormalised**, unlike §8. The question is how many people sit
   at each depth, not what fraction of volume they represent.
4. Use a longer window than the default; this is a slow-moving shift.
5. In the description, quote the change in the heaviest tier separately
   from the change in the total. Growth that is all in the lightest tier
   is a different story from growth in the heaviest, and the stack alone
   does not say which happened.

Going straight to distinct-member counts per bucket is the failure this
law exists to prevent: it yields a headcount line that cannot show depth
at all.

**Seen as:** query #5 — "Is adoption becoming broader over time?"

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

---

## §10.2 — Composition of each member's activity

**Applies when** the question asks what individuals spend their effort on
— whether someone is focused on one area or spread across many.

1. Group by the member and the category together.
2. Keep the top few categories per member and roll the rest into "other".
   Dozens of categories produce an unreadable stack of colours.
3. In the description, contrast the focused members with the spread ones.
   That contrast is the finding.

**Seen as:** query #13 — "What does each engineer actually spend their
review activity on?"

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

---

## §10.3 — Building up to a net figure from components

**Applies when** the question asks what something is worth, what it saved,
or what it cost overall — an answer assembled from parts that add and
subtract.

1. Take from the database only what it actually holds. Usually that is a
   volume and a real cost.
2. **Everything else is an assumption, and must be named as one.** A rate,
   an hours-saved figure, a subscription price — none are in the schema.
3. **State every assumption in the description.** A figure whose inputs
   are invisible is not persuasive to the person who has to defend it.
   Quote the rate and the hours so they can argue with the number rather
   than distrust it.
4. Compute each bar's invisible base before the chart; the mark only draws
   between the two values it is given.
5. Colour additions and subtractions differently, and let the last bar be
   the net.

**Seen as:** query #20 — "How much does LiveReview save versus
alternatives?"

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

---

## §10.4 — A long span in a small space

**Applies when** the question covers a long stretch and the answer has to
stay compact — a dashboard strip rather than a full panel.

**Reach for this only when §1 will not fit.** A plain line with a rolling
average is the default; this trades precision for density.

1. Produce a plain daily series, zero-filled.
2. Split each day's value into bands after the query: pick a band height,
   then derive one column per band holding that band's share of the value.
3. Stack the bands as areas of the same colour with rising opacity, so
   intensity reads as magnitude.
4. In the description, state the peak and the typical level in words. A
   compact chart is harder to read precisely, so the numbers matter more.

**Seen as:** query #21 — "How much code has LR reviewed?"

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

---

## §10.5 — Opposing quantities either side of zero

**Applies when** the question asks about sentiment, agreement or trust —
anything where two opposed counts share a subject.

1. Start from the table holding the opposed events, joined out for the
   subject's name. Subjects with no events contribute nothing here.
2. Exclude retracted or withdrawn events, and filter on the event's own
   timestamp rather than the subject's — a vote cast this week on an old
   item is this week's signal.
3. Return both sides as positive counts. The negation that puts one side
   below the zero line is presentation.
4. **Check the totals before drawing.** These counts are usually sparse.
   If they are tiny, quote them outright — "four up, one down" is honest
   where a chart alone implies a trend five points cannot support.

**Seen as:** query #26 — "Are people trusting the reviews?"

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

---

## §10.6 — When the requested chart type does not exist

**Applies when** the question names or implies a shape Vega-Lite has no
mark for — treemaps and Sankey diagrams being the common ones.

1. Do not fake it. A treemap approximated with nested rectangles is worse
   than an honest alternative.
2. Answer with the closest faithful shape — usually a sorted bar of counts
   per category, optionally coloured by a second dimension.
3. Say in the description that the shape differs from what was asked and
   why. The reader asked for a picture; they will accept a different one
   if told, and distrust it if not.

**Seen as:** query #17 — "What kinds of engineering problems is LiveReview
finding?" Also blocked on issue data (§0.8), so today this answers with
neither shape.

---

## §10.7 — Trajectory across successive states

**Applies when** the question asks whether the overall picture is
improving — several indicators moving together over time, where the path
matters more than any one point.

Plot successive periods as points joined in chronological order, so the
trajectory itself carries the meaning. Order the points with an explicit
ordering channel, not by row order.

**Currently unanswerable** on the indicators this needs: feedback is too
sparse to plot a weekly path. Follow §0.8.

**Seen as:** query #28 — "What does a healthy engineering-review workflow
look like?"
