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
