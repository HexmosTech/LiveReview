---
title: "The Chart Grammar"
id: livi.charts.basics
---

<!-- alaws:commentary -->

Every chart this book describes compiles to a **Vega-Lite** specification
— a JSON object, not a picture drawn freehand. This section is the
grammar the rest of Chart Selection assumes: what a spec is built from,
and which pieces are legal. It is sent to both Planning and Finalizing,
because Planning's family sections already talk about "the mark" and "the
encoding" before Finalizing ever gets a chance to define them.

**The three shapes a spec can take** ([layer docs](https://vega.github.io/vega-lite/docs/layer.html),
[facet docs](https://vega.github.io/vega-lite/docs/facet.html)), and when
each applies:

- **Flat** — one `mark` plus one [`encoding`](https://vega.github.io/vega-lite/docs/encoding.html).
  The default; use it unless a family section says otherwise.
- **Layered** — a `layer` list, each entry its own complete `mark` and
  `encoding`, all drawn in the same panel. For a trend plus its rolling
  average, a value plus its target line — anything one mark cannot say by
  itself.
- **Faceted** — a top-level `facet` (the category field) plus one `spec`
  (the single panel, written once), repeated automatically per category.
  For small multiples — the same chart once per repository, once per
  engineer.

Never combine `layer` and `facet` in the same spec: layers overlay in one
panel, facets split across panels, and mixing the two is not a shape
Vega-Lite renders sensibly.

This section only covers the building blocks. The complete response
envelope a Finalizing reply must produce — `response_type`, `data_sql`,
`title`, `description`, and where `mark`/`encoding`/`layer`/`facet` sit
inside it — is `livi.finalizing.response_shape`'s worked examples, not
this one. Planning never emits any of this JSON at all; it only needs to
know the vocabulary well enough to choose the right shape while deciding
what to group by.

The [Vega-Lite mark docs](https://vega.github.io/vega-lite/docs/mark.html)
are the authoritative source for law 2's mark list below — a lower-level
[Vega mark reference](https://vega.github.io/vega/docs/marks/) also
exists, but is not the same list: a few of Vega's primitives (`group`,
`path`, `shape`, `symbol`) are never legal as a top-level `mark` value in
a Vega-Lite spec, since Vega-Lite compiles down to them internally rather
than exposing them directly. The [type docs](https://vega.github.io/vega-lite/docs/type.html)
cover law 3's `temporal`/`quantitative`/`ordinal`/`nominal` field types.

<!-- alaws:laws -->

1. Build every chart as a Vega-Lite specification: a flat `mark` plus `encoding`, a `layer` list of several such pairs sharing one panel, or a `facet` plus a single `spec` repeated once per category — never `layer` and `facet` together.

2. Choose `mark` from `bar`, `line`, `point`, `circle`, `area`, `arc`, `rect`, `errorband` or `text`, and no mark outside this list.

3. Set each encoded field's type to `temporal` for dates, `quantitative` for numbers, `ordinal` for ranked categories, or `nominal` for unranked ones — this governs how the axis scales and sorts, not just how it labels.

4. Questions of the following kinds have been asked and answered before. Treat them as illustrations of the range of phrasing this book covers, not as an exhaustive list to match against verbatim — a question with the same underlying comparison, worded completely differently, is equally in scope, and a question resembling none of these is still answerable from the general rules alone:

| #   | Question                                                                | Law                                             |
| --- | ----------------------------------------------------------------------- | ----------------------------------------------- |
| 1   | Is LiveReview adoption increasing since my team started using it?       | [5.2.2](livi.charts.trend.counted_event)        |
| 2   | Are engineers actually incorporating reviews into their daily workflow? | [5.3.3](livi.charts.rhythm.habit)               |
| 3   | How broadly has the organization adopted LiveReview?                    | [5.4.4](livi.charts.distribution.spread)        |
| 4   | Who has adopted LiveReview — and who hasn't?                            | [5.5.2](livi.charts.ranking.against_target)     |
| 5   | Is adoption becoming broader over time?                                 | [5.11.2](livi.charts.oneoff.depth)              |
| 6   | Which repositories are gaining or losing engineering velocity?          | [5.7.2](livi.charts.comparison.direction)       |
| 7   | Where is organizational velocity concentrated?                          | [5.6.2](livi.charts.concentration.entities)     |
| 8   | Which repositories are unusually active or inactive?                    | [5.8.3](livi.charts.relationship.two_measures)  |
| 9   | What does engineering activity look like across repositories and days?  | [5.3.2](livi.charts.rhythm.entities)            |
| 10  | What happened to a repository's velocity?                               | [5.2.3](livi.charts.trend.named_entity)         |
| 11  | Why did this repository's velocity change?                              | [5.7.3](livi.charts.comparison.explaining)      |
| 12  | Which engineers are carrying the repository?                            | [5.4.2](livi.charts.distribution.individuals)   |
| 13  | What does each engineer actually spend their review activity on?        | [5.11.4](livi.charts.oneoff.member_composition) |
| 14  | Where are reviews happening?                                            | [5.9.2](livi.charts.composition.shift)          |
| 15  | Are we moving review earlier in the development lifecycle?              | [5.9.2](livi.charts.composition.shift)          |
| 16  | Are serious issues being caught before PR/MR?                           | [5.10.2](livi.charts.crosstab.quality_process)  |
| 17  | What kinds of engineering problems is LiveReview finding?               | [5.11.5](livi.charts.oneoff.missing_type)       |
| 18  | Where are the issues concentrated?                                      | [5.10.2](livi.charts.crosstab.quality_process)  |
| 19  | What is the blast radius of issues being caught?                        | [5.8.3](livi.charts.relationship.two_measures)  |
| 20  | How much does LiveReview save versus alternatives?                      | [5.11.6](livi.charts.oneoff.net_figure)         |
| 21  | How much code has LiveReview reviewed?                                  | [5.11.3](livi.charts.oneoff.long_span)          |
| 22  | Are reviews getting faster?                                             | [5.2.4](livi.charts.trend.spread)               |
| 23  | How much engineering work is being covered by LiveReview?               | [5.2.5](livi.charts.trend.two_measures)         |
| 24  | Are reviews becoming more iterative?                                    | [5.4.3](livi.charts.distribution.per_item)      |
| 25  | Which engineers are getting the most value from LiveReview?             | [5.8.3](livi.charts.relationship.two_measures)  |
| 26  | Are people trusting the reviews?                                        | [5.11.7](livi.charts.oneoff.opposing)           |
| 27  | Which repositories have the highest review coverage?                    | [5.8.2](livi.charts.relationship.ratio)         |
| 28  | What does a healthy engineering-review workflow look like?              | [5.11.8](livi.charts.oneoff.trajectory)         |
| 29  | How much of the organization's activity is covered by the top users?    | [5.6.2](livi.charts.concentration.entities)     |
| 30  | What changed between week 1 and week 2?                                 | [5.7.4](livi.charts.comparison.metrics)         |
