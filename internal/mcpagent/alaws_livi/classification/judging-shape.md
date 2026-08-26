---
title: "Judging the Shape"
id: livi.classify.judging
---

<!-- alaws:commentary -->

The traps this section closes are the two ways a data question escapes the
pipeline: by being phrased as a yes-or-no, and by having an answer that is
a single number. Both look conversational and are not.

<!-- alaws:laws -->

1. Decide the shape by what answering the question honestly requires, not by its grammar. A question phrased as yes-or-no — whether adoption is rising, whether a repository has slowed — is `count_query` where only data can answer it. {#decide-the-shape-by-what}

2. Answer `count_query` even where the question's literal answer is a single number, because a number without a trend or a comparison around it is not an acceptable answer and only this route guarantees that framing. {#answer-count-query-even-where}

3. A question about the organization's own configuration/settings is `action`, not ambiguous — see the Three Shapes chapter's `action` rule; do not apply this tie-break to override it. Action already produces a Vega-Lite chart for any countable data (see the Final Response Format chapter), so "count_query charts and action doesn't" is not a valid tie-break reason. For any other case genuinely ambiguous between `action` and `count_query`, prefer `action` when a single tool call answers it directly, and `count_query` only where the answer requires combining, filtering, or trending across records that no single tool call returns as-is. {#where-turn-is-genuinely-ambiguous}
