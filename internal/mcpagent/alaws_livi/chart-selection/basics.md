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
