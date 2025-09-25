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

- [x] Review Detail page (live)
	- Add: `hexmoshomepage/pages/livereview/reviews/[id].tsx` (page)
	- Add: `hexmoshomepage/components/reviews/ReviewDetail.tsx` (timeline, logs w/ level filters, previews, batches, summary)
	- Add: `hexmoshomepage/hooks/usePolling.ts` (polling-based updates)
	- Verify: [M] Live run shows updates; filters and previews render correctly

- [x] Permissions and privacy
	- Change: only show previews by default; wire “View Full” to artifact URL
	- Verify: [M] Manual confirm (based on current auth model)

## Phase 5 — Hardcore Resiliency & Error Recovery

- [x] JSON Response Repair Infrastructure
	- Add: `LiveReview/internal/llm/json_repair.go`
		- Implement: `go get github.com/kaptinlin/jsonrepair` library integrated
		- Functions: RepairJSON(raw string) (repaired string, stats JsonRepairStats, error)
		- JsonRepairStats: OriginalBytes, RepairedBytes, CommentsLost, FieldsRecovered, ErrorsFixed
	- Add: `LiveReview/internal/llm/response_processor.go`
		- ProcessLLMResponse(raw string) (parsed interface{}, repair JsonRepairStats, error)
		- Clear before/after logging with repair statistics
	- Add: `LiveReview/internal/ai/langchain/json_repair_integration.go`
		- Integrated with existing LangChain provider parseResponse calls
	- Verify: [x] Unit tests with malformed JSON samples; repair stats accurate; active in production

- [x] Retry & Exponential Backoff Infrastructure  
	- Add: `LiveReview/internal/retry/backoff.go`
		- RetryConfig: MaxRetries (default 3), BaseDelay (default 1s), MaxDelay (default 30s), Multiplier (default 2.0)
		- RetryWithBackoff(ctx, config, operation func() error) (attempts int, totalDuration time.Duration, error)
	- Add: `LiveReview/internal/llm/resilient_client.go`
		- Wrapper around LLM client with retry logic, timeout handling, circuit breaker patterns
		- Detailed logging for each retry attempt with timing and reason
	- Verify: [x] Unit tests simulate failures; exponential backoff timing correct

- [x] Enhanced Event Types for Resiliency
	- Add to event types: "retry", "json_repair", "timeout", "circuit_breaker", "batch_stats"
	- Extend JSON contract:
		```json
		"retry": { "attempt": 2, "reason": "timeout", "delay": "2.1s", "nextAttempt": "2025-09-24T11:46:00Z" }
		"json_repair": { "originalSize": 1024, "repairedSize": 987, "commentsLost": 3, "fieldsRecovered": 1, "repairTime": "45ms" }
		"timeout": { "operation": "llm_request", "configuredTimeout": "30s", "actualDuration": "31.2s" }
		"batch_stats": { "batchId": "batch-1", "totalRequests": 10, "successful": 7, "retries": 8, "jsonRepairs": 2, "avgResponseTime": "2.4s" }
		```
	- Update: `LiveReview/internal/api/review_events_repo.go` to handle new event types
	- Add: Helper functions for creating resiliency events with proper JSON payloads
	- Verify: [x] New event types persist and query correctly; tested with comprehensive test suite

## Phase 6 — Advanced Log UI & Timeline Design

- [x] Intelligent Log Grouping & Statistics
	- Add: `LiveReview/ui/src/components/reviews/LogAnalytics.tsx` ✅
		- Summary cards: Total batches, Success rate, Avg response time, JSON repairs needed
		- Quick stats: Retries required, Timeouts encountered, Circuit breaker trips
		- Visual indicators for batch completion progress with color coding
		- Health status badges with intelligent insights and recommendations
		- Dark theme styling consistent with existing UIPrimitives
	- Add: `LiveReview/ui/src/components/reviews/BatchSummary.tsx` ✅
		- Collapsible batch sections with timing information
		- Batch-level statistics: success/retry/repair counts
		- Visual timeline showing batch overlap and dependencies
		- Interactive timeline with expandable file details
		- Smart progress indicators and status badges
	- Verify: [M] Analytics provide quick overview; batch grouping reduces clutter

- [x] Modern Timeline UI with Smart Whitespace
	- Redesign: `LiveReview/ui/src/components/reviews/ReviewTimeline.tsx` ✅
		- Remove excessive boxes; use subtle backgrounds and borders
		- Group related events (retry sequences, JSON repair cycles) with visual indentation
		- Smart whitespace between logical sections (batch boundaries, phase transitions)
		- Eliminate redundant "Log" labels; use event type icons and color coding
		- Live updates with auto-scroll functionality
		- Event filtering and smart grouping
	- Add visual hierarchy: ✅
		- Major milestones: Bold timestamps, larger text, accent colors
		- Batch sections: Subtle background, grouped events, collapsible details
		- Individual events: Minimal styling, focus on message content
		- Error/retry sequences: Warning colors, clear progression indicators
	- Verify: [M] Timeline is visually clean; information hierarchy clear

- [x] Enhanced Timing & Progress Visualization
	- Add: `LiveReview/ui/src/components/reviews/TimingChart.tsx` ✅
		- Horizontal timeline showing batch durations and overlaps
		- Retry attempt visualizations with backoff timing
		- JSON repair time indicators with before/after comparisons
		- Interactive hover details for precise timing information
		- Overlap detection and performance insights
		- Summary statistics with visual indicators
	- Add: `LiveReview/ui/src/components/reviews/ProgressIndicators.tsx` ✅
		- Overall review progress bar with phase indicators
		- Per-batch progress with success/retry/error breakdown
		- Real-time updates with smooth animations
		- Animated progress bars with live updates
		- Performance summary cards with key metrics
	- Verify: [M] Timing information easily understandable; progress clear

- [x] Smart Event Filtering & Search
	- Enhance: Event filtering beyond basic level filtering ✅
		- Filter by: Event type, Batch ID, Success/retry/error status, Time range
		- Search: Full-text search in log messages, JSON repair details
		- Presets: "Show only errors and retries", "Batch summary view", "Timing details"
		- Advanced time range filtering with datetime pickers
		- Real-time search with debouncing
	- Add: `LiveReview/ui/src/components/reviews/EventFilters.tsx` ✅
		- Advanced filter controls with clear visual state
		- Saved filter preferences per user
		- Quick filter buttons for common views
		- Active filter badges with individual removal
		- Expandable/collapsible filter panel
	- Verify: [M] Filtering helps users find relevant information quickly

## Phase 6.5 — Enhanced Progress Experience

- [x] Progressive Review Interface
	- Add: `LiveReview/ui/src/components/reviews/ReviewEventsPage.tsx` ✅
		- Tabbed interface with "Progress" (default) and "Raw Events" views
		- Smooth live polling without jarring page refreshes or scroll position loss
		- Auto-scroll to bottom when user is already at bottom (Raw view only)
		- Single "Live Updates On/Off" control (defaults to ON)
	- Add: `LiveReview/ui/src/components/reviews/ReviewProgressView.tsx` ✅
		- 5-stage review hierarchy: Fetching MR → Building Context → Constructing Batches → Processing Batches → Summary/Posting
		- Visual progress timeline with expandable stages and substages
		- Batch-level substages with individual progress tracking and file counts
		- Clear resiliency event indicators (retries, JSON repairs, timeouts) with resolution status
		- Improved batch ID extraction (uses details.batchId first, falls back to "general")
	- Verify: [M] Users get clear sense of progression; resiliency events show system sophistication

## Phase 6.6 — UX Refinements

- [x] Layout & Text Improvements
	- Restructure: `LiveReview/ui/src/pages/Reviews/ReviewDetail.tsx` ✅
		- Changed from 2-column to single-column layout with info panel at top
		- Review information displayed in responsive grid at top of page
		- Events section now uses full width for better log readability
	- Enhance: Text wrapping and readability ✅
		- Added `break-words` class to event messages and error messages
		- Prevented timestamp squeezing with proper flex layout
		- Improved responsive design for review information grid
	- Simplify: Remove confusing sub-tabs ✅ 
		- Removed "All Events" vs "Progress" filtering from Raw Events view
		- Raw Events now shows all logs without confusing subdivisions
		- Streamlined event display with clear visual hierarchy
	- Verify: [M] Layout supports wide logs; single clear interface without confusion

## Phase 7 — QA, Ops, and Rollout

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

- [x] Resiliency Integration Testing
	- Test JSON repair with actual malformed LLM responses
	- Verify retry logic with simulated network failures and timeouts
	- JSON repair system active and integrated with LangChain provider
	- Test new event types persist and display properly in UI
	- Verify: [x] Automated tests for resiliency features; JSON repair active in production LLM calls

- [ ] UI/UX Validation
	- Verify modern timeline UI reduces visual clutter and improves readability
	- Test analytics components provide useful insights without overwhelming details
	- Validate timing charts and progress indicators update smoothly
	- Ensure filtering and search help users navigate complex logs efficiently
	- Verify: [M] User testing confirms improved log experience; visual hierarchy effective

- [ ] Settings Page for Timeout Configuration
	- Add: `LiveReview/ui/src/pages/Settings/ResiliencySettings.tsx`
		- Form controls: LLM request timeout, retry count, base delay, max delay
		- Real-time validation with recommended ranges
		- Save to user preferences or organization settings
	- Add: `LiveReview/internal/api/settings_endpoints.go`
		- GET/PUT /api/v1/settings/resiliency with org scoping
		- Default values: RequestTimeout=30s, MaxRetries=3, BaseDelay=1s, MaxDelay=30s
	- Add: Navigation integration in settings menu
	- Verify: [M] Settings save/load correctly; values applied to retry logic

- [ ] End-to-End Testing (Pending Deployment)
	- Create test review and simulate events to verify full pipeline
	- Verify UI responsiveness with polling updates and proper error handling
	- Test create review functionality with actual API integration
	- Validate authentication and organization scoping in live environment
	- Test resiliency features under realistic failure conditions
	- Verify: [M] Deploy and test complete user workflow from creation to completion with error scenarios

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
