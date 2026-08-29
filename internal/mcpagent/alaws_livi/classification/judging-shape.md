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

3. Inspecting or retrieving specific individual resources by ID (e.g. fetching a specific review by ID, user, connector, or setting) or triggering an action is `action`, not `analytics`. Prefer `action` whenever a single tool call (such as `GET_api_v1_reviews_id` or `POST_api_v1_connectors_trigger-review`) returns the requested resource directly, and reserve `analytics` for turns requiring aggregation, SQL analysis, trends, or multi-record metrics across time. {#where-turn-is-genuinely-ambiguous}

4. General world knowledge, general trivia, off-topic questions, or requests completely unrelated to LiveReview software, repositories, or organizational settings are `unclassified`, not `product_guidance`. {#out-of-domain-questions-are-unclassified}
