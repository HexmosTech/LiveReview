---
title: "LiveReview Domain Context"
id: livi.action.domain
---

<!-- alaws:commentary -->

Vocabulary and tool-selection knowledge specific to this platform's REST
API, for turns answered straight from tool results rather than SQL.

<!-- alaws:laws -->

1. Treat "a user who did code reviews" as the same person as a review object's `authorName`/`authorUsername`. Reviews carry `id`, `authorName`, `authorUsername`, `friendlyName`, `aiSummaryTitle`, `status`, `createdAt`, `completedAt`, and a `metadata` object (including `ai_connector_name`, `ai_provider_name`). {#treat-user-who-did-code}

2. Count, group, sort, and rank review data directly - for "top reviewers", call `GET_api_v1_reviews`, count grouped by `authorUsername`, sort descending, return the top N. {#count-group-sort-and-rank}

3. Where the user asks "who got the most code reviewed", "most code reviewed", or anything about LOC per user/member, answer ranked by total LOC reviewed. Call the metric "LOC", never "billable LOC" - the API fields `total_billable_loc`/`totalBillableLoc` are just LOC, since "billable" is a cloud-only distinction that does not exist on self-hosted/unlimited plans. Use `GET_api_v1_billing_usage_members` first for user/member LOC rankings; fall back to `GET_api_v1_reviews_id_accounting` for a single review's LOC, or to `GET_api_v1_billing_usage_summary` for an org-wide total. Where `GET_api_v1_billing_usage_members` returns a permission error, fall back to counting reviews per user via `GET_api_v1_reviews`. {#where-user-asks-who-got}

4. Request `per_page=200` from paginated list endpoints (`GET_api_v1_reviews` and similar) for accurate aggregation, and fetch every remaining page while `hasNext` is true - never report that data is partial due to pagination. Use exact parameter names from each tool's inputSchema; `GET_api_v1_reviews` takes `per_page` (snake_case), not `perPage`. {#request-per-page-200-from}

5. To add an AI provider via `POST_api_v1_aiconnectors`, first call `GET_api_v1_aiconnectors_providers` for the supported-provider list (each entry's `id` is the canonical `provider_name`, alongside a display `name`), match the user's raw request to an entry's `name`/`id`, and pass that canonical `id` as `provider_name` - never a display label or an invented value. Where the user's provider is not in the list, list the supported providers and ask them to choose one. {#to-add-ai-provider-via}

6. Match a request to the closest of these patterns, using exact parameter names from each tool's inputSchema: "top reviewers" → `GET_api_v1_reviews` per_page=200, fetch all pages, group by `authorUsername`, count, sort descending. "Reviews per week/month" → same fetch, group by week/month, count, chart. "Review trends" → same fetch, sort by `createdAt`, group by time period. "Top users by LOC" → `GET_api_v1_billing_usage_members` sorted by `total_billable_loc` descending. "Recent reviews" → `GET_api_v1_reviews` with per_page=20. {#match-request-to-closest-of}

7. Where the user asks "how many members in this org?", "who are the members?", "list org members/team roster", or similar, call `GET_api_v1_org_members` - it returns every member's `name`, `email`, `role`, and `joined_at` plus a `total` count for the caller's own org (no parameters, no org_id needed). State the `total` directly and list names when asked "who". Never use `GET_api_v1_billing_usage_members` for a membership question - that tool only returns members with billing/LOC usage in the current period, so it omits members with zero activity and undercounts the org. {#how-many-members-in-org}

8. Where the user asks "what repos do we have", "list our repositories", "which repos are connected", or similar with no grouping/trend/ranking, call `GET_api_v1_repositories` - it returns each repository plus `open_pr_count` and `last_pr_activity_at`, and a `total` count, paginated (`per_page`, default 20, max 200; pass `per_page=200` and follow `total`/`total_pages` for a full org). Filter with `connector_id`, `provider`, or `search` when the user names one. State the `total` directly for a bare count. {#what-repos-do-we-have}

9. Where the user asks "what PRs are open", "list pull requests", "show merge requests for repo X", or similar with no grouping/trend/ranking, call `GET_api_v1_pull-requests` - it returns every PR/MR across the org's repositories plus a `total` count, paginated the same way as `GET_api_v1_repositories`. Filter with `repository_id` or `connector_id` when the user names a specific repo/connector, `provider` for the Git host, and `state` (e.g. `open`, `closed`, `merged`) - omit `state` for every state, never pass an invented value. {#what-prs-are-open}
