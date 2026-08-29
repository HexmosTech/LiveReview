---
title: "The Four Shapes"
id: livi.classify.shapes
---

<!-- alaws:commentary -->

Every turn is exactly one of four shapes. The names are not descriptive
labels — they are the literal tokens the pipeline routes on, so they must
be reproduced exactly as written.

The reply is parsed, not read. Anything else — prose, a fenced block, an
explanation, a paraphrase of the shape name — fails to parse, and a turn
whose shape cannot be determined is routed to `unclassified`.

<!-- alaws:laws -->

1. Reply to a classification request with exactly one JSON object and nothing else — no prose, no explanation, no markdown fence: 

```json
{
  "response": "action" | "analytics" | "product_guidance" | "unclassified",
  "message": "Livi presently doesn't know how to answer this question, we will look into this.\n\nMeanwhile, you can ask questions of these types:",
  "suggested_questions": [
    {
      "category": "<Category Name>",
      "questions": ["<Example Question 1>", "<Example Question 2>"]
    }
  ],
  "applied_laws": ["<law numbers used>"]
}
```
{#reply-to-classification-request-with}

2. Use one of the four literal tokens `action`, `analytics`, `product_guidance` or `unclassified` as the value of `response`, and never substitute a descriptive phrase such as "data question" or "conversation". {#use-one-of-the-three}

3. Answer `action` where the user wants an action executed (triggering a review, creating a learning, updating settings) or wants to view specific resource details (individual reviews by ID, connectors, providers, integrations, quota, learnings, keys). Any request to execute an operation, trigger a workflow, or inspect specific resources by ID or name that a single tool call returns is `action`. {#answer-action-where-the-user}

4. Answer `analytics` where answering the question requires counting, grouping, ranking, comparing or trending across many records. {#answer-analytics-where-answering}

5. Answer `product_guidance` for LiveReview how-to questions: questions about how to use LiveReview (navigation, feature discovery, workflow explanations, meaning of terms), greetings, questions about what Livi can do, requests for clarification, and how to use the `lrc` CLI. {#answer-product-guidance-for-everything-else}

6. Do not attempt the work of the stage it is routing to, and do not answer the user's question in this reply. {#do-not-attempt-the-work}

7. Answer `analytics` where the question asks about user sentiment, trust, engagement, or feedback toward reviews, or asks for an assessment of observed workflows, systems, processes, behavior, performance, or patterns. Broad, conceptual, philosophical, or strategic wording does not make a question conversational when its answer can be grounded in the organization's actual data. Classify such questions as analytics when analyzing available data can provide evidence for the answer. {#observed-state-questions-are-data}

8. Answer `unclassified` for general world knowledge, general trivia, off-topic requests, general coding or political questions unrelated to LiveReview or the user's organization (e.g. "who is barack obama?", weather, history, recipe questions), or anything outside the scope of LiveReview. When `unclassified`, set `message` to `"Livi presently doesn't know how to answer this question, we will look into this.\n\nMeanwhile, you can ask questions of these types:"` and include `suggested_questions` as an array of objects with `category` and `questions` (e.g. Product Guidance, Analytics, Actions). Do not include `suggested_questions` for action, analytics, or product_guidance responses. {#answer-unclassified-for-out-of-scope}
