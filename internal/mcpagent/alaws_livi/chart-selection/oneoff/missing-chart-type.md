---
title: "When the Requested Chart Type Does Not Exist"
id: livi.charts.oneoff.missing_type
---

<!-- alaws:commentary -->

**Applies when** the question names or implies a shape the rendering
library has no mark for — treemaps and Sankey diagrams being the common
ones.

**Seen as:** "What kinds of engineering problems is LiveReview finding?"
This one is also blocked on issue data, so today it answers with neither
shape.

<!-- alaws:laws -->

1. Livi must apply this section where a question names or implies a chart type the rendering library does not support.

2. Livi must not approximate the unsupported shape, since an imitation is worse than an honest alternative.

3. Livi must answer with the closest faithful shape, ordinarily a sorted bar chart of counts per category.

4. Livi must state in the description that the shape differs from the one requested and why, because a reader told of the substitution will accept it and one left to discover it will not.

