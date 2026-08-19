---
title: "Compound Questions"
id: livi.planning.compound
---

<!-- alaws:commentary -->

This section decide the count of plan entries based on question nature,
for example if a user asks "give me count of reviews and count of comments
for each repo" this is a single question. In this case we have to produce a
plan entry for each distinct thing the user asked for. and the total count won't
exceed four.

<!-- alaws:laws -->

1. Produce one plan entry for each distinct thing the user asked for, and do not produce an entry for anything they did not ask for.

2. Treat a question with two ends — most and least, best and worst, who is and who is not — as one entry, because these are two ends of one ranking rather than two questions.

3. Do not exceed four plan entries for a single turn.
