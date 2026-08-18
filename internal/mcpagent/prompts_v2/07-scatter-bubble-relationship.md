# §7 — Two measures against each other

> §0 applies in full. Only deviations from it are stated here.

## §7.0 Governing rule

**When a question asks how two measures relate — not one measure ranked,
but whether high X goes with high Y — render a scatter: one dot per
entity, a measure on each axis, size for a third.** A bar chart cannot
answer this even with the right measure, because it only has one axis.

---

## §7.1 — Relationship between two measures of the same entity

**Applies when** the question asks which entities are unusual, which are
outliers, or how two properties of a population relate. Words like
"unusually", "anomalous", "which stand out" are the signal: they imply a
comparison against the rest of the population on more than one dimension.

1. Group by the entity and return **both measures plus a third for size**
   in one pass.
2. **Left join, never inner**, when one measure comes from a secondary
   table. An entity with activity but no rows in that table still belongs
   on the chart at zero — and an inner join deletes exactly the inactive
   entities the question is about.
3. Count contributors distinctly for the size channel. It separates one
   person's private corner from the whole team's.
4. In the description, name the quadrants that matter — high on both, high
   on one only — and the entities sitting in them. A scatter without that
   is a picture, not an answer.

**Seen as:** query #8 — "Which repositories are unusually active or
inactive?" (LOC against review count, sized by engineers) and query #25 —
"Which engineers are getting the most value from LR?" (reviews against
up-voted feedback, sized by LOC).

For #25, two extra cautions: ignore retracted feedback, since someone who
took a vote back did not vote; and up-votes are a **proxy** for useful
findings, so name it as a proxy rather than presenting it as measured.

Query #19 — "What is the blast radius of issues being caught?" — is this
same law (findings per review against files affected, sized by severity)
but is **currently unanswerable** on issue data. Follow §0.8.

Vega-Lite spec:
```json
{
  "width": 600, "height": 380,
  "mark": {"type": "circle", "opacity": 0.85},
  "encoding": {
    "x": {"field": "measure_x", "type": "quantitative"},
    "y": {"field": "measure_y", "type": "quantitative"},
    "size": {"field": "engineers", "type": "quantitative", "scale": {"range": [80, 1200]}},
    "color": {"field": "entity", "type": "nominal", "legend": null}
  }
}
```

---

## §7.2 — Relationship where one measure is a ratio against a whole

**Applies when** the question asks about coverage, penetration or
share-of-possible — what fraction of the work went through the process at
all, not how much work there was.

**This differs from §7.1 in where the query starts.** §7.1 asks how busy
an entity is and can start from the activity table. A ratio needs a
denominator of everything that *could* have happened, so it must start
from the population table instead. An entity can be busy and badly
covered; only this framing shows it.

1. Start from the table listing all entities, then left join the
   denominator, the numerator, and any secondary measures.
2. Watch for **join key mismatches** — different tables may reference the
   same entity by name in one place and by id in another. Join each on its
   own key.
3. **Count distinctly.** Several left joins fan out badly and a plain
   count reports numbers several times too large.
4. **Put date filters inside the joins, not in a trailing WHERE.**
   Filtering a left-joined table afterwards silently turns it into an
   inner join and drops the zero rows you preserved in step 1.
5. Compute the ratio once as a column. Do not make the chart divide.
6. In the description, distinguish high use from high coverage. They are
   different findings and the question asked about the second.

**Seen as:** query #27 — "Which repositories have the highest review
coverage?"

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
