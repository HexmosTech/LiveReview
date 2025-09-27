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

## Contextual Prompt Enhancement — Before/After Comment Demarcation

### Problem Statement

Current prompt building in `cmd/mrmodel/main.go` only considers timeline events (commits, comments) up to the target comment's timestamp. This creates a limited view that excludes:
- Commits that happened after the comment was made
- Subsequent discussion in the same thread or related threads
- Resolution or evolution of the issue being discussed
- Current state of the code that may have changed since the comment

### Proposed Solution

Enhance the prompt structure to include **Before Comment** and **After Comment** sections, each containing the same types of contextual information but clearly demarcated by temporal relationship to the target comment.

#### Enhanced Prompt Structure (XML Format)

```xml
#### Enhanced Prompt Structure (Hybrid: Plain Text + XML Context)

```
ROLE: You are a senior/principal engineer doing a contextual MR review.

GOAL: Provide a specific, correct, and helpful reply to the latest message in the thread, grounded in the actual code and diff.

PRINCIPLES: Be concrete, cite evidence (file/line, diff), keep it concise yet comprehensive. Prefer examples and exact snippets over abstract advice.

You MUST:
- Output valid Markdown. Separate paragraphs with two blank lines; use fenced code blocks for code.
- Use the focused diff and code excerpt to anchor your reasoning (mention file/line when helpful).
- Stay consistent with the codebase's style and patterns visible in the excerpt.
- Consider readability, correctness, performance, security, cost, and best practices when relevant.
- If the thread asks a direct question (e.g., 'does it warrant documentation?'), explicitly answer Yes/No with rationale.
- Choose the appropriate response type and label it: Defend | Correct | Clarify | Answer | Other.
- Pay special attention to the BEFORE/AFTER comment context to understand if issues were already resolved.
- If prior AI guidance was correct, defend with specifics; if wrong, correct it with reasoning.
- If context is insufficient to be certain, state the assumption and provide the best actionable recommendation.
- Avoid formalities like 'Acknowledged'; be direct, kind, and constructive.

OUTPUT FORMAT:
1) ResponseType: <Defend|Correct|Clarify|Answer|Other>
2) Verdict (only if a direct question is present): <Yes/No + 1‑2 lines rationale>
3) Rationale: 3‑6 concise bullets referencing code/diff lines when applicable
4) Proposal: concrete snippet(s) or steps (e.g., docstring, code change), fenced code if applicable
5) Notes: optional risks/trade‑offs, alternatives, or references

---

CONTEXT DATA:

<mr_context>
  <target_comment>
    <author>Shrijith</author>
    <message>Does this function warrant documentation?</message>
    <location file="handler.go" new_line="441" old_line="438" sha="abc12345"/>
    <timestamp>2025-09-26T14:30:00Z</timestamp>
  </target_comment>
  
  <before_comment label="Historical Context - What led to this comment">
    <commits>
      <commit sha="def67890" author="Alice" timestamp="2025-09-26T10:00:00Z">
        Fix validation logic
      </commit>
      <commit sha="ghi78901" author="Bob" timestamp="2025-09-26T12:15:00Z">
        Add error handling
      </commit>
    </commits>
    
    <thread_context>
      <message timestamp="2025-09-26T13:45:00Z" author="Bob" note_id="123">
        This function looks complex, should we document it?
      </message>
      <message timestamp="2025-09-26T14:30:00Z" author="Shrijith" note_id="124">
        Does this function warrant documentation?
      </message>
    </thread_context>
    
    <code_state_at_comment_time>
      <focused_diff>
        <![CDATA[
--- a/handler.go
+++ b/handler.go
@@ -438,6 +441,10 @@
 func ProcessRequest(req *Request) error {
+    if req == nil {
+        return errors.New("request cannot be nil")
+    }
     // existing logic...
        ]]>
      </focused_diff>
      <code_excerpt>
        <![CDATA[
  438 | func ProcessRequest(req *Request) error {
  439 |     if req == nil {
  440 |         return errors.New("request cannot be nil")
  441 |     }
  442 |     // existing logic...
        ]]>
      </code_excerpt>
    </code_state_at_comment_time>
  </before_comment>
  
  <after_comment label="Evolution & Resolution - What happened since the comment">
    <commits>
      <commit sha="jkl90123" author="Alice" timestamp="2025-09-27T09:00:00Z">
        Add inline documentation per review feedback
      </commit>
    </commits>
    
    <thread_evolution>
      <message timestamp="2025-09-27T09:30:00Z" author="Alice" note_id="125">
        I added some inline comments based on the discussion
      </message>
    </thread_evolution>
    
    <related_discussions>
      <thread id="456" file="handler.go" lines="450-460">
        Discussion about error handling patterns
      </thread>
    </related_discussions>
    
    <current_code_state>
      <evolution_diff from_comment_time="abc12345" to_current="jkl90123">
        <![CDATA[
--- comment-time (abc12345)
+++ current HEAD (jkl90123)
@@ -441,6 +441,8 @@
 func ProcessRequest(req *Request) error {
+    // ProcessRequest validates and processes incoming requests.
+    // Returns error if request is nil or validation fails.
     if req == nil {
         return errors.New("request cannot be nil")
     }
        ]]>
      </evolution_diff>
      <current_excerpt>
        <![CDATA[
  439 | // ProcessRequest validates and processes incoming requests.
  440 | // Returns error if request is nil or validation fails.
  441 | func ProcessRequest(req *Request) error {
  442 |     if req == nil {
  443 |         return errors.New("request cannot be nil")
        ]]>
      </current_excerpt>
    </current_code_state>
    
    <resolution_indicators>
      <thread_resolved>false</thread_resolved>
      <emoji_reactions>
        <reaction emoji="eyes" count="1" users="LiveReview-AI"/>
        <reaction emoji="thumbs_up" count="2" users="Alice,Bob"/>
      </emoji_reactions>
    </resolution_indicators>
  </after_comment>
</mr_context>
```
```

#### Implementation Changes Required

1. **Timeline Partitioning**: Split timeline events into before/after based on target comment timestamp
2. **Thread Evolution Tracking**: Capture full thread including messages after target comment
3. **Code Evolution Diff**: Generate diff from comment-time SHA to current HEAD for the same file/lines
4. **Cross-Thread References**: Identify related discussions that reference the same code areas
5. **Resolution Status**: Include thread resolution, reactions, and follow-up activity

#### Benefits

- **Historical Understanding**: AI sees what led to the comment (current behavior)
- **Evolution Awareness**: AI understands how the issue has progressed since the comment
- **Resolution Context**: AI can see if the issue was already addressed or is still pending
- **Current Relevance**: AI can determine if the comment is still applicable to current code state
- **Comprehensive Response**: AI can reference both historical context and current state

#### Implementation Priority

- **Phase 1**: Extend timeline partitioning to include after-comment commits and thread messages
- **Phase 2**: Add current code state and evolution diffs
- **Phase 3**: Add cross-thread references and resolution indicators

---

## Next steps after modeling

- Plug the model into the Reply workflow for context building.
- Add knowledge refs/memory extraction as separate, opt-in steps.
- Expand to GitHub/Bitbucket with provider-specific mappers.
