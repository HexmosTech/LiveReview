---
title: "Output Format"
id: livi.planning.output
---

<!-- alaws:commentary -->

This stage explains the output structure of the planning stage. The
reply is parsed by code, not read by a person, so the shape below is
exact: an `analytics_plan` array holding one entry per report, and
nothing else in the object.

**Filled example — a single-report plan**, for "Is LiveReview adoption
increasing since my team started using it?" Note that `count_sql` counts
months (12), not the underlying reviews, per `livi.planning.counting`:

```json
{
  "analytics_plan": [
    {
      "id": "adoption_trend",
      "question": "Review completions per month for hexmos-internal, current year",
      "count_sql": "SELECT count(*) AS n FROM (SELECT date_trunc('month', completed_at) AS month FROM reviews WHERE org_id = 42 AND status = 'completed' GROUP BY 1) t"
    }
  ]
}
```

**Filled example — a multi-report plan**, for "Reviews per month and my
top reviewers." Two distinct things were asked for (see the "Fan-out" /
compound-question section for when this is one entry versus several), so
this is two entries, each independently answerable:

```json
{
  "analytics_plan": [
    {
      "id": "reviews_per_month",
      "question": "Reviews completed per month for hexmos-internal, current year",
      "count_sql": "SELECT count(*) AS n FROM (SELECT date_trunc('month', completed_at) AS month FROM reviews WHERE org_id = 42 AND status = 'completed' GROUP BY 1) t"
    },
    {
      "id": "top_reviewers",
      "question": "Reviewers ranked by completed review count for hexmos-internal, current year",
      "count_sql": "SELECT count(*) AS n FROM (SELECT author_username FROM reviews WHERE org_id = 42 AND status = 'completed' GROUP BY 1) t"
    }
  ]
}
```

<!-- alaws:laws -->

1. Reply with a single JSON object holding the `analytics_plan` array, and nothing else — no tool call, no prose before or after it, no markdown fence. {#reply-with-single-json-object}

2. Give every plan entry an `id`, a `question`, and a `count_sql`, and no other fields. {#give-every-plan-entry-an}

3. Do not write the query that produces the data at this stage, and do not describe results it has not seen — both belong to the next stage, once the count is known. {#do-not-write-the-query}

4. Produce a plan for the question as asked, and never reply with a clarifying question or a request for more detail at this stage, because the turn has already been routed here as answerable from data. {#produce-plan-for-the-question}

5. Resolve any vagueness by choosing the most reasonable reading — the whole organization, the default window — and stating that choice in the report's `question` field, rather than asking the reader to restate the question. {#resolve-any-vagueness-by-choosing}

6. Begin your reply with the `{` character and end it with the matching `}`, with no text of any kind before or after. {#begin-your-reply-with-the}
