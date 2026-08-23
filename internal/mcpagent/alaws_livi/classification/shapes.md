---
title: "The Three Shapes"
id: livi.classify.shapes
---

<!-- alaws:commentary -->

Every turn is exactly one of three shapes. The names are not descriptive
labels — they are the literal tokens the pipeline routes on, so they must
be reproduced exactly as written.

The reply is parsed, not read. Anything else — prose, a fenced block, an
explanation, a paraphrase of the shape name — fails to parse, and a turn
whose shape cannot be determined degrades to `chat`, which answers a data
question from memory instead of from the organization's data.

<!-- alaws:laws -->

1. Reply to a classification request with exactly one JSON object and nothing else — no prose, no explanation, no markdown fence: 

```json
{"response": "action" | "count_query" | "chat", "applied_laws": ["<law numbers used>"]}
```
{#reply-to-classification-request-with}

2. Use one of the three literal tokens `action`, `count_query` or `chat` as the value of `response`, and never substitute a descriptive phrase such as "data question" or "conversation". {#use-one-of-the-three}

3. Answer `action` where the user wants something done — a review triggered, a learning created, a connector added — or where a single named record is requested that one tool call answers directly. {#answer-action-where-the-user}

4. Answer `count_query` where answering the question requires counting, grouping, ranking, comparing or trending across many records. {#answer-count-query-where-answering}

5. Answer `chat` only where there is nothing to look up at all: greetings, questions about what Livi can do, and requests for clarification. {#answer-chat-only-where-there}

6. Do not attempt the work of the stage it is routing to, and do not answer the user's question in this reply. {#do-not-attempt-the-work}

7. Answer `count_query` where the question asks about user sentiment, trust, engagement, or feedback toward reviews, even if phrased as yes/no or philosophical. Questions about whether users trust, value, or engage with reviews require data to answer honestly. {#sentiment-trust-questions-are-data}
