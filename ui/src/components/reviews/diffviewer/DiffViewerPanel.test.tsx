// Component-level regression test for the "findings disappear mid-review" bug
// (see reviewStreamState.ts's shouldClearStreamedComments doc comment for the
// full root-cause writeup). Exercises the real DiffViewerPanel end-to-end
// against mocked API responses, with fake timers driving the 1s event-poll
// loop exactly as it runs in production.
import React from 'react';
import { act, render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import DiffViewerPanel from './DiffViewerPanel';
import { getDiffReview, getReviewEvents } from '../../../api/reviews';
import { DiffReviewStatusResponse, ReviewEvent, ReviewEventsResponse } from '../../../types/reviews';

jest.mock('../../../api/reviews');
jest.mock('../../../api/githubApp', () => ({
  getGitHubAppInstallCheck: jest.fn(),
  startGitHubAppConnect: jest.fn(),
}));
jest.mock('../../../utils/redirectOverlay', () => ({ showRedirectOverlay: jest.fn() }));
jest.mock('../../../utils/notify', () => ({ notify: { success: jest.fn(), error: jest.fn() } }));

const mockGetDiffReview = getDiffReview as jest.Mock;
const mockGetReviewEvents = getReviewEvents as jest.Mock;

function renderPanel(reviewId = 1) {
  return render(
    <MemoryRouter>
      <DiffViewerPanel reviewId={reviewId} />
    </MemoryRouter>
  );
}

function inProgressResponse(): DiffReviewStatusResponse {
  return {
    status: 'in_progress',
    review_id: '1',
    files: [
      { file_path: 'a.go', hunks: [], comments: [] },
      { file_path: 'b.go', hunks: [], comments: [] },
    ],
  };
}

function batchEvent(id: number, filePath: string, count: number, startLine = 1): ReviewEvent {
  const comments = Array.from({ length: count }, (_, i) => ({
    FilePath: filePath,
    Line: startLine + i,
    Content: `finding ${filePath}:${startLine + i}`,
    Severity: 'warning',
  }));
  return {
    id,
    reviewId: 1,
    orgId: 1,
    time: new Date(2026, 0, 1, 0, 0, id).toISOString(),
    type: 'batch',
    batchId: `batch-${id}`,
    data: { status: 'completed', comments } as any,
  };
}

function eventsResponse(events: ReviewEvent[]): ReviewEventsResponse {
  return { events, meta: { reviewId: 1, count: events.length, limit: 1000 } };
}

async function flush() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe('DiffViewerPanel - findings streaming / disappearing-comments regression', () => {
  beforeEach(() => {
    jest.useFakeTimers({ legacyFakeTimers: false });
    jest.clearAllMocks();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('keeps all 8 streamed comments visible when the review ends "failed" with an empty result (the exact reported bug)', async () => {
    mockGetDiffReview.mockResolvedValueOnce(inProgressResponse());
    // First poll: 5 comments from a.go's batch. Second poll: 3 more from
    // b.go's batch, plus the terminal "failed" status - mirroring one bad
    // Gemini batch wiping review_result after other batches already streamed.
    mockGetReviewEvents
      .mockResolvedValueOnce(eventsResponse([batchEvent(1, 'a.go', 5)]))
      .mockResolvedValueOnce(
        eventsResponse([
          batchEvent(2, 'b.go', 3),
          { id: 3, reviewId: 1, orgId: 1, time: new Date(2026, 0, 1, 0, 0, 3).toISOString(), type: 'completion', data: { errorSummary: 'one batch failed' } },
        ])
      )
      .mockResolvedValue(eventsResponse([]));
    mockGetDiffReview.mockResolvedValueOnce({
      status: 'failed',
      review_id: '1',
      message: 'one batch failed',
      files: [
        { file_path: 'a.go', hunks: [], comments: [] },
        { file_path: 'b.go', hunks: [], comments: [] },
      ],
    });

    renderPanel();
    await flush();

    for (let i = 0; i < 3; i++) {
      await act(async () => { jest.advanceTimersByTime(1000); });
      await flush();
    }

    // The bug: all 8 comments must still be visible, not wiped to 0.
    await waitFor(() => expect(screen.getByText('— / 8')).toBeInTheDocument());
    expect(screen.getByText(/This review failed before finishing/)).toBeInTheDocument();
  });

  it('trusts the smaller final set on "completed" (legitimate backend dedup/cap is not padded back out by the stream)', async () => {
    mockGetDiffReview.mockResolvedValueOnce(inProgressResponse());
    mockGetReviewEvents
      .mockResolvedValueOnce(eventsResponse([batchEvent(1, 'a.go', 4)]))
      .mockResolvedValueOnce(
        eventsResponse([
          { id: 2, reviewId: 1, orgId: 1, time: new Date(2026, 0, 1, 0, 0, 2).toISOString(), type: 'completion', data: {} },
        ])
      )
      .mockResolvedValue(eventsResponse([]));
    // Final, authoritative result: AggregateAndCombineOutputs deduped down to 2.
    mockGetDiffReview.mockResolvedValueOnce({
      status: 'completed',
      review_id: '1',
      files: [
        { file_path: 'a.go', hunks: [], comments: [
          { file_path: 'a.go', line: 1, content: 'deduped finding 1' },
          { file_path: 'a.go', line: 2, content: 'deduped finding 2' },
        ] },
        { file_path: 'b.go', hunks: [], comments: [] },
      ],
    });

    renderPanel();
    await flush();

    for (let i = 0; i < 3; i++) {
      await act(async () => { jest.advanceTimersByTime(1000); });
      await flush();
    }

    await waitFor(() => expect(screen.getByText('— / 2')).toBeInTheDocument());
  });

  it('keeps showing streamed comments through a network error on every poll, instead of clearing or crashing', async () => {
    mockGetDiffReview.mockResolvedValueOnce(inProgressResponse());
    mockGetReviewEvents
      .mockResolvedValueOnce(eventsResponse([batchEvent(1, 'a.go', 3)]))
      .mockRejectedValue(new Error('network down'));

    renderPanel();
    await flush();
    await act(async () => { jest.advanceTimersByTime(1000); });
    await flush();
    expect(screen.getByText('— / 3')).toBeInTheDocument();

    for (let i = 0; i < 3; i++) {
      await act(async () => { jest.advanceTimersByTime(1000); });
      await flush();
    }
    // still 3 - the repeated poll failures neither cleared the stream nor crashed the component
    expect(screen.getByText('— / 3')).toBeInTheDocument();
  });

  it('does not duplicate comments when the same event page is effectively re-delivered (overlap window)', async () => {
    mockGetDiffReview.mockResolvedValueOnce(inProgressResponse());
    const firstBatch = batchEvent(1, 'a.go', 2);
    mockGetReviewEvents
      .mockResolvedValueOnce(eventsResponse([firstBatch]))
      // Same event id reappears (overlap window), nothing genuinely new.
      .mockResolvedValueOnce(eventsResponse([firstBatch]))
      .mockResolvedValue(eventsResponse([]));

    renderPanel();
    await flush();
    await act(async () => { jest.advanceTimersByTime(1000); });
    await flush();
    expect(screen.getByText('— / 2')).toBeInTheDocument();

    await act(async () => { jest.advanceTimersByTime(1000); });
    await flush();
    // Still 2, not 4 - extractNewEvents' seen-id set deduped the replay.
    expect(screen.getByText('— / 2')).toBeInTheDocument();
  });
});
