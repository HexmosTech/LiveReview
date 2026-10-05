import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import BlastRadiusPanel from './BlastRadiusPanel';
import { BlastRadiusHunkReport } from '../../../types/reviews';

// Minimal structural report. Symbols is empty so the sunburst/flamegraph
// chart section (which needs d3/canvas) is not rendered.
const makeDetail = (overrides: Partial<BlastRadiusHunkReport> = {}): BlastRadiusHunkReport => ({
  FilePath: 'a.go',
  Header: '@@ -5,3 +5,3 @@',
  NewStart: 5,
  NewLines: 3,
  BlastRadiusRaw: 40,
  BlastRadiusNorm: 60,
  MaxBlastRadiusRaw: 100,
  ReviewPriorityRaw: 10,
  ReviewPriorityNorm: 20,
  MaxReviewPriorityRaw: 50,
  Combined: 24.57,
  HygieneMultiplier: 1.0,
  Weights: { BlastRadius: 0.6, ReviewPriority: 0.4 },
  Symbols: [],
  ...overrides,
});

const critical = {
  FindingSeverity: 100,
  FindingSeverityLabel: 'critical' as const,
  FindingSeverityCounts: { critical: 1, warning: 0, info: 0 },
};

describe('BlastRadiusPanel severity rendering', () => {
  it('renders the severity chip and Finding Severity card for a critical finding', () => {
    render(<BlastRadiusPanel detail={makeDetail(critical)} />);
    // Severity chip: severity 100 at 10% weight → +10.0
    expect(screen.getByText('Critical +10.0')).toBeInTheDocument();
    // Third dimension card
    expect(screen.getByText('Finding Severity')).toBeInTheDocument();
    // Blended headline: 0.9 * 24.57 + 0.1 * 100 = 32.113 → 32
    expect(screen.getByText(/Score 32/)).toBeInTheDocument();
  });

  it('shows the severity step and final blend in Math Mode', () => {
    render(<BlastRadiusPanel detail={makeDetail(critical)} />);
    fireEvent.click(screen.getByText('Math Mode'));
    expect(screen.getByText(/score the finding severity/i)).toBeInTheDocument();
    expect(screen.getByText(/blend in finding severity/i)).toBeInTheDocument();
  });

  it('renders a neutral chip and unblended score when there are no findings', () => {
    render(<BlastRadiusPanel detail={makeDetail()} />);
    expect(screen.getByText('Severity +0')).toBeInTheDocument();
    // 0.9 * 24.57 = 22.113 → 22
    expect(screen.getByText(/Score 22/)).toBeInTheDocument();
  });
});
