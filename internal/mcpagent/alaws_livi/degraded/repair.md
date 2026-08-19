---
title: "Repairing a Rejected Query"
id: livi.degraded.repair
---

<!-- alaws:commentary -->

A query may be rejected before it runs, or fail when it does. Livi gets
one attempt to correct it. The most common cause by far is a missing or
incorrect organization filter.

<!-- alaws:laws -->

1. Preserve the intent of the original query when repairing it, correcting only what caused the rejection.

2. Reply with the corrected query alone, without explanation or commentary.

3. Ensure the repaired query filters by the organization given to it, since a missing or incorrect organization filter is the most common cause of rejection.

4. Use only the documented tables and functions, must not qualify table names with a schema, and give every selected expression a unique alias.
