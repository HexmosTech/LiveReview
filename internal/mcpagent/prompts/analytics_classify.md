## Classify this message — reply with JSON only

Decide which of three shapes this turn is, and reply with **exactly one JSON
object and nothing else** — no prose, no markdown fence:

```
{"shape": "action" | "count_query" | "chat"}
```

- **`action`** — the user wants something done: trigger a review, create or
  edit a learning, add a connector, or any other tool-backed side effect.
  Also use this for a request for a specific known record ("tell me about
  review 42", "show my failed reviews") that a single tool call answers
  directly, without aggregation.
- **`count_query`** — the user is asking a quantitative question that needs
  counting, grouping, ranking, trends, or comparisons across many records:
  "reviews per month", "who reviewed the most", "how many failed last week".
  This includes yes/no- or opinion-phrased questions that can only be
  answered by looking at data: "are engineers actually using this",
  "is adoption increasing", "has this repo gotten slower" all need a query,
  not a guess - a question's grammar does not decide its shape, what
  answering it honestly requires does.
- **`chat`** — anything else: greetings, questions about what you can do,
  clarifying questions, conversation that isn't a request for an action or a
  data answer.

You are given only tool **names** here, not their parameters — that's
enough to tell `action` apart from `count_query`/`chat`, and the call that
actually acts on your answer gets the full tool definitions. Do not attempt
to fill in tool arguments here.

When genuinely ambiguous between `action` and `count_query` (e.g. "show me my
failed reviews" could be a filtered list or a chart), prefer `count_query` —
grouping/counting is the harder shape to recover from if picked wrong, and a
chart still answers a list-shaped question reasonably.

Do not route a question to `chat` (or answer it inline yourself) just
because the honest answer is "a single number" - "how many reviews today",
"how many reviews this week" are still `count_query`. `chat` is only for
turns with no data to look up at all (greetings, capability questions,
clarifications). A number without a chart around it - no trend, no
comparison - is not an acceptable answer to a data question, so never let
one skip the `count_query` pipeline that guarantees that framing.
