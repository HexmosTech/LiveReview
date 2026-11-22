// Mirror of git-lrc's blast_math_model.test.mjs, against synthetic detail
// objects with the same numbers as the real review 11632 artifacts. The Go
// tests (internal/blastradius/mathmode_test.go) pin the real fixtures; these
// tests keep the UI's derivation and step numbering identical.

import { buildMathMode, detectCombineMode, sumExpression } from './blastMath';
import { BlastRadiusHunkReport } from '../types/reviews';

function makeDetail(overrides: Partial<BlastRadiusHunkReport> = {}): BlastRadiusHunkReport {
  return {
    FilePath: 'a.go',
    Header: '@@ -1,1 +1,2 @@',
    NewStart: 5,
    NewLines: 2,
    BlastRadiusRaw: 0,
    BlastRadiusNorm: 0,
    MaxBlastRadiusRaw: 0,
    ReviewPriorityRaw: 0,
    ReviewPriorityNorm: 0,
    MaxReviewPriorityRaw: 0,
    Combined: 0,
    HygieneMultiplier: 1,
    Weights: { BlastRadius: 0.6, ReviewPriority: 0.4 },
    ...overrides,
  };
}

const near = (got: number, want: number, tolerance = 1e-9) => expect(Math.abs(got - want)).toBeLessThanOrEqual(tolerance);

const symbol = (name: string, blast: number, priority: number) => ({
  QualifiedName: `svc.${name}`,
  Name: name,
  Label: 'Function',
  Method: 'calls',
  Signals: [
    { Name: 'Caller reach', Points: blast, Category: 'graph' },
    { Name: 'Test coverage', Points: priority, Category: 'code-metrics' },
  ],
  BlastRadiusRaw: blast,
  ReviewPriorityRaw: priority,
  DirectCount: 0,
  TransitiveCount: 0,
});

describe('blastMath legacy (linear) artifacts', () => {
  const detail = makeDetail({
    BlastRadiusRaw: 30.702042046643918,
    BlastRadiusNorm: 100,
    MaxBlastRadiusRaw: 30.702042046643918,
    ReviewPriorityRaw: 69.7,
    ReviewPriorityNorm: 100,
    MaxReviewPriorityRaw: 69.7,
    Combined: 100,
    Signals: [{ Name: 'File co-change coupling', Points: 0.25980762113533157, Category: 'graph' }],
    Symbols: [
      symbol('SlopeEncodingParts', 1, 3),
      symbol('SlopeStats', 1.7320508075688772, 3),
      symbol('computeHeatmapStats', 6.8728982993122445, 16.3),
      symbol('computeSlopeStats', 6.8728982993122445, 29.8),
      symbol('getSlopeEncoding', 7.091488720002977, 10.8),
      symbol('isSlopeGraph', 6.8728982993122445, 6.8),
    ],
  });

  it('sums every subtotal in one step and never adds a sqrt step', () => {
    const mm = buildMathMode(detail);
    expect(mm.blast.mode).toBe('linear');
    expect(mm.blast.perSymbol.map((l) => l.name)).toEqual([
      'SlopeEncodingParts', 'SlopeStats', 'computeHeatmapStats',
      'computeSlopeStats', 'getSlopeEncoding', 'isSlopeGraph',
    ]);
    near(mm.blast.symbolSubtotal, 30.442234425508587);
    near(mm.blast.hunkSubtotal, 0.25980762113533157);
    near(mm.blast.total, 30.702042046643918);
    near(mm.blast.artifactDelta, 0);
    expect([mm.blast.stepAdd, mm.blast.stepSum, mm.blast.stepCombine, mm.blast.stepHunk, mm.blast.stepScale])
      .toEqual([1, 2, 0, 0, 3]);
    expect([mm.priority.stepAdd, mm.priority.stepSum, mm.priority.stepScale]).toEqual([4, 5, 6]);
    expect(mm.stepBlend).toBe(7);
    near(mm.structural, 100);
  });

  it('skips the sum step for a single-symbol hunk', () => {
    const single = makeDetail({
      BlastRadiusRaw: 6.980266605821071,
      BlastRadiusNorm: 22.735512495280727,
      MaxBlastRadiusRaw: 30.702042046643918,
      ReviewPriorityRaw: 38.1,
      ReviewPriorityNorm: 54.66284074605452,
      MaxReviewPriorityRaw: 69.7,
      Combined: 35.50644379559024,
      Symbols: [symbol('ChatConversation', 6.980266605821071, 38.1)],
    });
    const mm = buildMathMode(single);
    expect([mm.blast.stepAdd, mm.blast.stepSum, mm.blast.stepScale]).toEqual([1, 0, 2]);
    expect(mm.stepBlend).toBe(5);
    near(mm.structural, 35.50644379559024);
  });
});

describe('blastMath current (sqrt) artifacts', () => {
  const detail = makeDetail({
    SymbolCombine: 'sqrt',
    BlastRadiusRaw: 5.763008763747323,
    BlastRadiusNorm: 100,
    MaxBlastRadiusRaw: 5.763008763747323,
    ReviewPriorityRaw: 8.34865258589672,
    ReviewPriorityNorm: 100,
    MaxReviewPriorityRaw: 8.34865258589672,
    Combined: 100,
    Signals: [{ Name: 'File co-change coupling', Points: 0.24556058315617352, Category: 'graph' }],
    Symbols: [
      symbol('SlopeEncodingParts', 1, 3),
      symbol('SlopeStats', 1.7320508075688772, 3),
      symbol('computeHeatmapStats', 6.8728982993122445, 16.3),
      symbol('computeSlopeStats', 6.8728982993122445, 29.8),
      symbol('getSlopeEncoding', 7.091488720002977, 10.8),
      symbol('isSlopeGraph', 6.8728982993122445, 6.8),
    ],
  });

  it('renders sum -> sqrt -> add file-level signals -> scale', () => {
    const mm = buildMathMode(detail);
    expect(mm.blast.mode).toBe('sqrt');
    expect(mm.blast.declaredCombine).toBe('sqrt');
    near(mm.blast.symbolSubtotal, 30.442234425508587);
    near(mm.blast.combinedSymbols, 5.51744818059115);
    near(mm.blast.hunkSubtotal, 0.24556058315617352);
    near(mm.blast.total, 5.763008763747323);
    near(mm.blast.artifactDelta, 0);
    expect([mm.blast.stepAdd, mm.blast.stepSum, mm.blast.stepCombine, mm.blast.stepHunk, mm.blast.stepAdjust, mm.blast.stepScale])
      .toEqual([1, 2, 3, 4, 0, 5]);
    expect([mm.priority.stepAdd, mm.priority.stepSum, mm.priority.stepCombine, mm.priority.stepHunk, mm.priority.stepScale])
      .toEqual([6, 7, 8, 0, 9]);
    expect(mm.stepBlend).toBe(10);
    near(mm.structural, 100);
  });

  it('applies the sqrt step even to a single symbol', () => {
    const single = makeDetail({
      SymbolCombine: 'sqrt',
      BlastRadiusRaw: 2.6420194181385326,
      BlastRadiusNorm: 57.08039644945656,
      MaxBlastRadiusRaw: 5.763008763747323,
      ReviewPriorityRaw: 6.172519744804386,
      ReviewPriorityNorm: 86.17251974480439,
      MaxReviewPriorityRaw: 8.34865258589672,
      Combined: 57.08039644945656,
      Symbols: [symbol('ChatConversation', 6.980266605821071, 38.1)],
    });
    const mm = buildMathMode(single);
    expect([mm.blast.stepAdd, mm.blast.stepSum, mm.blast.stepCombine, mm.blast.stepScale]).toEqual([1, 0, 2, 3]);
    near(mm.blast.total, 2.6420194181385326);
  });
});

describe('blastMath post-scoring adjustments', () => {
  it('shows a cross-repo bonus as its own step', () => {
    const raw = Math.sqrt(12) + 8;
    const detail = makeDetail({
      SymbolCombine: 'sqrt',
      BlastRadiusRaw: raw,
      BlastRadiusNorm: 100,
      MaxBlastRadiusRaw: raw,
      Combined: 60,
      Symbols: [{
        ...symbol('readAadhaar', 12, 0),
        Signals: [
          { Name: 'Caller reach', Points: 12, Category: 'graph' },
          { Name: 'Cross-repo impact', Points: 8, Category: 'graph' },
        ],
      }],
    });
    const mm = buildMathMode(detail);
    near(mm.blast.symbolSubtotal, 20);
    near(mm.blast.combinedSymbols, Math.sqrt(20));
    expect(mm.blast.stepAdjust).toBeGreaterThan(0);
    expect(mm.blast.stepScale).toBe(mm.blast.stepAdjust + 1);
    near(mm.blast.artifactDelta, raw - Math.sqrt(20));
    near(mm.blast.raw, raw);
  });
});

describe('blastMath inference and helpers', () => {
  it('infers the formula from the recorded score when unmarked', () => {
    expect(detectCombineMode(undefined, 30.442, 0.26, 30.702).mode).toBe('linear');
    expect(detectCombineMode(undefined, 30.442, 0.26, 5.517 + 0.26).mode).toBe('sqrt');
    expect(detectCombineMode('sqrt', 30.442, 0.26, 30.702).mode).toBe('sqrt');
    expect(detectCombineMode('linear', 30.442, 0.26, 5.777).mode).toBe('linear');
    expect(detectCombineMode(undefined, 0, 0, 0).mode).toBe('linear');
  });

  it('discounts a post-scoring cross-repo bonus when inferring the formula', () => {
    // review 14285: symbol sum 14.21 includes an 8-point bonus; raw is
    // sqrt(14.21 - 8) + 8 = 10.492, closer to linear unless discounted.
    expect(detectCombineMode(undefined, 14.21, 0, 10.492, 8).mode).toBe('sqrt');
    expect(detectCombineMode(undefined, 14.21, 0, 14.21, 8).mode).toBe('linear');
  });

  it('recovers a missing max from the recorded norm', () => {
    const detail = makeDetail({
      BlastRadiusRaw: 183.75178077156346,
      BlastRadiusNorm: 100,
      MaxBlastRadiusRaw: 0,
      Symbols: [symbol('f', 183.75178077156346, 0)],
    });
    const mm = buildMathMode(detail);
    near(mm.blast.max, 183.75178077156346);
    expect(mm.blast.maxInferred).toBe(true);
  });

  it('formats sums literally', () => {
    const expr = sumExpression([1.8, 1.4, -0.9]);
    expect(expr.text).toBe('1.8 + 1.4 - 0.9');
    near(expr.total, 2.3, 1e-12);
    expect(sumExpression([])).toEqual({ text: '0', total: 0 });
  });
});
