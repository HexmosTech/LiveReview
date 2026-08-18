---
title: "Growth in Depth, Not Only Headcount"
id: livi.oneoff.depth
---

<!-- alaws:commentary -->

**Applies when** the question asks whether something is becoming *broader*
or *deeper* over time — whether the organization is moving from a few
enthusiasts to habitual use, rather than simply adding names.

Counting distinct members per bucket is the failure this section exists to
prevent: it yields a headcount line that cannot show depth at all.

**Seen as:** "Is adoption becoming broader over time?"

```json
{
  "width": 800, "height": 380,
  "mark": {"type": "area", "interpolate": "monotone", "line": {"strokeWidth": 1.5}},
  "encoding": {
    "x": {"field": "week", "type": "temporal"},
    "y": {"field": "members", "type": "quantitative", "stack": true},
    "color": {"field": "band", "type": "nominal", "sort": "<band_order>",
              "scale": {"domain": "<band_order>", "range": "<color_range>"}},
    "order": {"field": "band", "sort": "ascending"}
  }
}
```

<!-- alaws:laws -->

1. Livi must apply this section where a question asks whether something is becoming broader or deeper over time rather than merely larger.

2. Livi must group by the time bucket and the member together, since each member's activity level within a bucket must be known before their tier can be assigned.

3. Livi must assign each member-bucket to a tier using the same thresholds as the Distribution chapter, then count members per tier per bucket, because the chart plots people and not events.

4. Livi must stack the tiers without normalising, as the question concerns how many people sit at each depth rather than what fraction of volume they represent.

5. Livi must use a window longer than the default, since this is a slow-moving shift.

6. Livi must quote the change in the heaviest tier separately from the change in the total, because growth confined to the lightest tier tells a different story and the stack alone does not distinguish them.

