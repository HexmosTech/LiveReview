---
title: "Spread of a Measure Across a Population"
id: livi.charts.distribution.spread
---

<!-- alaws:commentary -->

**Applies when** the question asks how widely or evenly something is
distributed across a group — whether use is broad or concentrated in a
few, how many are light users against heavy ones.

**Seen as:** "How broadly has the organization adopted LiveReview?"

<!-- alaws:laws -->

1. Apply this section where a question asks how widely or evenly something is distributed across a group.

2. Group by the member to obtain one row each, then bucket those rows into bands.

3. Use the same band thresholds throughout a conversation, because a tier that means one thing on this chart and another on the next renders both untrustworthy.

4. Plot the bands in their natural order and do not sort them by height, since reordering destroys the shape the question asks about.

5. Quote how many members fall in each band and name the headline — broad, or concentrated in a few.

6. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Adapt the field names to those its own query produced:

```json
{
  "width": 700, "height": 340,
  "mark": {"type": "bar", "cornerRadiusTopLeft": 4, "cornerRadiusTopRight": 4},
  "encoding": {
    "x": {"field": "band", "type": "nominal", "sort": "<band_order>", "axis": {"labelAngle": 0}},
    "y": {"field": "members", "type": "quantitative"},
    "color": {"field": "band", "type": "nominal", "sort": "<band_order>",
              "scale": {"domain": "<band_order>", "range": "<color_range>"}, "legend": null}
  }
}
```
