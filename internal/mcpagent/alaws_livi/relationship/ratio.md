---
title: "Relationship Where a Measure Is a Ratio"
id: livi.relationship.ratio
---

<!-- alaws:commentary -->

**Applies when** the question asks about coverage, penetration or
share-of-possible — what fraction of the work went through the process at
all, rather than how much work there was.

This differs from the preceding section in where the query starts. A
question about how busy an entity is can start from the activity table. A
ratio needs a denominator of everything that *could* have happened, so it
must start from the population table instead. An entity can be busy and
badly covered, and only this framing shows it.

**Seen as:** "Which repositories have the highest review coverage?"

```json
{
  "width": 600, "height": 380,
  "mark": {"type": "circle", "opacity": 0.85},
  "encoding": {
    "x": {"field": "coverage", "type": "quantitative"},
    "y": {"field": "loc", "type": "quantitative"},
    "size": {"field": "members", "type": "quantitative", "scale": {"range": [60, 900]}},
    "color": {"field": "coverage", "type": "quantitative", "scale": {"scheme": "blues"}}
  }
}
```

<!-- alaws:laws -->

1. Livi must apply this section where a question asks about coverage, penetration or the share of what was possible.

2. Livi must begin the query from the table listing every entity and join the denominator, the numerator and any further measures onto it.

3. Livi must join each table on its own key, since the same entity may be referenced by name in one table and by identifier in another.

4. Livi must count distinctly, because several outer joins multiply rows and a plain count reports figures several times too large.

5. Livi must place date filters inside the join conditions rather than in a trailing filter, since filtering an outer-joined table afterwards silently converts it to an inner join and discards the zero rows.

6. Livi must compute the ratio once as a column and must not require the chart to divide.

7. Livi must distinguish high use from high coverage in the description, as they are different findings and the question asked about the second.

