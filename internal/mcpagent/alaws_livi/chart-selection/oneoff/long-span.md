---
title: "A Long Span in a Small Space"
id: livi.charts.oneoff.long_span
---

<!-- alaws:commentary -->

**Applies when** the question covers a long stretch and the answer must
stay compact — a dashboard strip rather than a full panel.

Reach for this only where a plain trend line will not fit; it trades
precision for density.

**Seen as:** "How much code has LiveReview reviewed?"

```json
{
  "width": 800, "height": 90,
  "layer": [
    {"mark": {"type": "area", "color": "#7c9cff", "opacity": 0.35, "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "b1", "type": "quantitative", "scale": {"domain": [0, "<band>"]}}}},
    {"mark": {"type": "area", "color": "#7c9cff", "opacity": 0.6, "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "b2", "type": "quantitative", "scale": {"domain": [0, "<band>"]}}}},
    {"mark": {"type": "area", "color": "#7c9cff", "opacity": 1.0, "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "b3", "type": "quantitative", "scale": {"domain": [0, "<band>"]}}}}
  ],
  "resolve": {"scale": {"y": "shared"}}
}
```

<!-- alaws:laws -->

1. Livi must apply this section only where a question covers a long stretch and the answer must remain compact, since a plain trend line with a rolling average is otherwise the default.

2. Livi must produce a plain daily series, zero-filled.

3. Livi must split each day's value into bands after querying, deriving one column per band holding that band's share of the value.

4. Livi must stack the bands as areas of one colour with rising opacity, so that intensity reads as magnitude.

5. Livi must state the peak and the typical level in words, because a compact chart is harder to read precisely and the figures therefore matter more.

6. The specification below is an example of the shape this section's chart takes, not a template to copy verbatim. Livi should adapt the field names to those its own query produced:

```json
{
  "width": 800, "height": 90,
  "layer": [
    {"mark": {"type": "area", "color": "#7c9cff", "opacity": 0.35, "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "b1", "type": "quantitative", "scale": {"domain": [0, "<band>"]}}}},
    {"mark": {"type": "area", "color": "#7c9cff", "opacity": 0.6, "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "b2", "type": "quantitative", "scale": {"domain": [0, "<band>"]}}}},
    {"mark": {"type": "area", "color": "#7c9cff", "opacity": 1.0, "interpolate": "monotone"},
     "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "b3", "type": "quantitative", "scale": {"domain": [0, "<band>"]}}}}
  ],
  "resolve": {"scale": {"y": "shared"}}
}
```
