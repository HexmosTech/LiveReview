---
title: "Variables"
id: livi.foundation.variables
---

<!-- alaws:commentary -->

This lawbook references the following `{{variables}}`. An application must
supply every one of them before rendering a law that uses it:

* `org_id` — the organization whose data is being queried.

<!-- alaws:laws -->

1. An application rendering these laws for a prompt must supply a value for every variable referenced by the laws it selects, and the render must fail rather than silently emit an unresolved placeholder.

