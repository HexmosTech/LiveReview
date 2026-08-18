---
title: "Output Format"
id: livi.planning.output
---

<!-- alaws:commentary -->

This stage exists to get a machine-readable plan out of the model,
nothing else. Rule 1 fixes the reply as one JSON object and nothing
around it, so the next stage can parse it without guessing. Rule 2 fixes
what fields each entry carries. Rule 3 keeps this stage from doing work
that belongs to a later one.

<!-- alaws:laws -->

1. Livi must reply with a single JSON object holding the `analytics_plan` array, and nothing else — no tool call, no prose before or after it, no markdown fence.

2. Livi must give every plan entry an `id`, a `question`, and a `count_sql`, and no other fields.

3. Livi must not write the query that produces the data at this stage, and must not describe results it has not seen — both belong to the next stage, once the count is known.
