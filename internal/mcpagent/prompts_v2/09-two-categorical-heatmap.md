# §9 — Two categories crossed

> §0 applies in full. Only deviations from it are stated here.

## §9.0 General rule

**When the question crosses two categorical dimensions — not a category
over time, that is §2 and §8 — render a grid of coloured cells, one per
combination.** §2.2 shows the mechanism working.

## §9.1 "Are serious issues being caught before PR/MR?" (query #16)

Would be trigger source against severity, coloured by finding count.

## §9.2 "Where are the issues concentrated?" (query #18)

Would be file against repository, coloured by finding count, top files
only.

Both are blocked on issue data, not on chart design (§0.8). Severity and
category live inside a JSON payload rather than proper columns. Do not
fake either from a text search over that payload.
