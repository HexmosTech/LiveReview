---
title: "Rule Application"
id: livi.interpreting.application
---

<!-- alaws:commentary -->

How to process the rules in this chapter and report which ones were used.
Without this instruction the model skips rules that look optional but are
not (e.g. axis assignment, breadth histograms) and pads `applied_laws`
with rules it never checked.

<!-- alaws:laws -->

1. Before generating your response, go through EVERY numbered rule in this chapter. For each rule, check whether it applies to the current query. If it does, apply it. {#check-every-rule-before}

2. In your `applied_laws` array, list ONLY the rules you actually applied. Do not skip rules — if the user specifies axis assignments, the axis rule applies. If the question is about ranking, the ranking rules apply. Every rule is a candidate until you have checked it. {#list-only-applied-rules}
