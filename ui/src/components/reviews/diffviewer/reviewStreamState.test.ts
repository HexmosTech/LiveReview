import {
  appendStreamedCommentsToFiles,
  extractExternalCommentsFromEvents,
  extractNewEvents,
  inferReviewStatusFromEvents,
  normalizeStreamedComment,
  sinceWithOverlap,
} from './reviewStreamState';
import { DiffReviewFile, ReviewEvent } from '../../../types/reviews';

function batchEvent(id: number, comments: unknown[], overrides: Partial<ReviewEvent> = {}): ReviewEvent {
  return {
    id,
    reviewId: 1,
    orgId: 1,
    time: `2026-01-01T00:00:0${id}Z`,
    type: 'batch',
    batchId: `batch-${id}`,
    data: { status: 'completed', comments } as any,
    ...overrides,
  };
}

describe('extractNewEvents', () => {
  it('dedupes against already-seen event ids and tracks the latest time', () => {
    const events = [batchEvent(1, []), batchEvent(2, [])];
    const first = extractNewEvents(events, undefined);
    expect(first.newEvents).toHaveLength(2);
    expect(first.lastSeenAt).toBe('2026-01-01T00:00:02Z');

    const second = extractNewEvents([...events, batchEvent(3, [])], first.nextSeenEventIds);
    expect(second.newEvents.map((e) => e.id)).toEqual([3]);
  });
});

describe('inferReviewStatusFromEvents', () => {
  it('reads the most recent status event', () => {
    const events: ReviewEvent[] = [
      { id: 1, reviewId: 1, orgId: 1, time: 't1', type: 'status', data: { status: 'in_progress' } },
      { id: 2, reviewId: 1, orgId: 1, time: 't2', type: 'status', data: { status: 'completed' } },
    ];
    expect(inferReviewStatusFromEvents(events)).toBe('completed');
  });

  it('treats a completion event with an errorSummary as failed', () => {
    const events: ReviewEvent[] = [
      { id: 1, reviewId: 1, orgId: 1, time: 't1', type: 'completion', data: { errorSummary: 'boom' } },
    ];
    expect(inferReviewStatusFromEvents(events)).toBe('failed');
  });

  it('returns empty string when no status/completion event exists', () => {
    expect(inferReviewStatusFromEvents([])).toBe('');
  });
});

describe('normalizeStreamedComment', () => {
  it('normalizes a PascalCase comment off the wire and drops internal-only comments', () => {
    const normal = normalizeStreamedComment({
      FilePath: 'a.go',
      Line: 10,
      Content: 'looks off',
      Severity: 'CRITICAL',
      Category: 'bug',
    });
    expect(normal).toEqual({
      file_path: 'a.go',
      line: 10,
      content: 'looks off',
      severity: 'critical',
      confidence: '',
      type: '',
      category: 'bug',
      subcategory: '',
    });

    expect(normalizeStreamedComment({ FilePath: 'a.go', Line: 1, Content: 'x', IsInternal: true })).toBeNull();
    expect(normalizeStreamedComment({ FilePath: '', Line: 1, Content: 'x' })).toBeNull();
    expect(normalizeStreamedComment(null)).toBeNull();
  });
});

describe('extractExternalCommentsFromEvents', () => {
  it('only reads comments off completed batch events, ignoring other event types/statuses', () => {
    const events: ReviewEvent[] = [
      batchEvent(1, [{ FilePath: 'a.go', Line: 1, Content: 'c1' }]),
      { id: 2, reviewId: 1, orgId: 1, time: 't2', type: 'batch', data: { status: 'processing' } as any },
      { id: 3, reviewId: 1, orgId: 1, time: 't3', type: 'log', data: { message: 'hi' } },
    ];
    const comments = extractExternalCommentsFromEvents(events);
    expect(comments).toEqual([{ file_path: 'a.go', line: 1, content: 'c1', severity: 'info', confidence: '', type: '', category: 'review', subcategory: '' }]);
  });
});

describe('appendStreamedCommentsToFiles', () => {
  const baseFiles: DiffReviewFile[] = [
    { file_path: 'a.go', hunks: [], comments: [] },
    { file_path: 'b.go', hunks: [], comments: [{ line: 1, content: 'existing' }] },
  ];

  it('merges incoming comments into the matching file by path, leaving others untouched', () => {
    const result = appendStreamedCommentsToFiles(baseFiles, [
      { file_path: 'a.go', line: 5, content: 'new finding' },
    ]);
    expect(result[0].comments).toEqual([{ file_path: 'a.go', line: 5, content: 'new finding' }]);
    expect(result[1].comments).toEqual([{ line: 1, content: 'existing' }]);
    // original untouched
    expect(baseFiles[0].comments).toEqual([]);
  });

  it('drops a comment whose file_path matches nothing in the file list', () => {
    const result = appendStreamedCommentsToFiles(baseFiles, [
      { file_path: 'missing.go', line: 1, content: 'orphan' },
    ]);
    expect(result[0].comments).toEqual([]);
    expect(result[1].comments).toEqual([{ line: 1, content: 'existing' }]);
  });

  it('returns the input files unchanged when there is nothing to merge', () => {
    expect(appendStreamedCommentsToFiles(baseFiles, [])).toBe(baseFiles);
    expect(appendStreamedCommentsToFiles([], [{ file_path: 'a.go', line: 1, content: 'x' }])).toEqual([]);
  });
});

describe('sinceWithOverlap', () => {
  it('subtracts the overlap window from the last-seen timestamp', () => {
    const result = sinceWithOverlap('2026-01-01T00:00:02.000Z');
    expect(result).toBe('2026-01-01T00:00:01.000Z');
  });

  it('returns undefined for null/invalid input', () => {
    expect(sinceWithOverlap(null)).toBeUndefined();
    expect(sinceWithOverlap('not-a-date')).toBeUndefined();
  });
});
