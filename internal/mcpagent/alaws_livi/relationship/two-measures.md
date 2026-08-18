---
title: "Relationship Between Two Measures of an Entity"
id: livi.relationship.two_measures
---

<!-- alaws:commentary -->

**Applies when** the question asks which entities are unusual, which are
outliers, or how two properties of a population relate. Words such as
"unusually", "anomalous" or "which stand out" are the signal: they imply a
comparison against the rest of the population on more than one dimension.

**Seen as:** "Which repositories are unusually active or inactive?" (lines
of code against review count, sized by contributors) and "Which engineers
are getting the most value from LiveReview?" (reviews against up-voted
feedback, sized by lines of code).

"What is the blast radius of issues being caught?" belongs to this section
but is currently unanswerable on issue data — see the Unavailable Data
section.

```json
{
  "width": 600, "height": 380,
  "mark": {"type": "circle", "opacity": 0.85},
  "encoding": {
    "x": {"field": "measure_x", "type": "quantitative"},
    "y": {"field": "measure_y", "type": "quantitative"},
    "size": {"field": "members", "type": "quantitative", "scale": {"range": [80, 1200]}},
    "color": {"field": "entity", "type": "nominal", "legend": null}
  }
}
```

<!-- alaws:laws -->

1. Livi must apply this section where a question asks which entities are unusual or how two properties of a population relate.

2. Livi must group by the entity and return both measures, together with a third for the size channel, in a single pass.

3. Livi must join secondary measures with an outer join rather than an inner one, because an entity with activity but no rows in the secondary table still belongs on the chart at zero and an inner join deletes precisely the inactive entities the question asks about.

4. Livi must count contributors distinctly for the size channel, since that is what separates one person's private corner from the whole team's.

5. Livi must disregard retracted feedback when feedback is one of the measures, because a member who withdrew a vote did not vote.

6. Livi must name a measure as a proxy in the description where it stands in for something it does not directly record.

7. Livi must name the regions of the chart that matter and the entities sitting in them, since a scatter without that reading is a picture rather than an answer.

