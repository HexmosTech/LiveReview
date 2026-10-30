import fc from 'fast-check';
import {
  appendStreamedCommentsToFiles,
  extractExternalCommentsFromEvents,
  extractNewEvents,
  inferReviewStatusFromEvents,
  normalizeStreamedComment,
  shouldClearStreamedComments,
  sinceWithOverlap,
} from './reviewStreamState';
import { DiffReviewComment, DiffReviewFile, ReviewEvent } from '../../../types/reviews';

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

// Regression coverage for the "findings disappear mid-review" bug: a single
// failed batch wipes review_result entirely (internal/ai/gemini/batch.go /
// internal/jobqueue/review_worker.go), so the final getDiffReview fetch on a
// 'failed' review can legitimately have fewer comments than already
// streamed. See DiffViewerPanel.tsx's polling effect and this function's own
// doc comment for the full root-cause writeup.
describe('shouldClearStreamedComments', () => {
  it('clears on completed - the final result is authoritative (dedup/cap already ran correctly)', () => {
    expect(shouldClearStreamedComments('completed')).toBe(true);
  });

  it('does NOT clear on failed - a single bad batch can wipe review_result despite other batches having already streamed real comments', () => {
    expect(shouldClearStreamedComments('failed')).toBe(false);
  });
});

describe('appendStreamedCommentsToFiles - adversarial/edge cases', () => {
  it('with empty streamedComments and a failed review, produces no comments (not a false positive)', () => {
    const files: DiffReviewFile[] = [{ file_path: 'a.go', hunks: [], comments: [] }];
    expect(appendStreamedCommentsToFiles(files, [])).toEqual(files);
  });

  it('re-attaches streamed comments onto the failed-path shape (buildDiffFiles(preloaded, nil): files present, every comments[] empty)', () => {
    // internal/api/diff_review.go:197-223 - any non-'completed' status, including
    // 'failed', returns this exact shape: the correct file/hunk structure with
    // zero comments per file (nil was passed as the comments arg).
    const failedPathFiles: DiffReviewFile[] = [
      { file_path: 'a.go', hunks: [{} as any], comments: [] },
      { file_path: 'b.go', hunks: [{} as any], comments: [] },
    ];
    const streamed = [
      { file_path: 'a.go', line: 3, content: 'already streamed before the batch that failed' },
      { file_path: 'b.go', line: 9, content: 'also streamed, different file' },
    ];
    const result = appendStreamedCommentsToFiles(failedPathFiles, streamed);
    expect(result[0].comments).toEqual([streamed[0]]);
    expect(result[1].comments).toEqual([streamed[1]]);
  });

  it('does not double-append the same streamed comment across overlapping polls (dedup happens upstream in extractNewEvents, this just verifies append is additive, not idempotent on its own, so callers must only pass genuinely-new comments)', () => {
    const files: DiffReviewFile[] = [{ file_path: 'a.go', hunks: [], comments: [] }];
    const comment = { file_path: 'a.go', line: 1, content: 'dup' };
    // Simulates what WOULD happen if a caller accidentally passed the same
    // comment twice - appendStreamedCommentsToFiles itself has no dedup, so
    // the real protection is extractNewEvents' seenEventIds set (tested
    // below). This test documents that boundary explicitly.
    const result = appendStreamedCommentsToFiles(files, [comment, comment]);
    expect(result[0].comments).toHaveLength(2);
  });
});

describe('extractNewEvents - adversarial/edge cases', () => {
  it('dedupes the same event id seen across overlapping poll pages (the real since-overlap protection)', () => {
    const events = [batchEvent(1, [{ FilePath: 'a.go', Line: 1, Content: 'c1' }])];
    const first = extractNewEvents(events, undefined);
    // Second poll's overlap window re-returns the same event (plus a genuinely new one).
    const second = extractNewEvents([events[0], batchEvent(2, [])], first.nextSeenEventIds);
    expect(second.newEvents.map((e) => e.id)).toEqual([2]);
  });

  it('still dedupes correctly when an event has no id (falls back to a composite type/time/batchId/data key)', () => {
    const malformed: ReviewEvent = {
      reviewId: 1, orgId: 1, time: 't1', type: 'batch', batchId: 'b1',
      data: { status: 'completed', comments: [{ FilePath: 'a.go', Line: 1, Content: 'x' }] } as any,
    } as any; // no `id` field at all
    const first = extractNewEvents([malformed], undefined);
    expect(first.newEvents).toHaveLength(1);
    // Same malformed event re-appears (e.g. re-fetched in the next page) - same
    // composite key, so it must be recognized as already-seen, not duplicated.
    const second = extractNewEvents([malformed], first.nextSeenEventIds);
    expect(second.newEvents).toHaveLength(0);
  });

  it('treats a completion event immediately followed by a straggler batch event in the same page as two distinct new events (both surfaced, nothing silently dropped)', () => {
    const events: ReviewEvent[] = [
      { id: 1, reviewId: 1, orgId: 1, time: 't1', type: 'completion', data: { errorSummary: 'boom' } },
      batchEvent(2, [{ FilePath: 'a.go', Line: 1, Content: 'late straggler' }]),
    ];
    const { newEvents } = extractNewEvents(events, undefined);
    expect(newEvents).toHaveLength(2);
    // Both the failure and the straggler's comment are extractable from this page -
    // DiffViewerPanel's poll handler processes extractExternalCommentsFromEvents
    // and inferReviewStatusFromEvents from the SAME newEvents array, so the
    // straggler's comment is captured before/alongside the terminal-status handling,
    // not lost to it.
    expect(extractExternalCommentsFromEvents(newEvents)).toHaveLength(1);
    expect(inferReviewStatusFromEvents(newEvents)).toBe('failed');
  });
});

// Property-based test: generates hundreds of randomized streaming/completion
// scenarios and asserts the core invariant that fixes the disappearing-
// comments bug - a 'failed' review's rendered comments are never fewer than
// what had already streamed, by (file_path, line, content) identity.
describe('property: findings never shrink on a failed review (fast-check)', () => {
  const commentArb = fc.record({
    file_path: fc.constantFrom('a.go', 'b.go', 'c.go'),
    line: fc.integer({ min: 1, max: 50 }),
    content: fc.string({ minLength: 1, maxLength: 20 }),
  }) as fc.Arbitrary<DiffReviewComment & { file_path: string }>;

  function identity(c: DiffReviewComment & { file_path: string }): string {
    return `${c.file_path}|${c.line}|${c.content}`;
  }

  it('never loses a streamed comment once the review ends in "failed"', () => {
    fc.assert(
      fc.property(
        fc.array(commentArb, { minLength: 0, maxLength: 12 }), // everything that streamed in live
        fc.array(commentArb, { minLength: 0, maxLength: 12 }), // the final fetch's comment set (arbitrary - may be a subset, superset, or disjoint)
        (streamed, finalComments) => {
          const files: DiffReviewFile[] = [
            { file_path: 'a.go', hunks: [], comments: [] },
            { file_path: 'b.go', hunks: [], comments: [] },
            { file_path: 'c.go', hunks: [], comments: [] },
          ];
          const byFile = new Map(files.map((f) => [f.file_path, f]));
          finalComments.forEach((c) => byFile.get(c.file_path)?.comments.push(c));

          const shouldClear = shouldClearStreamedComments('failed');
          const effectiveStreamed = shouldClear ? [] : streamed;
          const rendered = appendStreamedCommentsToFiles(files, effectiveStreamed);
          const renderedIdentities = new Set(rendered.flatMap((f) => f.comments.map((c) => identity(c as any))));

          for (const s of streamed) {
            if (byFile.has(s.file_path)) {
              // every streamed comment (for a file that exists in the final
              // response) must still be visible after reconciliation
              expect(renderedIdentities.has(identity(s))).toBe(true);
            }
          }
        }
      ),
      { numRuns: 250 }
    );
  });

  it('on "completed", trusts the final set as-is (no streamed-only comments survive, no duplicates invented)', () => {
    fc.assert(
      fc.property(
        fc.array(commentArb, { minLength: 0, maxLength: 12 }),
        fc.array(commentArb, { minLength: 0, maxLength: 12 }),
        (streamed, finalComments) => {
          const files: DiffReviewFile[] = [
            { file_path: 'a.go', hunks: [], comments: [] },
            { file_path: 'b.go', hunks: [], comments: [] },
            { file_path: 'c.go', hunks: [], comments: [] },
          ];
          const byFile = new Map(files.map((f) => [f.file_path, f]));
          finalComments.forEach((c) => byFile.get(c.file_path)?.comments.push(c));

          const shouldClear = shouldClearStreamedComments('completed');
          expect(shouldClear).toBe(true);
          const effectiveStreamed = shouldClear ? [] : streamed;
          const rendered = appendStreamedCommentsToFiles(files, effectiveStreamed);
          const renderedIdentities = rendered.flatMap((f) => f.comments.map((c) => identity(c as any)));

          const finalIdentities = finalComments.map(identity);
          expect(renderedIdentities.sort()).toEqual(finalIdentities.sort());
        }
      ),
      { numRuns: 250 }
    );
  });
});
