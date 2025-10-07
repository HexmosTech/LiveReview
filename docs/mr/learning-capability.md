## Learning capability — MVP spec (compute-first, cross-platform)

### Typical interaction (NL, zero-slash)
- New MR → LiveReview posts issues/suggestions.
- Teammate disagrees; explains org preference (natural language).
- LiveReview recognizes a learning opportunity; auto-adds learning.
- Bot replies agreeing, reports: "Learning added • LR-ID: <short_id> • [Edit] [Delete] [Open]".
- User may reply with a small modification; LiveReview updates learning and replies: "Learning updated • LR-ID".
- User may also say in natural language to delete; LiveReview infers deletion, deletes, and replies: "Learning deleted • LR-ID".
- Delete via button or ❌ reaction also works; can reference LR-ID in comments for clarity.
- Future MRs show this learning in the reply section when relevant.

Goal: persist org-scope “learnings” from MR discussions; auto upsert; show/edit; reuse in future MRs; minimal tokens.

### Data model (PostgreSQL)
- learnings
	- id (uuid, pk)
	- short_id (text, unique, e.g., base36/ksuid prefix) — LR-ID for threads
	- org_id (uuid, idx)
	- scope_kind (enum: org, repo)
	- repo_id (nullable)
	- title (text)
	- body (text)
	- tags (text[] default '{}', GIN idx)
	- status (enum: active, archived)
	- confidence (int, default 1)
	- simhash (bigint, idx) — dedupe
	- embedding (bytea, nullable) — small, quantized vector for NL match
	- tsv (tsvector) — search (title/body)
	- source_urls (text[] default '{}')
	 - source_context (jsonb, nullable) — repo, pr/issue/mr ids, commit sha, file path, line range, provider refs at creation
	- created_by (uuid), updated_by (uuid)
	- created_at, updated_at (timestamptz)
- learning_events (audit)
	- id, learning_id, org_id, action (add|update|delete|restore),
			provider (github|gitlab|bitbucket), thread_id, comment_id,
			repository (text), commit_sha (text), file_path (text), line_start int, line_end int,
			actor_id, reason_snippet (text), classifier (nl|fallback), context jsonb

Indexes: (org_id, simhash), GIN(tags), GIN(tsv), partial on status='active'.

### Upsert & dedupe (compute-only)
- Normalize text: lower, strip markdown/code, collapse ws.
- simhash(text) (64-bit). Find nearest within Hamming <= 3 inside same org + scope.
- If found -> update: bump confidence, merge tags, append source_urls; keep title unless new title shorter/clearer; update body if length change < 30% or explicit edit.
- If not -> insert.
- Record learning_events.

### Capture triggers (natural language, no slash, no keywords)
Use existing reply model (Gemini/configured LLM) — no extra LLM calls:
- When LiveReview composes any reply, augment prompt: "Also detect learnings; if add/update/delete is warranted, output structured metadata".
- Model returns normal reply text + metadata block (single call).
- If metadata.action=add/update/delete:
	- Extract candidate (title/body/scope/tags) or LR-ID from context.
	- Upsert/delete (dedupe via simhash + optional embedding cosine); record event.
	- Post acknowledgement with LR-ID.

In-thread NL edits/deletes (reply-driven):
- For replies in the LR-ID thread (or mentioning LR-ID), LiveReview runs the same reply model; prompt includes prior LR-ID + current learning.
- Model can output metadata to update or delete; apply and reply with result.

### Thread reporting + inline actions
- After auto-commit: "Learning added • LR-ID: <short_id>" + [Edit] [Delete] [Open].
- Actions mapping
	- Buttons via provider links; fallback: ❌ reaction deletes.
	- Direct reply acts as edit/delete request; LiveReview updates/deletes and posts result.
	- No slash commands.

### UI (LiveReview web, minimal)
- Page: /learnings (org via useOrgContext)
	- Table: LR-ID, title, scope, tags, updated_at, confidence, status.
	- Filters: text (tsv), tags, scope, repo, status; pagination (limit/offset).
	- Inline edit: title/body/tags/scope/status; delete/restore.
	- Detail drawer shows history (learning_events) + source thread links.

### Use in future MRs (reply section)
- On composing MR reply, fetch top-N relevant learnings (N=5):
	- Scope filter: repo match > org.
	- Text rank: tsvector rank vs MR title+description+filenames (no LLM).
	- Order by scope priority, rank desc, confidence desc, recency.
- Insert section "Org learnings" with bullets: title + short body excerpt + link (#id).

### API surface (Go HTTP, JSON)
- POST /api/learnings/apply-action-from-reply
	- {org_id, provider, thread_id, comment_id, metadata:{action:add|update|delete|none, short_id?, title?, body?, scope_kind?, repo_id?, tags?}}
	  -> {action:added|updated|deleted|none, id?, short_id?}
	- Server enriches with MR context (repo/PR/MR id, commit sha, file path/line range) from webhook payload before persisting.
- Standard CRUD
	- GET /api/learnings (q, tags, scope, repo, status, page, limit)
	- GET/PUT/DELETE /api/learnings/:id; POST /api/learnings/:id/restore
- POST /api/learnings/preview (optional): returns dedupe candidate and action

### Provider hooks (GitHub/GitLab/Bitbucket)
- Inbound: events that cause LiveReview to reply (review start, follow-ups, LR-ID thread replies) → run reply model (augmented prompt) → call /apply-action-from-reply with metadata.
- Outbound: bot reply includes LR-ID and action result; observe ❌ reaction for delete as alternative UI.
	- Context sources (reuse existing structures in `webhook_handler.go`):
		- GitHub: issue_comment and review_comment payload fields (repo, PR number, `comment.path`, commit ids, positions).
		- GitLab: Note Hook `object_attributes`, `line_code`/diff refs in discussions; MR ids.
		- Bitbucket: comment.inline.path, from/to line, PR ids.

### Token minimization
- Zero extra LLM calls: piggyback on existing reply call; metadata parsed from same response.
- Compute-only dedupe/search: simhash + tsv; optional quantized embedding for better dedupe (no tokens).
- Keep reply prompt narrow for learning metadata; avoid expanding MR context beyond normal reply needs.

### Auth/RBAC
- Add/update via NL detection; edit via UI or reply; delete/archive require maintainers/admins.
- All endpoints scoped by org_id; audit all changes (learning_events).

### Schema migration (outline)
- CREATE TYPE learning_status, scope_kind.
- CREATE TABLE learnings (... + short_id text unique, embedding bytea nullable ..., source_context jsonb NULL). (No path_glob in MVP)
- CREATE TABLE learning_events (..., classifier text ..., repository text, commit_sha text, file_path text, line_start int, line_end int, context jsonb NULL).
- Indexes: GIN(tags), GIN(tsv), BTREE(simhash), partial active; unique(short_id).

### Today MVP delivery (fast path)
1) DB: migrations (short_id, embedding, tables/indexes).
2) API: ingest-comment, apply-edit-from-comment, upsert/CRUD/list/auth.
3) Webhooks: wire comment create/edit and replies; bot reply with LR-ID.
4) UI: list+edit+delete with LR-ID; source thread links.
5) MR reply: fetch+rank learnings; add section.
6) Optional: reaction-based delete; edit-intent classifier behind flag.

### Notes
- Simhash lib in Go or implement simple 64-bit variant; optional small embedding with on-disk cache.
- Trigram/tsvector via PostgreSQL.
- Cross-platform: reuse existing provider clients; no slash commands.

## Grounding in current codebase (files/functions)

### DB (dbmate migrations)
- Add migration files under `db/migrations/` (dbmate). Do NOT edit `db/schema.sql` directly; it’s generated.
	- 20xxxxxxxx_add_learnings.sql: create `learnings`, `learning_events`, enums, indexes; add `short_id`, `embedding`.
	- 20xxxxxxxx_seed_gist.sql (optional): none required.

### API/server (Go)
- File: `internal/api/webhook_handler.go`
	- Augment prompt builders to request learning metadata in the same reply call:
		- `buildGeminiPromptEnhanced` (GitLab) and `buildGitHubEnhancedPrompt` (GitHub), `buildBitbucketEnhancedPrompt` (Bitbucket): add a small instruction block and output schema for `{action, short_id?, title?, body?, scope?, tags?}`.
	- After generating model output:
		- GitLab: `generateAndPostGitLabResponse`
		- GitHub: `generateAndPostGitHubResponse`
		- Bitbucket: `generateAndPostBitbucketResponse`
		Extract metadata block; collect MR context already assembled by helpers; call learnings service to persist (with context) and return LR-ID; append acknowledgement text to posted reply.
	- For replies to bot comments (edit/delete):
		- GitLab: `processGitLabNoteEvent` and downstream reply path
		- GitHub: `processGitHubCommentForAIResponse`
		- Bitbucket: `processBitbucketCommentForAIResponse`
		Ensure LR-ID in context; pass to prompt; parse metadata with action=update|delete and apply; persist with MR context.
	- Reactions (delete): existing emoji/reaction helpers (`postGitLabEmojiReaction`, `postGitHubCommentReaction`) can be complemented by a small reaction listener path (if available) or rely on NL delete.

- New/modified internal packages (create if absent):
	- `internal/learnings/service.go`
		- Methods: UpsertFromMetadata(ctx, orgID, draft, mrCtx) -> (id, shortID, action)
							 UpdateFromMetadata(ctx, orgID, shortID, deltas) -> (id, action)
								DeleteByShortID(ctx, orgID, shortID, mrCtx) -> ok
							 FetchRelevant(ctx, orgID, repo, changedFiles, title, desc, limit) -> []Learning
	- `internal/learnings/store.go`
		- CRUD with sqlc/pgx: insert/update/delete/select; tsv index; simhash dedupe; optional embedding column; persist `source_context`; record per-event `context`.
	- `internal/learnings/simhash.go` (tiny helper) and `internal/learnings/shortid.go` (base36/ksuid prefix).

### Review integration (reuse existing flow)
- Where MR reply section is assembled (in the same `generate*Response` funcs), after composing main text, fetch `FetchRelevant` and append an "Org learnings" section with top-N.

### UI (React, uses OrganizationSelector context)
- Org context: `ui/src/hooks/useOrgContext.ts` (already used by `OrganizationSelector.tsx`).
- New route: `ui/src/pages/learnings/index.tsx`
	- Reads org from `useOrgContext`; calls `/api/learnings` with filters; shows table with LR-ID.
	- Edit dialog: PUT `/api/learnings/:id`; Delete/Restore actions.
	- Detail drawer: GET `/api/learnings/:id` + events.
- Optionally a compact editor: `ui/src/components/Learnings/LearningEditor.tsx` used by page and by deep link from bot comment.

### HTTP handlers (Go API)
- File: `internal/api/learnings_handler.go` (new)
	- Handlers: GET list, GET one, POST upsert, PUT update, DELETE, POST restore, POST apply-action-from-reply.
	- Wire routes in existing server setup where other API routes are registered.

### Security/RBAC
- Reuse existing auth middleware; org scope from token/session.
- Allow add/update via normal contributor roles when sourced from their replies; restrict delete/restore to maintainers/admins.

### Logging/Audit
- Store `learning_events` on every add/update/delete with provider, thread_id, comment_id from webhook context in `webhook_handler.go`.

## Appendix A — Path scoping (future)
- Idea: allow learnings to target file path patterns (e.g., client/**, infra/**) to increase relevance for file clusters.
- Storage: later extend scope_kind with 'path' and add `path_glob` text; index with trigram; match via doublestar.
- Ranking: if any changed file matches, boost above repo/org.
- Risks: setup burden, pattern brittleness, added complexity; not needed for MVP.
