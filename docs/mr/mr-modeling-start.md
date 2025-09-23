# MR Modeling Start — Phase-by-phase Plan (GitLab-first)

Goal: Given an MR URL (self-hosted GitLab), build a robust modeling foundation that produces:
- A normalized Timeline (Commits + Comments) in actual chronological order
- A hierarchical Comment graph (threads, root → replies) with anchors
- A Present State snapshot (current diff refs, open/closed threads)
- Participants summary (actors and roles)

Reference MR (hardcoded for test program)
- https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/426

Assumptions
- We will use existing GitLab provider code (`internal/providers/gitlab/gitlab.go` and `gitlab_comment.go`).
- Integration token configured for the self-hosted GitLab instance.
- Minimal local DAO or in-memory store for modeling phases; DB migrations can follow later.

---

## Phase 0 — Scaffolding and contracts

Deliverables
- Minimal domain structs for modeling (in-memory first):
  - MRConversation { providerType, providerMRURL, repoFullName, sourceBranch, targetBranch, state, createdAt }
  - Commit { sha, parentSHAs[], title, message, author, createdAt }
  - Thread { hostThreadID, anchor { kind: general|line, filePath?, baseSHA?, headSHA?, line? }, state }
  - Message { hostMessageID, hostThreadID, actorHandle, actorType, body, createdAt, inReplyToHostID?, kind }
  - TimelineEvent { at, type: commit|comment, refID (sha or hostMessageID), actorHandle }
  - PresentState { diffRefs { baseSHA, headSHA, startSHA }, openThreads[], resolvedThreads[] }
  - Participants { actors[] with counts and lastSeenAt }
- Builder contract (test-first):
  - BuildMRModel(ctx, mrURL) → { Timeline[], Threads[], Messages[], PresentState, Participants }

Validation
- Compile-only with unit tests that validate type shapes and basic ordering logic stubs.

---

## Phase 1 — Commits timeline

Actions
- Fetch MR details and commits using GitLab helpers (versions/changes if needed).
- Map commits to TimelineEvent{ type=commit, at=commit.createdAt, refID=sha, actor=author }.
- Preserve MR-createdAt as the zero point for ordering if helpful.

Validation
- Test program prints the commit sequence in chronological order (oldest → newest).
- Assert monotonic time order; tie-breaker by sequence if timestamps equal.

Output examples
- timeline_commits.json: ordered list of commit SHAs with timestamps.

---

## Phase 2 — Discussions and comment hierarchy

Actions
- Query GitLab MR discussions/notes via `gitlab_comment.go` helpers.
- For each discussion:
  - Identify root note (no in_reply_to) → create Thread with anchor (general or file+line using position data and diff refs).
  - For each reply note, create Message with inReplyToHostID set; link to Thread.
- Derive actorType (human/ai/system) using AI Identity (if available) or config.

Validation
- Test program prints each thread with an indented tree of messages.
- Assert for each message: hostThreadID matches thread; inReplyToHostID chain is acyclic and rooted.
- For line-anchored threads, verify filePath/line exist in MR changes (best-effort mapping).

Output examples
- threads.json: array of threads with root + replies (depth-first or breadth-first order).

---

## Phase 3 — Full Timeline ordering (Commits + Comments)

Actions
- Merge commit and comment notes into a single Timeline.
  - Commit events: use commit.createdAt.
  - Comment events: use note.createdAt.
- Sort by timestamp; if equal, stable order: commits before comments or vice versa (choose and document).
- Include type + reference (sha or hostMessageID) and actor.

Validation
- Test program prints the first N and last N timeline entries with timestamps to verify ordering looks natural (commits → comments → commits …).
- Unit test: inject synthetic commits/notes with equal timestamps and assert our stable ordering.

Output examples
- timeline_full.json: merged, ordered events with type and IDs.

---

## Phase 4 — Present State snapshot

Actions
- Read MR DiffRefs (base/head/start) and current open/resolved state of discussions.
- Populate PresentState:
  - diffRefs: baseSHA, headSHA, startSHA
  - openThreads: list of thread IDs (hostThreadID) with brief anchors
  - resolvedThreads: likewise

Validation
- Test program prints PresentState summary.
- Assert counts match API (number of open/resolved discussions).

---

## Phase 5 — Participants

Actions
- Aggregate unique actors from commits (authors) and messages (note authors).
- Count comments per actor; capture lastSeenAt.
- If AI identity configured/discovered, mark it and separate human vs ai.

Validation
- Test program prints top actors by comment count, and distinct commit authors.
- Unit test: ensure AI identity is correctly classified.

---

## Phase 6 — Persistable store (optional in Phase 1)

Actions
- Introduce simple persistence (start with SQLite or Postgres):
  - Tables: mr_conversations, mr_commits, mr_threads, mr_messages
  - Optional: mr_reactions, provider_identities (if not already present)
- Add DAO methods used by BuildMRModel; keep function pure over interfaces.

Validation
- Round-trip test: build → persist → load produces identical model (ignoring non-deterministic fields).

---

## Phase 7 — Test program (end-to-end harness)

Behavior
- Reads MR URL from env or uses the hardcoded URL above.
- Calls BuildMRModel(ctx, mrURL) and writes artifacts:
  - timeline_commits.json
  - threads.json
  - timeline_full.json
  - present_state.json
  - participants.json
- Prints concise console summaries (counts, first/last timestamps, top actors).

Validation
- Run against MR #426 and confirm files are generated with sane counts.
- Smoke check: at least one commit and threads/messages present (or explicit reason if none).

---

## Risks and edge cases

- Force-push/rebase: timestamps can appear out-of-order; rely on createdAt from API; document limitations.
- Deleted threads/messages: skip or mark; do not break hierarchy.
- Timezone/clock skew: use API-provided ISO8601 strings; parse to UTC.
- Rate limiting: backoff; allow local caching.

---

## Next steps after modeling

- Plug the model into the Reply workflow for context building.
- Add knowledge refs/memory extraction as separate, opt-in steps.
- Expand to GitHub/Bitbucket with provider-specific mappers.
