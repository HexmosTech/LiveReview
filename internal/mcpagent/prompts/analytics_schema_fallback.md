### Tables

The live schema reference is unavailable right now, so only table names are
listed below — no columns. Propose a plan using these table names; if you
need to guess a column name, prefer the obvious one (`id`, `created_at`,
`org_id`) and let the query validator's rejection guide a retry rather than
guessing something exotic.

`reviews`, `repositories`, `pull_requests`, `ai_comments`, `review_events`,
`review_feedback`, `users`, `user_roles`, `orgs`

An owner may additionally query `loc_usage_ledger` and `org_billing_state`.
