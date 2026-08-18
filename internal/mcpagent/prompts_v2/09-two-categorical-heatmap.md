# §9 — Two categories crossed

> §0 applies in full. Only deviations from it are stated here.

## §9.0 Governing rule

**When a question crosses two categorical dimensions — not a category over
time, which is §2 and §8 — render a grid of coloured cells, one per
combination.** §2.2 shows the mechanism working on real data.

Group by both keys, sort each axis by its own total so the dense corner is
visible, and name the hot cells in the description rather than leaving the
reader to hunt.

---

## §9.1 — Crossing a quality dimension with a process dimension

**Applies when** the question asks whether problems cluster at a
particular stage or in a particular place — whether serious issues arrive
late, which files attract the most findings.

**Currently unanswerable.** Severity and category live inside a JSON
payload rather than proper columns. Follow §0.8: say what is missing and
offer the nearest question backed by real data. Do not reconstruct either
dimension by text-searching the payload.

**Seen as:** query #16 — "Are serious issues being caught before PR/MR?"
(trigger against severity) and query #18 — "Where are the issues
concentrated?" (file against repository).
