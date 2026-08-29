---
title: "Judging the Shape"
id: livi.classify.judging
---

<!-- alaws:commentary -->

The traps this section closes are the two ways a data question escapes the
pipeline: by being phrased as a yes-or-no, and by having an answer that is
a single number. Both look conversational and are not.

<!-- alaws:laws -->

1. Decide the shape by what answering the question honestly requires, not by its grammar. A question phrased as yes-or-no — whether adoption is rising, whether a repository has slowed — is `analytics` where only data can answer it. {#decide-the-shape-by-what}

2. Answer `analytics` even where the question's literal answer is a single number, because a number without a trend or a comparison around it is not an acceptable answer and only this route guarantees that framing. {#answer-analytics-even-where}

3. Listing or retrieving configured system resources, active connectors, git/AI providers, API keys, settings, or a specific review by ID (e.g. "list the git providers I have", "show active connectors", "review #123") or triggering an action is strictly `action`, never `analytics`. Prefer `action` whenever a single tool call returns the requested configuration or resources directly, and reserve `analytics` strictly for turns requiring multi-record SQL analysis, aggregations, or historical trends over time. {#where-turn-is-genuinely-ambiguous}

4. General world knowledge, general trivia, off-topic questions, or requests completely unrelated to LiveReview software, repositories, or organizational settings are `unclassified`, not `product_guidance`. {#out-of-domain-questions-are-unclassified}
