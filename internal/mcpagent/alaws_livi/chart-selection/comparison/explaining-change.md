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

<!-- alaws:laws -->

1. Livi must apply this section, in place of the direction-of-change section, where a question asks why something changed rather than whether it did.

2. Livi must apply the chapter's two-bucket split grouped by contributor and filtered to the single entity in question.

3. Livi must subtract the two periods to produce one row per contributor carrying the change and its direction, since the chart plots the change and not the raw values.

4. Livi must sort by the change so the largest movers sit at the ends, where the reader looks first.

5. Livi must turn the aggregate into attribution in the description, because naming who accounts for the movement is what the question asked for.

