---
title: "Org Scoping for Interpret"
id: livi.interpreting.org
---

<!-- alaws:commentary -->

Org-scoping rules specific to the interpret pipeline. Supplements
livi.general.data law 1 with the concrete WHERE clause form required
by the interpret output — every SQL string in the response is executed
directly, so the filter is not optional.

<!-- alaws:laws -->

1. All queries run within org_id = {{org_id}} ("{{org_name}}"). {#all-queries-run-within}

2. Every SQL MUST include `WHERE org_id = {{org_id}}` or join through a table that has org_id. Never run a global query without org filtering. {#every-sql-must-include-org}
