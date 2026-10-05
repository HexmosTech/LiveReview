// Tests for the severity-aware blending added to the blast-radius sort
// pipeline (mirrors git-lrc's blast_radius_sort_state.test.mjs cases).
import {
  attachBlastData,
  blendRiskScore,
  buildBlastLookup,
  hunkSeverityInfo,
} from './blastRadius';
import { BlastRadiusHunkReport, DiffReviewFile, DiffReviewHunk } from '../types/reviews';

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
