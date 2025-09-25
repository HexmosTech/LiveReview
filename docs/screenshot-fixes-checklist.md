# Screenshot Issues Fix Checklist - Sep 25, 2025

## CRITICAL ISSUES TO FIX

### 1. ❌ Batch Events Not Fully Visible (Image 1)
- **Problem**: Review stage shows "43 events" but Recent Events section only shows ~5 events
- **Root Cause**: Event filtering/limiting in collapsed view (likely slice(-5) limitation)
- **Location**: ReviewProgressView.tsx - Recent Events section
- **Fix**: Show more events or add "Show All" button/pagination

### 2. ❌ Status Inconsistency (Images 2 vs 3-4)
- **Problem**: List shows "Completed" but detail shows "in-progress" (58% complete)
- **Root Cause**: Different status calculation logic between components
- **Locations**: Reviews list vs ReviewProgressView
- **Fix**: Unify status calculation logic

### 3. ❌ Progress Math Error (Image 4)
- **Problem**: "2 completed, 1 in progress, 2 pending" = 58% (should be ~50%)
- **Expected**: 2/5 = 40% + in-progress bonus = ~50%
- **Location**: ReviewProgressView.tsx - overallProgress calculation
- **Fix**: Correct the percentage math

### 4. ❌ Broken Tab Icons (Image 5)
- **Problem**: Progress/Raw tab icons not rendering (showing broken icon placeholders)
- **Root Cause**: Icon component issues in ReviewEventsPage tabs
- **Location**: ReviewEventsPage.tsx - tab header icons
- **Fix**: Correct icon usage/imports

### 5. ❌ Duplicate Polling Controls (Image 6)
- **Problem**: "Live Updates On" + separate polling controls = confusing UX
- **Root Cause**: Multiple polling implementations not consolidated
- **Location**: ReviewEventsPage.tsx + ReviewDetail.tsx
- **Fix**: Single 30s polling, remove duplicates

### 6. ❌ Disruptive Reloads
- **Problem**: Entire view reloads instead of smooth append
- **Root Cause**: Full re-render instead of incremental updates
- **Location**: Event polling logic
- **Fix**: Append-only updates, preserve scroll position

---

## EXECUTION PLAN

- [x] **Priority 1**: Fix broken tab icons (quick visual fix) ✅ COMPLETED
- [x] **Priority 2**: Fix event visibility in batch sections ✅ COMPLETED  
- [x] **Priority 3**: Correct progress percentage calculation ✅ COMPLETED
- [ ] **Priority 4**: Fix status inconsistency (backend vs UI calculation difference)
- [x] **Priority 5**: Consolidate polling mechanism ✅ COMPLETED
- [x] **Priority 6**: Implement smooth append-only updates ✅ COMPLETED

## Files to Modify
- `ui/src/components/reviews/ReviewProgressView.tsx`
- `ui/src/components/reviews/ReviewEventsPage.tsx`
- `ui/src/pages/Reviews/ReviewDetail.tsx`
- `ui/src/pages/Reviews/index.tsx` (list status logic)

## Testing Checklist
- [ ] Tab icons display correctly
- [ ] All 43 events visible in batch section
- [ ] Progress % matches stage counts
- [ ] Status consistent between list and detail
- [ ] Single polling control, 30s interval
- [ ] Smooth updates without page disruption