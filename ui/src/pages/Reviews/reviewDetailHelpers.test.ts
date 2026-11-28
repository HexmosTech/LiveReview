import { deriveSeverityCounts } from './reviewDetailHelpers';
import { ReviewEvent, ReviewSummary } from '../../types/reviews';

describe('deriveSeverityCounts', () => {
  const makeBatchEvent = (
    id: number,
    status: string,
    comments: Array<Record<string, any>>
  ): ReviewEvent => ({
    id,
    reviewId: 12557,
    orgId: 1,
    time: '2026-11-25T10:00:00Z',
    type: 'batch',
    batchId: `batch-${id}`,
    level: 'info',
    data: { status, comments } as any,
  });

  it('prefers non-zero summary.severityCounts when available', () => {
    const summary: Pick<ReviewSummary, 'severityCounts'> = {
      severityCounts: { high: 10, medium: 5, low: 2 },
    };

    const result = deriveSeverityCounts(
      summary,
      {
        metadata: {
          review_result: {
            comments: [{ severity: 'critical' }],
          },
        },
      },
      []
    );

    expect(result).toEqual({ high: 10, medium: 5, low: 2 });
  });

  it('falls back to review.metadata.review_result.comments when summary counts are 0, 0, 0', () => {
    const summary: Pick<ReviewSummary, 'severityCounts'> = {
      severityCounts: { high: 0, medium: 0, low: 0 },
    };

    const result = deriveSeverityCounts(
      summary,
      {
        metadata: {
          review_result: {
            comments: [
              { severity: 'critical' },
              { Severity: 'high' },
              { severity: 'warning' },
              { severity: '' },
              { severity: 'info' },
              { Severity: 'low' },
            ],
          },
        },
      },
      []
    );

    expect(result).toEqual({ high: 2, medium: 2, low: 2 });
  });

  it('falls back to completed batch event comments when review_result metadata is absent', () => {
    const summary: Pick<ReviewSummary, 'severityCounts'> = {
      severityCounts: { high: 0, medium: 0, low: 0 },
    };

    const events: ReviewEvent[] = [
      makeBatchEvent(1, 'processing', [{ severity: 'critical' }]),
      makeBatchEvent(2, 'completed', [
        { severity: 'critical' },
        { severity: 'warning' },
        { severity: 'info' },
        { severity: 'info' },
      ]),
    ];

    expect(deriveSeverityCounts(summary, { metadata: {} }, events)).toEqual({
      high: 1,
      medium: 1,
      low: 2,
    });
  });

  it('does not count generic info lifecycle logs as low severity findings', () => {
    const events: ReviewEvent[] = [
      {
        id: 1,
        reviewId: 12557,
        orgId: 1,
        time: '2026-11-25T10:00:00Z',
        type: 'status',
        level: 'info',
        data: { status: 'completed' },
      },
      {
        id: 2,
        reviewId: 12557,
        orgId: 1,
        time: '2026-11-25T10:00:01Z',
        type: 'log',
        level: 'error',
        data: { message: 'tool failed' },
      },
    ];

    expect(deriveSeverityCounts({ severityCounts: { high: 0, medium: 0, low: 0 } }, null, events)).toEqual({
      high: 1,
      medium: 0,
      low: 0,
    });
  });
});
