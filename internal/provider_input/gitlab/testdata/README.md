# GitLab Provider Test Fixtures

- `gitlab-webhook-unified-events-0001.json`: sanitized subset of unified webhook events captured on 2025-10-12 from MR https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/426. Includes three independent bot comments preserved exactly as returned by the provider.
- `gitlab-webhook-expected-timeline-0001.json`: golden timeline emitted by `UnifiedContextBuilderV2` after replaying the `0001` events. Used to catch regressions in timestamp ordering or metadata loss during replay.
- `gitlab-webhook-unified-events-thread-0002.json`: threaded sequence showing a bot suggestion, a human reply, and the bot acknowledgement within the same discussion. Demonstrates the `DiscussionID` link we rely on for GitLab threads.
- `gitlab-webhook-expected-timeline-thread-0002.json`: replay output for the threaded sequence. The regression test confirms the builder keeps all three timeline items grouped and chronologically ordered.
- `gitlab-mr-discussions-0001.json`: trimmed response from `/discussions` including only the threads referenced by the fixtures. Removes unrelated system notes and sensitive headers while keeping author metadata and diff positions.
- `gitlab-mr-notes-0001.json`: trimmed response from `/notes` covering the same comment IDs. Handy for tests that operate on the flat notes endpoint.
- `gitlab-mr-commits-0001.json`: shortened commit list showing the subset referenced by the captured comments. Includes only fields required by conversion tests.
- `gitlab-mr-changes-0001.json`: sanitized `/changes` payload containing two edited files. Mirrors how the GitLab provider currently surfaces diff metadata in production.
- `gitlab-mr-diffs-0001.json`: golden `CodeDiff` output produced from the changes payload. The regression test compares converter output to this snapshot.
