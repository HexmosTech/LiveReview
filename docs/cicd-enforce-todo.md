# CI/CD Enforcement UI — Round 5 Fix List

Tracking the 11 items from the user's 2026-09-05 feedback round. Check off as completed.

1. [x] Mega menu: "CI/CD Gates" section + "Rulesets" item must live under the "Reviews" top-level category, not its own top-level section.
2. [x] Ruleset list UI: rebuild using the same table component/patterns as https://manual-talent2.apps.hexmos.com/#/reviews (ClientTable, headings, buttons, styling) — not the ad hoc `Card` list.
3. [x] URL routing: ruleset create/edit, and tab state within the editor (Live Results tab, review tabs), must be reflected in the URL bar so back/forward and refresh work, matching the rest of LiveReview.
4. [x] "Add Review" dialog must show real review titles exactly like the main Reviews list does (source icon, status badge colors, PR/MR title logic) — reuse the same helpers as `Reviews.tsx`.
5. [x] "Ask LLM" links: ChatGPT/Gemini don't reliably prefill via URL param. Research and implement a robust approach (clipboard + open, or a supported prefill scheme per provider).
6. [x] Ctrl+Space hint: merge visually into/next to the "Insert reference" button instead of a separate floating `<kbd>`.
7. [x] Insert Reference: add a "Confidence" section (values from the taxonomy endpoint) alongside Severity/Type/Classification.
8. [x] Embed an adapted jq quick-help sidebar (sourced from https://learnxinyminutes.com/jq/) — non-intrusive, toggleable.
9. (skipped in original numbering)
10. [x] Disable "Save changes"/"Create ruleset" until the jq expression validates (gate on live-preview error state).
11. [x] Validate the generated CI integration script end-to-end against real backend behavior (endpoint contracts, jq parsing, exit codes) — as close to a real GitHub Actions run as achievable in this environment.

## Notes as of implementation

- `/api/v1/review-coverage` (POST `{commits:[...]}`) exists and returns `{commits, reports:[{ref, review_id, status, ...}], envelope}` — confirmed against `internal/api/review_coverage.go`. The curl snippet's `.reports[0].review_id` path is correct.
- `/api/v1/reviews` returns `{reviews: ReviewResponse[]}` with camelCase `mrTitle`/`friendlyName`/`authorName`/`prMrUrl`/`triggerType`/`provider` — matches what the Reviews list page already renders via `getPrimaryTitle`/`reviewStatusBadge`/`SourceIcon` in `ui/src/pages/Reviews/Reviews.tsx`. Extracting those into a shared helper module is the fix for item 4.
- No tunnel tool (ngrok/cloudflared) or `act` is available in this environment, and GitHub-hosted Actions runners cannot reach `localhost`. Item 11 is validated by `internal/api/ci_gate_script_e2e_test.go` (`TestCIGateGeneratedScriptE2E`): it boots the real ci-rulesets + review-coverage HTTP routes (full auth middleware chain: API key validation, org context, permission context) against a real Postgres DB via `httptest.Server`, seeds a real org/user/API key/review/two rulesets, and runs the *actual generated bash script text* as a real `bash` subprocess against it, asserting the real process exit codes for: blocks on a critical finding (exit 1), allows when the rule is false (exit 0), and skips gracefully when no review covers the commit (exit 0). All three pass. A live GitHub-hosted-runner-to-LiveReview-server run was not possible without a public endpoint; this limitation is called out explicitly rather than faked.
- This work also surfaced that `RequireAuthOrAPIKey` silently overwrites the `X-Org-Context` header from the API key's own org — so the header isn't strictly load-bearing for a single-org key today, but the generated script now sends it explicitly anyway rather than depending on that internal overwrite behavior.
- The list/editor/integration pages were split into `ui/src/pages/CiRulesets/{CiRulesetsRoutes,CiRulesetsList,CiRulesetEditor,CiRulesetIntegration,shared,jqQuickHelp}.tsx` (previously one `CiRulesets.tsx`). Review display helpers (`getPrimaryTitle`, `reviewStatusBadge`, `SourceIcon`, etc.) were extracted from `Reviews.tsx` into `ui/src/utils/reviewDisplay.tsx` so both pages render review titles identically.
