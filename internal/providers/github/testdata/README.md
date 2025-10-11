# GitHub Provider Test Fixtures

- `github-pr-files-0003.json`, `github-pr-diffs-0004.json`: raw API patch payload and expected `models.CodeDiff` output captured from a GitHub PR. Used by regression test `TestGitHubPatchConversionMatchesFixture`.
- `github-webhook-unified-events-0001.json`: sanitized subset of unified webhook events captured on 2025-10-11 from PR https://github.com/livereviewbot/glabmig/pull/2. Sensitive headers and tokens were removed; only fields referenced by the regression test are preserved.
- `github-webhook-expected-timeline-0001.json`: golden unified timeline built from the `github-webhook-unified-events-0001.json` input after replaying the events through the context builder.
