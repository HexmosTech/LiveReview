---
title: "Opposing Quantities Either Side of Zero"
id: livi.charts.oneoff.opposing
---

<!-- alaws:commentary -->

**Applies when** the question asks about sentiment, agreement or trust —
anything where two opposed counts share a subject.

**Seen as:** "Are people trusting the reviews?"

<!-- alaws:laws -->

1. Apply this section where a question asks about sentiment, agreement or trust between two opposed counts. {#apply-this-section-where-question}

2. Begin the query from the table holding the opposed events and join outwards for the subject's name, since subjects with no events contribute nothing to this chart. {#begin-the-query-from-the}

3. Exclude retracted or withdrawn events, and filter on the event's own timestamp rather than the subject's, because a vote cast this week on an older item is this week's signal. {#exclude-retracted-or-withdrawn-events}

4. Return both sides as positive counts and treat the negation that places one side below the zero line as presentation. {#return-both-sides-as-positive}

5. Check the totals before rendering and quote them outright where they are small, since a chart alone implies a trend that a handful of points cannot support. {#check-the-totals-before-rendering}

6. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Adapt the field names to those its own query produced:
```json
{
  "width": 600, "height": 320,
  "mark": {"type": "bar"},
  "encoding": {
    "y": {"field": "member", "type": "nominal"},
    "x": {"field": "n", "type": "quantitative"},
    "color": {"field": "vote_type", "type": "nominal",
              "scale": {"domain": ["up", "down"], "range": ["#39d353", "#ff5c7c"]}}
  }
}
```
{#the-specification-below-is-an}
