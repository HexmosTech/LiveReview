---
title: "Crossing Quality With Process"
id: livi.charts.crosstab.quality_process
---

<!-- alaws:commentary -->

**Applies when** the question asks whether problems cluster at a
particular stage or in a particular place — whether serious issues arrive
late, which files attract the most findings.

**Currently unanswerable.** Severity and category live inside a JSON
payload rather than proper columns.

**Seen as:** "Are serious issues being caught before PR/MR?" (trigger
against severity) and "Where are the issues concentrated?" (file against
repository).

<!-- alaws:laws -->

1. Apply this section where a question asks whether problems cluster at a particular stage or in a particular place. {#apply-this-section-where-question}

2. State that the question cannot presently be answered and offer the nearest question backed by real data, since the dimensions this section needs are not recorded as queryable columns. {#state-that-the-question-cannot}

3. Do not reconstruct a severity or a category by searching the text of a stored payload. {#do-not-reconstruct-severity-or}
