---
title: "Distribution of a Per-Item Count"
id: livi.charts.distribution.per_item
---

<!-- alaws:commentary -->

**Applies when** the question asks how often something happens *per item*
— reviews per commit, comments per review, retries per job — and whether
that number is growing.

**Seen as:** "Are reviews becoming more iterative?"

<!-- alaws:laws -->

1. Apply this section where a question asks how often something happens per item and whether that number is growing.

2. Aggregate twice: first counting the events per item, then counting how many items had each count. Stopping after the first pass yields a list of items, which is data rather than a distribution.

3. Exclude items carrying no identifier, since they otherwise collapse into a single fabricated item that dominates the chart.

4. Treat the resulting counts as ordinal buckets, as they are small integers and already form their own bands.

5. Explain what the tail means in the description, because a long tail is itself the finding: some items are being worked repeatedly.

6. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Adapt the field names to those its own query produced:

```json
{
  "width": 600, "height": 340,
  "mark": {"type": "bar", "color": "#7c9cff"},
  "encoding": {
    "x": {"field": "events_per_item", "type": "ordinal"},
    "y": {"field": "items", "type": "quantitative"}
  }
}
```
