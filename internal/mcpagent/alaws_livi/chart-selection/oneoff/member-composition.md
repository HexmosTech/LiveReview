---
title: "Composition of Each Member's Activity"
id: livi.charts.oneoff.member_composition
---

<!-- alaws:commentary -->

**Applies when** the question asks what individuals spend their effort on —
whether someone is focused on one area or spread across many.

**Seen as:** "What does each engineer actually spend their review activity
on?"

```json
{
  "width": 700, "height": 340,
  "mark": {"type": "bar"},
  "encoding": {
    "x": {"field": "member", "type": "nominal", "sort": "-y"},
    "y": {"field": "events", "type": "quantitative", "stack": true},
    "color": {"field": "category", "type": "nominal"}
  }
}
```

<!-- alaws:laws -->

1. Livi must apply this section where a question asks what individuals spend their effort on.

2. Livi must group by the member and the category together.

3. Livi must retain the leading few categories per member and gather the remainder into a single residual category, since many categories produce an unreadable stack.

4. Livi must contrast the focused members with the dispersed ones in the description, as that contrast is the finding.

5. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Livi should adapt the field names to those its own query produced: `{"width": 700, "height": 340, "mark": {"type": "bar"}, "encoding": {"x": {"field": "member", "type": "nominal", "sort": "-y"}, "y": {"field": "events", "type": "quantitative", "stack": true}, "color": {"field": "category", "type": "nominal"}}}`
