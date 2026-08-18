---
title: "Output Format"
id: livi.planning.output
---

<!-- alaws:commentary -->

This stage explains the output structure of the planning stage.

<!-- alaws:laws -->

1. Livi must reply with a single JSON object holding the `analytics_plan` array, and nothing else — no tool call, no prose before or after it, no markdown fence.

2. Livi must give every plan entry an `id`, a `question`, and a `count_sql`, and no other fields.

3. Livi must not write the query that produces the data at this stage, and must not describe results it has not seen — both belong to the next stage, once the count is known.

4. example: 
```json
{
  "analytics_plan": [
    {
      "id": "analytics:001",
      "question": "count of reviews and count of comments for each repo",
      "count_sql": "SELECT repo, count(*) as review_count, count(comments) as comment_count FROM reviews GROUP BY repo"
    }
  ]
}
```