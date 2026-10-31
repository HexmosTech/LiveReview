// Tests for the severity-aware blending added to the blast-radius sort
// pipeline (mirrors git-lrc's blast_radius_sort_state.test.mjs cases).
import fc from 'fast-check';
import {
  attachBlastData,
  blendRiskScore,
  buildBlastLookup,
  flattenFilesByRisk,
  hunkSeverityInfo,
} from './blastRadius';
import { DiffReviewComment, BlastRadiusHunkReport, DiffReviewFile, DiffReviewHunk } from '../types/reviews';

function makeReportHunk(overrides: Partial<BlastRadiusHunkReport> = {}): BlastRadiusHunkReport {
  return {
    FilePath: 'a.go',
    Header: '@@',
    NewStart: 5,
    NewLines: 3,
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

const hunk: DiffReviewHunk = {
  old_start_line: 0,
  old_line_count: 0,
  new_start_line: 5,
  new_line_count: 3,
  content: '',
};

describe('hunkSeverityInfo', () => {
  it('maps severity levels, counts, and max wins', () => {
    const file: DiffReviewFile = {
      file_path: 'a.go',
      hunks: [hunk],
      comments: [
        { line: 5, content: 'x', severity: 'critical' },
        { line: 6, content: 'y', severity: 'warning' },
        { line: 7, content: 'z', severity: 'info' },
      ],
    };
    expect(hunkSeverityInfo(file, hunk)).toEqual({
      score: 100,
      label: 'critical',
      counts: { critical: 1, warning: 1, info: 1 },
    });
  });

  it('ignores comments outside the hunk range and returns zero when none match', () => {
    const file: DiffReviewFile = {
      file_path: 'a.go',
      hunks: [hunk],
      comments: [{ line: 100, content: 'x', severity: 'critical' }],
    };
    expect(hunkSeverityInfo(file, hunk)).toEqual({
      score: 0,
      label: null,
      counts: { critical: 0, warning: 0, info: 0 },
    });
  });
});

describe('blendRiskScore', () => {
  it('blends at the configured weight and treats missing combined as 0', () => {
    expect(blendRiskScore(100, 0)).toBeCloseTo(90);
    expect(blendRiskScore(0, 100)).toBeCloseTo(10);
    expect(blendRiskScore(80, 100)).toBeCloseTo(82);
    expect(blendRiskScore(null, 100)).toBeCloseTo(10);
  });
});

describe('attachBlastData', () => {
  it('blends finding severity into BlastRadius and carries FindingSeverity* on BlastDetail', () => {
    const files: DiffReviewFile[] = [
      {
        file_path: 'a.go',
        hunks: [hunk],
        comments: [{ line: 5, content: 'x', severity: 'critical' }],
      },
    ];
    const lookup = buildBlastLookup({
      Project: 'p',
      GeneratedAt: 'now',
      Files: [{ Path: 'a.go', Hunks: [makeReportHunk({ Combined: 24.57 })] }],
    });
    const joined = attachBlastData(files, lookup);
    const jh = joined[0].hunks[0];
    expect(jh.BlastRadius).toBeCloseTo(blendRiskScore(24.57, 100));
    expect(jh.BlastDetail?.FindingSeverity).toBe(100);
    expect(jh.BlastDetail?.FindingSeverityLabel).toBe('critical');
    expect(jh.BlastDetail?.FindingSeverityCounts).toEqual({ critical: 1, warning: 0, info: 0 });
    expect(jh.BlastDetail?.Combined).toBe(24.57);
  });
});

// Regression coverage for the "comment appears then disappears" bug once
// blast-radius data arrives mid-review and the view switches to the default
// risk-flat sort: flattenFilesByRisk must never drop a comment just because
// its line doesn't fall inside the exact hunk range it gets matched to.
describe('flattenFilesByRisk - comment conservation', () => {
  const hunkA: DiffReviewHunk = { old_start_line: 0, old_line_count: 0, new_start_line: 5, new_line_count: 3, content: '' };
  const hunkB: DiffReviewHunk = { old_start_line: 0, old_line_count: 0, new_start_line: 20, new_line_count: 3, content: '' };

  it('keeps a comment whose line falls inside a hunk range exactly', () => {
    const files: DiffReviewFile[] = [
      { file_path: 'a.go', hunks: [hunkA], comments: [{ line: 5, content: 'in range' }] },
    ];
    const flat = flattenFilesByRisk(files);
    expect(flat.flatMap((f) => f.comments)).toEqual([{ line: 5, content: 'in range' }]);
  });

  it('does not drop a comment sitting in the gap between two hunks in the same file', () => {
    const files: DiffReviewFile[] = [
      {
        file_path: 'a.go',
        hunks: [hunkA, hunkB],
        // line 12 is in the gap between hunkA's [5,8) and hunkB's [20,23) -
        // this is exactly the shape that silently vanished before the fix.
        comments: [{ line: 12, content: 'gap comment' }],
      },
    ];
    const flat = flattenFilesByRisk(files);
    expect(flat.flatMap((f) => f.comments)).toEqual([{ line: 12, content: 'gap comment' }]);
  });

  it('does not drop a comment whose line is before the first hunk or after the last', () => {
    const files: DiffReviewFile[] = [
      {
        file_path: 'a.go',
        hunks: [hunkA, hunkB],
        comments: [
          { line: 1, content: 'before first hunk' },
          { line: 99, content: 'after last hunk' },
        ],
      },
    ];
    const flat = flattenFilesByRisk(files);
    const contents = flat.flatMap((f) => f.comments).map((c) => c.content);
    expect(contents.sort()).toEqual(['after last hunk', 'before first hunk']);
  });

  it('keeps unclaimed comments attached to the correct file when multiple files are flattened together', () => {
    const files: DiffReviewFile[] = [
      { file_path: 'a.go', hunks: [hunkA], comments: [{ line: 999, content: 'orphan in a.go' }] },
      { file_path: 'b.go', hunks: [hunkB], comments: [{ line: 5, content: 'orphan in b.go' }] },
    ];
    const flat = flattenFilesByRisk(files);
    const byFile = new Map<string, string[]>();
    flat.forEach((f) => byFile.set(f.file_path, [...(byFile.get(f.file_path) || []), ...f.comments.map((c) => c.content)]));
    expect(byFile.get('a.go')).toEqual(['orphan in a.go']);
    expect(byFile.get('b.go')).toEqual(['orphan in b.go']);
  });

  it('property: total comment count is conserved across any hunk/comment-line combination', () => {
    const hunkArb = fc.record({
      old_start_line: fc.constant(0),
      old_line_count: fc.constant(0),
      new_start_line: fc.integer({ min: 1, max: 200 }),
      new_line_count: fc.integer({ min: 1, max: 20 }),
      content: fc.constant(''),
    });
    const fileArb = fc
      .record({
        file_path: fc.constantFrom('a.go', 'b.go'),
        hunks: fc.array(hunkArb, { minLength: 0, maxLength: 5 }),
        lines: fc.array(fc.integer({ min: 0, max: 220 }), { minLength: 0, maxLength: 10 }),
      })
      .map(({ file_path, hunks, lines }): DiffReviewFile => ({
        file_path,
        hunks,
        comments: lines.map((line, i): DiffReviewComment => ({ line, content: `c${i}` })),
      }));

    fc.assert(
      fc.property(fc.array(fileArb, { minLength: 1, maxLength: 3 }), (files) => {
        const totalBefore = files.reduce((sum, f) => sum + (f.comments || []).length, 0);
        const flat = flattenFilesByRisk(files);
        const totalAfter = flat.reduce((sum, f) => sum + (f.comments || []).length, 0);
        expect(totalAfter).toBe(totalBefore);
      }),
      { numRuns: 300 }
    );
  });
});
