---
title: "Distribution of a Per-Item Count"
id: livi.charts.distribution.per_item
---

<!-- alaws:commentary -->

**Applies when** the question asks how often something happens *per item*
— reviews per commit, comments per review, retries per job — and whether
that number is growing.

**Seen as:** "Are reviews becoming more iterative?"

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

<!-- alaws:laws -->

1. Livi must apply this section where a question asks how often something happens per item and whether that number is growing.

2. Livi must aggregate twice: first counting the events per item, then counting how many items had each count. Stopping after the first pass yields a list of items, which is data rather than a distribution.

3. Livi must exclude items carrying no identifier, since they otherwise collapse into a single fabricated item that dominates the chart.

4. Livi must treat the resulting counts as ordinal buckets, as they are small integers and already form their own bands.

5. Livi must explain what the tail means in the description, because a long tail is itself the finding: some items are being worked repeatedly.

