# §8 — Share of total over time

> §0 applies in full. Only deviations from it are stated here.

## §8.0 Governing rule

**When a question asks how the mix is shifting — which category's share is
growing — stack it and normalise to 100%.** A raw stacked count conflates
"this category grew" with "everything grew"; normalising isolates the
shift, which is what is being asked about.

---

## §8.1 — Shift in the composition of a total

**Applies when** the question asks where something is coming from, whether
one channel is taking over from another, or whether work is moving
from one stage to another.

1. Group by the time bucket and the category, and **return raw counts.**
   The chart normalises; dividing in the query as well normalises twice
   and flattens everything to a hundred percent.
2. **Bucket weekly.** Daily is too jittery to read a mix shift from,
   monthly hides the transition being asked about.
3. Choose the mark from the question: `bar` when the reader is comparing
   discrete periods side by side, `area` when the question is about a
   continuous transition. Nothing else changes between the two.
4. In the description, quote the share at the start and at the end for the
   category that moved most. A shifting mix is invisible as a number
   unless you state both ends.

**Seen as:** query #14 — "Where are reviews happening?" (discrete periods,
`bar`) and query #15 — "Are we moving review earlier in the development
lifecycle?" (continuous transition, `area`).

Vega-Lite spec — swap `"bar"` for `"area"` with `"interpolate": "monotone"`
for the continuous form:
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
