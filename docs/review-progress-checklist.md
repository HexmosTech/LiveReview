# Review Progress UI — Phased Execution Checklist (Concrete Files + Commands)

This is a simple, file-oriented plan. It lists exact files to add/change and dbmate commands to run. No Swagger.

Legend
- [A] Auto-checkable by script/tests
- [M] Manual check (you run and confirm)

## Phase 1 — DB Foundation (dbmate)

- [x] Create review_events table migration (append-only JSON events)
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
		- [x] `dbmate up` succeeds; `dbmate down && dbmate up` round-trips
		- [x] `
			-- check
			SELECT to_regclass('public.review_events');
			SELECT indexname FROM pg_indexes WHERE tablename='review_events';
			` returns expected rows

## Phase 2 — Backend Event Pipeline

- [x] Event model + writer (persist to DB)
	- Add: `LiveReview/internal/api/review_events_repo.go`
		- Functions: InsertEvent(ctx, db, evt), ListEvents(ctx, db, reviewID, since/cursor)
		- Event struct: ReviewID, OrgID, TS, EventType, Level, BatchID, Data (json.RawMessage)
	- Verify: [x] Unit tests for insert/list (can be table-driven)

- [x] Polling Event Service (replaces SSE hub)
	- Add: `LiveReview/internal/api/polling_event_service.go`
		- Simple polling-based event storage and retrieval
		- Helper methods for creating different event types
	- Verify: [x] Unit tests for event creation and retrieval

- [x] Wire ReviewLogger to events
	- Change: `LiveReview/internal/logging/review_logger.go`
		- Add optional EventSink interface (SetEventSink). On Log/LogSection/LogRequest/LogResponse, emit structured events (status/log/artifact/completion) via sink
	- Add: `LiveReview/internal/api/review_events_sink.go`
		- Implements EventSink: persists to DB via repo and polling service
	- Verify: [x] Unit tests pass; events are emitted and stored correctly

## Phase 3 — API Endpoints

- [x] ~~GET /api/v1/reviews (list + filters + pagination)~~ (deferred - using existing reviews table)
	- ~~Change/Add: `LiveReview/internal/api/reviews.go` or `reviews_api.go`~~
		- ~~Ensure handler supports filters: status, connector/model (join via connector_id), date range, org scoping, pagination~~
	- Note: Using existing review management; focus on events endpoints

- [x] GET /api/v1/reviews/{id}/summary (enhanced with events)
	- Add: `LiveReview/internal/api/review_events_endpoints.go`
		- Include synthesized summary from recent events (last status, duration, batch counts)
	- Verify: [x] Unit tests pass; summary includes event data

- [x] GET /api/v1/reviews/{id}/events (polling)
	- Add: `LiveReview/internal/api/review_events_endpoints.go`
		- Handler returns recent events (since/cursor), org scoping
	- Verify: [x] Unit tests pass; polling endpoint works with cursor pagination

- [ ] ~~GET /api/v1/reviews/{id}/stream (SSE)~~ (skipped - using polling only)
	- ~~Add in same `review_events_endpoints.go` or `handlers/` file~~
	- ~~Verify: [M] curl -N connects and prints events live~~

- [ ] POST /api/v1/reviews/{id}/retry (deferred to Phase 4)
	- Change/Add: `LiveReview/internal/api/reviews.go` or `reviews_api.go`
	- Verify: [M] retry works end-to-end
	- Note: Will implement with frontend integration

- [x] Route registration
	- Change: `LiveReview/internal/api/server.go`
		- Added reviewsGroup with proper auth middleware
		- Registered all events endpoints with org scoping
	- Verify: [x] Unit tests pass; routes compile and respond correctly

## Phase 4 — Frontend UI

- [ ] Reviews List page
	- Add: `hexmoshomepage/pages/livereview/reviews/index.tsx` (page)
	- Add: `hexmoshomepage/components/reviews/ReviewList.tsx` (table with filters)
	- Verify: [M] Visual check (list, filters, pagination)

- [ ] Review Detail page (live)
	- Add: `hexmoshomepage/pages/livereview/reviews/[id].tsx` (page)
	- Add: `hexmoshomepage/components/reviews/ReviewDetail.tsx` (timeline, logs w/ level filters, previews, batches, summary)
	- Add: `hexmoshomepage/hooks/usePolling.ts` (polling-based updates)
	- Verify: [M] Live run shows updates; filters and previews render correctly

- [ ] Permissions and privacy
	- Change: only show previews by default; wire “View Full” to artifact URL
	- Verify: [M] Manual confirm (based on current auth model)

## Phase 5 — QA, Ops, and Rollout

- [ ] Seed and demo scripts
	- Output: script to create a test review and simulate events
	- Verify: [M] Open UI and observe full path to completion

- [ ] Performance checks
	- Output: events query uses proper indexes; polling efficiency optimized
	- Verify: [A] Explain analyze for /events; [M] Observe UI responsiveness with polling

- [ ] Docs and runbooks
	- Output: `docs/review-progress.md` updated with polling approach, tips, and troubleshooting
	- Verify: [M] Read-through for clarity

- [ ] Release toggle
	- Output: feature flag (env or config) to enable UI/polling per environment
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
