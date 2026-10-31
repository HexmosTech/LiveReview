import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import ReviewProgressView from './ReviewProgressView';
import type { ReviewEvent } from './types';

// Replays review 13536's real raw event shape for a batch that exhausted
// retries and failed permanently with zero comments (confirmed via the raw
// /api/v1/reviews/13536/events payload: only repeated "request timeout" LLM
// errors for batch-6, zero completions). Before the fix, nothing ever told
// the frontend this batch had failed - it only saw a "processing" event
// (which stashed fileCount as a comment-count fallback) and transient error
// logs, so some later loosely-matched event flipped it to "completed" and
// rendered "✓ Generated 6 comments" for a batch that produced none.
const batch6ProcessingEvent: ReviewEvent = {
  id: '1',
  timestamp: '2026-10-31T04:54:45.302Z',
  eventType: 'batch',
  message: 'Batch batch-6 started: processing 6 files',
  details: { batchId: 'batch-6', status: 'processing', fileCount: 6 },
  severity: 'info',
};

const batch6TransientErrorEvent = (n: number): ReviewEvent => ({
  id: `err-${n}`,
  timestamp: '2026-10-31T05:00:00.000Z',
  eventType: 'log',
  message: 'ERROR in LLM call batch batch-6: request timeout: API call exceeded deadline',
  severity: 'error',
});

// The NEW terminal-failure event this fix adds (ReviewLogger.EmitBatchFailed,
// EmitBatchEvent status="failed") - previously never emitted at all.
const batch6TerminalFailureEvent: ReviewEvent = {
  id: 'failed-1',
  timestamp: '2026-10-31T05:04:00.000Z',
  eventType: 'batch',
  message: 'Batch batch-6 failed permanently: request timeout: API call exceeded deadline',
  details: { batchId: 'batch-6', status: 'failed' },
  severity: 'error',
};

// A stray later event that loosely matches "completed" wording while still
// mentioning batch-6 - simulates the exact kind of unrelated/stale event
// that used to flip a failed batch back to fake success.
const staleCompletedLookingEvent: ReviewEvent = {
  id: 'stale-1',
  timestamp: '2026-10-31T05:05:00.000Z',
  eventType: 'log',
  message: 'Review stage completed; batch-6 posted to summary',
  severity: 'info',
};

const expandReviewStage = () => {
  fireEvent.click(screen.getByText('Review'));
};

// Replays review 13543's real shape: the frontend fetches at most 1000
// events (ReviewDetail.tsx hardcodes that limit), but a long review can emit
// well over that, so the Finalization stage's own "stage completed"/
// "completion" events - which only ever occur right at the very end - can
// fall outside the fetched window entirely. Once the review itself stops
// being live (status leaves in_progress), the page never fetches further
// events to pick them up, so this stage would stay "in-progress" forever
// even though the review record says "completed" - exactly what the review
// detail header, the Events tab, and the overall progress bar disagreed
// about for review 13543.
const stageEvent = (message: string, id: string): ReviewEvent => ({
  id,
  timestamp: '2026-10-31T10:12:05.000Z',
  eventType: 'log',
  message,
  severity: 'info',
});

const truncatedCompletedReviewEvents: ReviewEvent[] = [
  stageEvent('Stage started: Preparation', 'p1'),
  stageEvent('Stage completed: Preparation', 'p2'),
  stageEvent('Stage started: Analysis', 'a1'),
  stageEvent('Stage completed: Analysis', 'a2'),
  stageEvent('Stage started: Review', 'r1'),
  stageEvent('Stage completed: Review', 'r2'),
  stageEvent('Stage started: Artifact Generation', 'g1'),
  stageEvent('Stage completed: Artifact Generation', 'g2'),
  // Finalization only ever got its start marker in this truncated window -
  // its own completion/"review process completed" event landed past event
  // #1000 and was never fetched.
  stageEvent('Stage started: Finalization', 'f1'),
];

describe('ReviewProgressView - batch failure truth (review 13536 regression)', () => {
  it('renders a batch that failed outright as Failed, never as fake success', () => {
    render(
      <ReviewProgressView
        reviewId={13536}
        events={[
          batch6ProcessingEvent,
          batch6TransientErrorEvent(1),
          batch6TransientErrorEvent(2),
          batch6TransientErrorEvent(3),
          batch6TerminalFailureEvent,
        ]}
      />
    );
    expandReviewStage();

    expect(screen.getByText('✗ Failed')).toBeInTheDocument();
    expect(screen.queryByText(/✓ Generated/)).not.toBeInTheDocument();
    expect(screen.queryByText(/6 comments?/)).not.toBeInTheDocument();
  });

  it('does not let a later loosely-matched event flip a failed batch back to completed', () => {
    render(
      <ReviewProgressView
        reviewId={13536}
        events={[
          batch6ProcessingEvent,
          batch6TransientErrorEvent(1),
          batch6TerminalFailureEvent,
          staleCompletedLookingEvent,
        ]}
      />
    );
    expandReviewStage();

    expect(screen.getByText('✗ Failed')).toBeInTheDocument();
    expect(screen.queryByText(/✓ Generated/)).not.toBeInTheDocument();
  });

  it('renders a batch that genuinely completed with its real comment count', () => {
    const completedEvent: ReviewEvent = {
      id: 'completed-1',
      timestamp: '2026-10-31T05:01:00.000Z',
      eventType: 'batch',
      message: 'Batch batch-2 completed: generated 3 comments',
      details: { batchId: 'batch-2', status: 'completed', commentCount: 3 },
      severity: 'success',
    };
    const processingEvent: ReviewEvent = {
      id: 'processing-2',
      timestamp: '2026-10-31T04:54:45.000Z',
      eventType: 'batch',
      message: 'Batch batch-2 started: processing 9 files',
      details: { batchId: 'batch-2', status: 'processing', fileCount: 9 },
      severity: 'info',
    };

    render(<ReviewProgressView reviewId={13536} events={[processingEvent, completedEvent]} />);
    expandReviewStage();

    // Must show the real comment count (3), never the unrelated file count (9).
    expect(screen.getByText('✓ Generated 3 comments')).toBeInTheDocument();
    expect(screen.queryByText(/9 comments?/)).not.toBeInTheDocument();
  });
});

describe('ReviewProgressView - stuck-stage truth once the review is actually done (review 13543 regression)', () => {
  it('without reviewStatus, a stage missing its completion event (outside the fetched window) stays stuck in-progress', () => {
    render(<ReviewProgressView reviewId={13543} events={truncatedCompletedReviewEvents} />);

    expect(screen.getByText('90%')).toBeInTheDocument();
    fireEvent.click(screen.getByText('Finalization'));
    expect(screen.getByText('in-progress')).toBeInTheDocument();
  });

  it('once reviewStatus is "completed", a stuck stage resolves to completed and the bar reaches 100%', () => {
    render(
      <ReviewProgressView
        reviewId={13543}
        events={truncatedCompletedReviewEvents}
        reviewStatus="completed"
      />
    );

    expect(screen.getByText('100%')).toBeInTheDocument();
    fireEvent.click(screen.getByText('Finalization'));
    expect(screen.queryByText('in-progress')).not.toBeInTheDocument();
  });
});
