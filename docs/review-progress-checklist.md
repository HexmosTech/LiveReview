# Review Progress UI — Phased Execution Checklist (Concrete Files + Commands)

This is a simple, file-oriented plan. It lists exact files to add/change and dbmate commands to run. No Swagger.

Legend
- [A] Auto-checkable by script/tests
- [M] Manual check (you run and confirm)

## Phase 1 — DB Foundation (dbmate)

- [ ] Create review_events table migration (append-only JSON events)
	- Run (from `LiveReview/db`):
		```bash
		dbmate new review_events
		# paste the SQL below into the generated migration file
		dbmate up
		```
	- Paste this SQL into the generated file:
		```sql
		-- migrate:up
		CREATE TABLE IF NOT EXISTS public.review_events (
			id         bigserial PRIMARY KEY,
			review_id  bigint NOT NULL REFERENCES public.reviews(id) ON DELETE CASCADE,
			org_id     bigint NOT NULL,
			ts         timestamptz NOT NULL DEFAULT now(),
			event_type text NOT NULL,
			level      text NULL,
			batch_id   text NULL,
			data       jsonb NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_review_events_review_ts
			ON public.review_events (review_id, ts);

		CREATE INDEX IF NOT EXISTS idx_review_events_org_ts
			ON public.review_events (org_id, ts);

		CREATE INDEX IF NOT EXISTS idx_review_events_type
			ON public.review_events (review_id, event_type, ts DESC);

		-- migrate:down
		DROP INDEX IF EXISTS idx_review_events_type;
		DROP INDEX IF EXISTS idx_review_events_org_ts;
		DROP INDEX IF EXISTS idx_review_events_review_ts;
		DROP TABLE IF EXISTS public.review_events;
		```
	- Verify:
		- [A] `dbmate up` succeeds; `dbmate down && dbmate up` round-trips
		- [A] `
			-- check
			SELECT to_regclass('public.review_events');
			SELECT indexname FROM pg_indexes WHERE tablename='review_events';
			` returns expected rows

## Phase 2 — Backend Event Pipeline

- [ ] Event model + writer (persist to DB)
	- Add: `LiveReview/internal/api/review_events_repo.go`
		- Functions: InsertEvent(ctx, db, evt), ListEvents(ctx, db, reviewID, since/cursor)
		- Event struct: ReviewID, OrgID, TS, EventType, Level, BatchID, Data (json.RawMessage)
	- Verify: [A] Unit tests for insert/list (can be table-driven)

- [ ] SSE hub
	- Add: `LiveReview/internal/api/sse_hub.go`
		- In-memory hub with Subscribe(reviewID, orgID), Broadcast(evt)
		- Heartbeat ticker (e.g., 15–30s) to keep connections alive
	- Verify: [M] Connect two subscribers, broadcast test event, both receive

- [ ] Wire ReviewLogger to events
	- Change: `LiveReview/internal/logging/review_logger.go`
		- Add optional EventSink interface (SetEventSink). On Log/LogSection/LogRequest/LogResponse, emit structured events (status/log/artifact/completion) via sink
	- Add: `LiveReview/internal/api/review_events_sink.go`
		- Implements EventSink: persists to DB via repo and Broadcasts via SSE hub
	- Verify: [M] Trigger a review; confirm events appear via /events and /stream

## Phase 3 — API Endpoints

- [ ] GET /api/v1/reviews (list + filters + pagination)
	- Change/Add: `LiveReview/internal/api/reviews.go` or `reviews_api.go`
		- Ensure handler supports filters: status, connector/model (join via connector_id), date range, org scoping, pagination
	- Verify: [M] curl returns expected rows

- [ ] GET /api/v1/reviews/{id}
	- Change/Add: `LiveReview/internal/api/reviews.go` or `reviews_api.go`
		- Include synthesized summary from recent events (last status, duration, batch counts)
	- Verify: [M] curl spot-check

- [ ] GET /api/v1/reviews/{id}/events (polling)
	- Add: `LiveReview/internal/api/review_events_endpoints.go`
		- Handler returns recent events (since/cursor), org scoping
	- Verify: [M] Poll endpoint and see new events arrive

- [ ] GET /api/v1/reviews/{id}/stream (SSE)
	- Add in same `review_events_endpoints.go` or `handlers/` file
	- Verify: [M] curl -N connects and prints events live

- [ ] POST /api/v1/reviews/{id}/retry
	- Change/Add: `LiveReview/internal/api/reviews.go` or `reviews_api.go`
	- Verify: [M] retry works end-to-end

- [ ] Route registration
	- Change: `LiveReview/internal/api/server.go`
	- Verify: [M] All routes respond

## Phase 4 — Frontend UI

- [ ] Reviews List page
	- Add: `hexmoshomepage/pages/livereview/reviews/index.tsx` (page)
	- Add: `hexmoshomepage/components/reviews/ReviewList.tsx` (table with filters)
	- Verify: [M] Visual check (list, filters, pagination)

- [ ] Review Detail page (live)
	- Add: `hexmoshomepage/pages/livereview/reviews/[id].tsx` (page)
	- Add: `hexmoshomepage/components/reviews/ReviewDetail.tsx` (timeline, logs w/ level filters, previews, batches, summary)
	- Add: `hexmoshomepage/hooks/useEventStream.ts` (SSE with polling fallback)
	- Verify: [M] Live run shows updates; filters and previews render correctly

- [ ] Permissions and privacy
	- Change: only show previews by default; wire “View Full” to artifact URL
	- Verify: [M] Manual confirm (based on current auth model)

## Phase 5 — QA, Ops, and Rollout

- [ ] Seed and demo scripts
	- Output: script to create a test review and simulate events
	- Verify: [M] Open UI and observe full path to completion

- [ ] Performance checks
	- Output: events query uses proper indexes; SSE heartbeat in place
	- Verify: [A] Explain analyze for /events; [M] Observe UI responsiveness with large logs

- [ ] Docs and runbooks
	- Output: `docs/review-progress.md` updated with SSE fallback, tips, and troubleshooting
	- Verify: [M] Read-through for clarity

- [ ] Release toggle
	- Output: feature flag (env or config) to enable UI/streaming per environment
	- Verify: [M] Toggle on/off behaves as expected

---

Appendix — Review Events JSON contract (reference)

Envelope
{
	"type": "status" | "log" | "batch" | "artifact" | "completion",
	"time": "2025-09-24T11:44:44.601Z",
	"reviewId": "...",
	"data": { /* see below */ }
}

Payloads
- status: { status: "running"|"success"|"failed", startedAt?, finishedAt?, durationMs? }
- log: { level: "info"|"warn"|"error"|"debug", message: string }
- batch: { batchId: "batch-1", status, tokenEstimate?, fileCount?, startedAt?, finishedAt? }
- artifact: { kind: "prompt"|"response"|"diff", batchId?, sizeBytes?, previewHead?, previewTail?, url }
- completion: { resultSummary, commentCount, errorSummary? }

Notes
- Persist exactly this structure in `review_events.data`; keep it forward-compatible.
