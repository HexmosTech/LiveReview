---
title: "Exception: Explaining a Change Rather Than Showing It"
id: livi.charts.comparison.explaining
---

<!-- alaws:commentary -->

**Applies when** the question asks *why* something changed, not whether it
did.

**This section overrides the direction-of-change section.** Showing that
velocity moved does not explain it; the movement has already been
established. The answer is the change broken down by whoever caused it.

**Seen as:** "Why did this repository's velocity change?"

<!-- alaws:laws -->

1. Apply this section, in place of the direction-of-change section, where a question asks why something changed rather than whether it did. {#apply-this-section-in-place}

2. Apply the chapter's two-bucket split grouped by contributor and filtered to the single entity in question. {#apply-the-chapter-two-bucket}

3. Subtract the two periods to produce one row per contributor carrying the change and its direction, since the chart plots the change and not the raw values. {#subtract-the-two-periods-to}

4. Sort by the change so the largest movers sit at the ends, where the reader looks first. {#sort-by-the-change-so}

5. Turn the aggregate into attribution in the description, because naming who accounts for the movement is what the question asked for. {#turn-the-aggregate-into-attribution}

6. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Adapt the field names to those its own query produced:
```json
{
  "width": 600, "height": "<max(200, 30 * n_members)>",
  "mark": {"type": "bar"},
  "encoding": {
    "y": {"field": "member", "type": "nominal", "sort": "x"},
    "x": {"field": "delta", "type": "quantitative"},
    "color": {"field": "direction", "type": "nominal", "legend": null,
              "scale": {"domain": ["up", "down"], "range": ["#39d353", "#ff5c7c"]}}
  }
}
```
{#the-specification-below-is-an}
