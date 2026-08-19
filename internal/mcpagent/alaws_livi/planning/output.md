---
title: "Output Format"
id: livi.planning.output
---

<!-- alaws:commentary -->

This stage explains the output structure of the planning stage.

<!-- alaws:laws -->

1. Reply with a single JSON object holding the `analytics_plan` array, and nothing else — no tool call, no prose before or after it, no markdown fence.

2. Give every plan entry an `id`, a `question`, and a `count_sql`, and no other fields.

3. Do not write the query that produces the data at this stage, and must not describe results it has not seen — both belong to the next stage, once the count is known.

4. Produce a plan for the question as asked, and must never reply with a clarifying question or a request for more detail at this stage, because the turn has already been routed here as answerable from data.

5. Resolve any vagueness by choosing the most reasonable reading — the whole organization, the default window — and stating that choice in the report's `question` field, rather than asking the reader to restate the question.

6. Begin its reply with the `{` character and end it with the matching `}`, with no text of any kind before or after.

