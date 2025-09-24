# Review Progress UI — Product & Technical Specification

## Overview

Build a customer-facing Review Progress UI with an API + database backend that:
- Lists all reviews with key metadata and live status.
- Shows per-review logs and progress in near real-time, including prompt/response visibility, batching, and AI-connector health.

Primary goals:
- Transparency: Customers can see what’s happening during a review in real time.
- Self-diagnosis: Clear surface of problems (auth, rate limit, streaming stalls), with actionable tips.
- Operability: Minimal friction to triage issues (copy URLs, download logs, retry).

Non-goals (for MVP):
- Full audit-trail governance (export-only is enough initially).
- Fine-grained RBAC beyond org/user scope already in use.

## User Stories

1. As a user, I can see a list of my recent reviews, their status (queued/running/success/failed/canceled), duration, and target MR/PR.
2. As a user, I can drill into a review and watch live progress (batches, model calls, streaming chunks or fallback), and see the final summary/comments.
3. As a user, if something fails, I’m told exactly why and what I can try (retry, check token, change model, open log).
4. As an admin, I can filter by org, connector, model, and time window to spot systemic issues (e.g., an Ollama proxy blocking streams).
5. As a support engineer, I can download the full review log for a single review.

## Glossary

- Review: One end-to-end execution for a given MR/PR initiated by a user or webhook.
- Batch: A chunk of diffs processed together by the AI provider (e.g., batch-1).
- Connector: AI provider configuration (e.g., Ollama, Gemini, OpenAI) and its runtime settings.

## Data Model (DB) — Minimal, reuse existing tables

Re-use (already exist in schema):
- reviews (public.reviews): id, pr_mr_url, connector_id, status (created/in_progress/completed/failed), created_at/started_at/completed_at, provider, org_id, metadata jsonb, etc.
- ai_connectors (public.ai_connectors): connector configuration.
- integration_tokens (public.integration_tokens): tokens for providers.
- ai_comments (public.ai_comments): persists final comments/summaries.

Add ONE new table for structured JSON events (append-only):

- review_events (new)
	- id bigserial PK
	- review_id bigint references public.reviews(id) on delete cascade
	- ts timestamptz default now()
	- event_type text — e.g., status | log | batch | artifact | completion | metric
	- level text nullable — info | warn | error | debug (for event_type=log)
	- batch_id text nullable — e.g., "batch-1"
	- data jsonb not null — event payload (see Event Schema)
	- org_id bigint not null — denormalized for auth/indexing (copy from reviews.org_id)

Indexes:
- idx_review_events_review_ts (review_id, ts)
- idx_review_events_org_ts (org_id, ts)
- optional: idx_review_events_type (review_id, event_type, ts desc)

Notes:
- No separate review_batches or artifacts tables needed. Batch info and artifact pointers live in JSON events. This keeps the model simple and future-proof.
- Continue writing current file logs; store references/preview via events.

## API Design

Base path: /api/v1/reviews

- GET /api/v1/reviews
	- Query: status?, org_id?, model?, provider_type?, connector_name?, from?, to?, page?, per_page?
	- Returns: paginated list of reviews with summary fields.

- GET /api/v1/reviews/{id}
	- Returns: review detail composed from existing tables + recent event-derived summary (counts, last status, durations). Batch info is derived from events.

- GET /api/v1/reviews/{id}/stream
	- SSE or WebSocket streaming of live log events and progress updates.
	- Event schema:
		- type: "status" | "log" | "batch" | "artifact" | "completion"
		- payload: JSON (see Event Schema section)

- GET /api/v1/reviews/{id}/events
	- Returns: recent events (cursor or timestamp-based; used for polling fallback).

- GET /api/v1/reviews/{id}/artifacts?kind=prompt|response|diff
	- Optional: if large artifacts are exposed; otherwise artifacts are referenced as URLs inside event payloads.

- POST /api/v1/reviews/{id}/retry
	- Retries a failed review (auth + permission required).

Auth: Same JWT/org model as existing endpoints. Rate-limit streaming endpoints per user/org.

## Event Schema (Streaming)

Common envelope:
```
{
	"type": "status" | "log" | "batch" | "artifact" | "completion",
	"time": "2025-09-24T11:44:44.601Z",
	"reviewId": "frontend-review-…",
	"data": { ... }
}
```

Types:
- status:
	- data: { status: "running" | "success" | "failed", startedAt?, finishedAt?, durationMs? }
- log:
	- data: { level: "info"|"warn"|"error"|"debug", message: string }
- batch:
	- data: { batchId: "batch-1", status, tokenEstimate?, fileCount?, startedAt?, finishedAt? }
- artifact:
	- data: { kind: "prompt"|"response"|"diff", batchId?, sizeBytes?, previewHead?, previewTail?, url }
- completion:
	- data: { resultSummary, commentCount, errorSummary? }

Transport:
- Prefer SSE for simplicity (auto-reconnect, backoff). Support WebSocket as an option later.
- If streaming fails (proxy buffering), UI falls back to polling logs endpoint every 5–10s.

## UI/UX

Pages/components:

1) Reviews List
- Columns: Created, Status, Title (from MR/PR), Provider, Model, Duration, Last Update.
- Filters: Status, Connector, Model, Date range; search by URL or ID.
- Row actions: Open Progress, Copy Link, Download Summary, Retry (if failed).

2) Review Detail / Live Progress
- Header: Title, URL, Status pill, duration, connector+model, org.
- Live timeline (SSE):
	- Milestones: Queued → Running → Batch N → Fallback used → Parsing → Completed
	- Realtime logs panel with level filters (info/warn/error)
	- “Prompt Preview” and “Response Preview” sections (head/tail), with a “View Full” link to download artifact.
	- Batches table: Batch ID, files, token estimate, started/finished, status.
	- Final results: summary and comment count; link to posted comments in Git provider.
- Empty/Invalid JSON states:
	- Show the graceful summary we now produce, with a visible “Why” (e.g., “Truncated JSON unrecoverable”).
- Actions: Retry, Download full log, Copy review ID.

Accessibility/perf:
- Virtualized log viewer for large logs.
- Color-blind friendly status colors.

## Backend Integration

- Hook into existing ReviewLogger:
	- In addition to writing file logs, emit structured events (as defined above) and persist them to review_events; also broadcast via SSE.
	- When LogRequest/LogResponse occurs, publish artifact events with previewHead/Tail and a URL pointing to the existing file log/artifact location.
	- When streaming starts, send status running; on Ollama fallback, publish a log + batch event indicating fallback; on completion/errors, send completion.

- Resilience:
	- If SSE not possible (proxy buffering), server still persists logs; UI falls back to polling.
	- JSON parse failures already produce a graceful result summary (no silent failure).

## Error Handling & Customer Tips

- Common cases surfaced with tips:
	- No streaming chunks → “Your proxy may buffer streams; we’ll auto-retry non-streaming.” Tip: allow streaming or use non-streaming mode.
	- Auth/token failures → “Token invalid/expired.” Tip: rotate token in settings.
	- Truncated/invalid JSON → “Model output was unstructured.” Tip: reduce batch size/model verbosity.
	- Rate limits → “Backed off due to rate limit.” Tip: retry later or adjust connector.

## Security & Privacy

- Respect org scoping for all queries and streams.
- Don’t echo full prompts by default in UI; show preview with a “View Full” gated by permission.
- Signed URLs for artifact downloads, short TTL.

## Phased Delivery

MVP (v1):
- DB tables as above, SSE endpoint, list and detail pages, basic filters.
- Store prompt/response previews + full artifacts (download).
- Fallback to polling when SSE fails.

v1.1:
- Retry action, export logs as file, better batch visualization.

v2:
- WebSocket option, advanced filtering/analytics, alerting (webhook) on systemic failures.

## Acceptance Criteria (MVP)

- Reviews list loads under 1s with pagination (10–50 per page).
- Opening a running review shows live updates within 2s of new events (SSE) or 10s (polling fallback).
- On invalid JSON, UI displays a visible summary with raw excerpt; no silent failures.
- Users can download full log and prompt/response artifacts securely.
- All endpoints enforce org scoping and JWT auth.

## Open Questions

- Artifact storage: local disk vs S3 (recommend S3-compatible, with signed URLs).
- Long-term log retention policy per org.
- Dark mode and theming.

