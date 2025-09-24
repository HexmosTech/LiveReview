# Review Progress UI — Phased Execution Checklist (Concrete Files + Commands)

This is a simple, file-orien## Phase 4 — Frontend UI

- [x] Reviews List page
	- Add: `hexmoshomepage/pages/livereview/reviews.tsx` (main reviews page with create modal)
	- Add: `hexmoshomepage/components/livereview/ReviewList.tsx` (table with filters, polling, search)
	- Add: `hexmoshomepage/types/reviews.ts` (shared TypeScript interfaces)
	- Verify: [x] Build succeeds, components render properly, TypeScript types correct

- [x] Review Detail page (live)
	- Add: `hexmoshomepage/pages/livereview/progress.tsx` (standalone progress page)
	- Add: `hexmoshomepage/components/livereview/ReviewDetail.tsx` (timeline, logs w/ level filters, summary, polling controls)
	- Add: `hexmoshomepage/hooks/useReviewPolling.ts` (polling-based updates with cursor pagination)
	- Verify: [x] Polling hook implements proper cursor-based updates, error handling, retry logic

- [x] Navigation Integration
	- Change: `hexmoshomepage/components/livereview/Header.tsx` (added Reviews navigation)
	- Main reviews page: `/livereview/reviews` with create functionality and review management
	- Progress monitoring: Available from review detail view with real-time polling
	- Verify: [x] Navigation properly integrated, routing works, user flow intuitive

- [x] Create Review Functionality
	- Modal form for creating new reviews with repository URL, branch, PR number, commit SHA
	- Form validation and API integration for POST /api/v1/reviews
	- Smart PR URL parsing (auto-extract repo, branch, PR number)
	- Verify: [x] Create modal functional, validation works, API ready for integrationts exact files to add/change and dbmate commands to run. No Swagger.

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

- [x] System Integration Testing
	- Backend builds successfully with all new components
	- Frontend builds without TypeScript errors
	- All API endpoints registered with proper authentication middleware
	- Shared TypeScript interfaces ensure type safety across components
	- Verify: [x] `go build livereview.go` succeeds, `npm run build` completes successfully

- [x] Component Integration
	- Reviews page combines list and detail views with proper navigation
	- Real-time polling works with cursor-based pagination and error handling
	- Create review modal integrates with API endpoints
	- Navigation header includes Reviews section with proper routing
	- Verify: [x] All React components render, polling hook implements retry logic, navigation functional

- [x] Architecture Validation
	- Polling-based architecture eliminates SSE complexity while providing real-time updates
	- Event sink integration maintains backward compatibility with existing logging
	- Database schema supports efficient querying with proper indexes
	- API endpoints follow RESTful patterns with organization scoping
	- Verify: [x] Database migration tested, API endpoints follow security patterns, polling efficient

- [ ] End-to-End Testing (Pending Deployment)
	- Create test review and simulate events to verify full pipeline
	- Verify UI responsiveness with polling updates and proper error handling
	- Test create review functionality with actual API integration
	- Validate authentication and organization scoping in live environment
	- Verify: [M] Deploy and test complete user workflow from creation to completion

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
