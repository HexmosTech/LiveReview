---
title: "Relationship Where a Measure Is a Ratio"
id: livi.charts.relationship.ratio
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

<!-- alaws:laws -->

1. Apply this section where a question asks about coverage, penetration or the share of what was possible. {#apply-this-section-where-question}

2. Begin the query from the table listing every entity and join the denominator, the numerator and any further measures onto it. {#begin-the-query-from-the}

3. Join each table on its own key, since the same entity may be referenced by name in one table and by identifier in another. {#join-each-table-on-its}

4. Count distinctly, because several outer joins multiply rows and a plain count reports figures several times too large. {#count-distinctly-because-several-outer}

5. Place date filters inside the join conditions rather than in a trailing filter, since filtering an outer-joined table afterwards silently converts it to an inner join and discards the zero rows. {#place-date-filters-inside-the}

6. Compute the ratio once as a column and do not require the chart to divide. {#compute-the-ratio-once-as}

7. Distinguish high use from high coverage in the description, as they are different findings and the question asked about the second. {#distinguish-high-use-from-high}

8. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Adapt the field names to those its own query produced:
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
{#the-specification-below-is-an}
