## GitHub Review Regression – 2025-10-18

### Core Problems
1. **UI says 5 events, GitHub got 41+ comments** - massive event/comment mismatch
2. **UI shows "100% complete" + "In Progress" simultaneously** - broken state logic
3. **Batch spinner stuck forever** - polling never terminates
4. **Comment quality collapsed** - bot explaining obvious code instead of finding issues
5. **Suspect review trigger mixing with reply mechanism** - possible cross-contamination

### Investigation Plan

**Step 1: Find the comment flood source**
- Check DB: `review_sessions` table for review 246 - how many batches created?
- Check `review_comments` or equivalent - how many rows inserted?
- Check logs for review 246 - grep for "posting comment" or similar
- **Goal:** confirm if backend actually posted 41 times or if UI miscounted

**Step 2: Trace review trigger flow**
- Review trigger endpoint - does it queue to job_queue or call orchestrator directly?
- Does review flow go through webhook orchestrator or separate path?
- Check if review comments use same posting mechanism as webhook replies
- **Goal:** confirm review path is isolated from webhook reply path

**Step 3: Find UI state bugs**
- Dashboard status calc - where does "100% complete" come from?
- Where does "In Progress" flag come from?
- Check batch polling logic - when does spinner stop?
- **Goal:** fix contradictory status display

**Step 4: Find prompt regression**
- Check review prompt vs webhook reply prompt - are they different?
- Did Phase 5 changes affect review prompts?
- Look for commented-out quality filters or validation
- **Goal:** restore actionable comment quality

### Files to Check (Priority Order)
1. Review trigger handler (API endpoint that starts reviews)
2. Review job processor (worker that executes review)
3. Comment posting logic (GitHub output client)
4. Dashboard status calculation
5. Review prompt builder
6. Batch status/polling endpoints

### Quick Wins
- Add logging to see exact flow when review triggered
- Add comment counter per review session with hard cap (e.g., 20)
- Separate review and webhook flows if currently mixed
- Restore original review prompt if changed recently

---

## TriggerReviewV2 Refactoring Plan (Parked)

**Current State:** 400+ line monolithic function doing 7 distinct phases

**Proposed Split:**
1. `validateAndSetupReview()` - Lines 47-105: org_id extraction, request parsing, DB record creation, logger init
2. `prepareIntegrationToken()` - Lines 107-167: URL validation, token lookup, provider validation, OAuth refresh
3. `buildReviewRequest()` - Lines 169-206: review service creation, request object building
4. `enrichReviewMetadata()` - Lines 208-308: MR metadata fetch, DB enrichment, provider normalization
5. `launchReviewProcessing()` - Lines 310-365: activity tracking, goroutine launch, completion callback setup
6. HTTP response return - Lines 367-380

**Benefits:** Easier testing, clearer error boundaries, independent phase debugging
