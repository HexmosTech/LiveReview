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

3. Triggering a review for a PR URL or repository (e.g. "trigger review for https://github.com/...", "review PR #11"), listing configured system resources, active connectors, git/AI providers, API keys, settings, or org members/team roster is strictly `action` (never `unclassified`, `analytics`, or `product_guidance`) - this overrides law 2 above wherever a direct tool answers the count/list more cheaply than SQL generation. Prefer `action` whenever an action tool or single REST call handles the requested operation. {#where-turn-is-genuinely-ambiguous}

4. Reserve `unclassified` ONLY for general world knowledge, general trivia, weather, history, recipes, or off-topic questions completely unrelated to LiveReview software, repositories, reviews, or actions (e.g. "who is barack obama?"). Any request to trigger a review or process a PR URL is a LiveReview action and MUST NOT be classified as `unclassified`. {#out-of-domain-questions-are-unclassified}
