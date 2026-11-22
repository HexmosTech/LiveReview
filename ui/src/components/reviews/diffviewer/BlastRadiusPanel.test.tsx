import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
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

describe('BlastRadiusPanel cross-repo rendering', () => {
  const impact = {
    RelatedRepo: 'engine-project',
    RelatedRepoID: 2203,
    RelatedRepoName: 'org/integration',
    CallSite: '__route__POST__/v1/e-kyc/read-aadhaar',
    EdgeType: 'CROSS_HTTP_CALLS',
    MatchKind: 'exact',
  };

  const detail = makeDetail({
    ImpactedRelatedRepos: [impact],
    Symbols: [
      {
        QualifiedName: 'svc.readAadhaar',
        Name: 'readAadhaar',
        Label: 'Method',
        Method: 'calls',
        Signals: [
          { Name: 'Cross-repo impact', Detail: 'Called from org/integration', Points: 8, Category: 'graph' },
        ],
        BlastRadiusRaw: 10,
        ReviewPriorityRaw: 0,
        DirectCount: 0,
        TransitiveCount: 0,
        ImpactedRelatedRepos: [impact],
      },
    ],
  });

  it('links the repository named in the cross-repo signal sentence', () => {
    render(<MemoryRouter><BlastRadiusPanel detail={detail} /></MemoryRouter>);
    const links = screen.getAllByRole('link', { name: /org\/integration/ });
    expect(links.length).toBeGreaterThan(0);
    links.forEach((link) => {
      expect(link).toHaveAttribute('href', '/explore/repositories/2203/settings');
      expect(link.className).toContain('underline');
    });
    // The matched route is visible in the card, not only in a hover tooltip.
    expect(screen.getAllByText('/v1/e-kyc/read-aadhaar').length).toBeGreaterThan(0);
  });

  it('draws an impact chart from related repos even when the symbol has no callers', () => {
    const { container } = render(<MemoryRouter><BlastRadiusPanel detail={detail} /></MemoryRouter>);
    expect(screen.queryByText(/No callers in the dependency graph/)).not.toBeInTheDocument();
    const paths = container.querySelectorAll('svg path');
    expect(paths.length).toBeGreaterThan(0);
  });
});

describe('BlastRadiusPanel Math Mode formula rendering', () => {
  const signal = (category: string, points: number) => ({ Name: 'Signal', Points: points, Category: category });

  it('renders the sqrt combine and file-level-signal steps for current reports', () => {
    const detail = makeDetail({
      SymbolCombine: 'sqrt',
      BlastRadiusRaw: 5.763008763747323,
      BlastRadiusNorm: 100,
      MaxBlastRadiusRaw: 5.763008763747323,
      ReviewPriorityRaw: 8.34865258589672,
      ReviewPriorityNorm: 100,
      MaxReviewPriorityRaw: 8.34865258589672,
      Combined: 100,
      Signals: [signal('graph', 0.24556058315617352)],
      Symbols: [{
        QualifiedName: 'svc.f',
        Name: 'f',
        Label: 'Function',
        Method: 'calls',
        Signals: [signal('graph', 30.442234425508587), signal('code-metrics', 69.7)],
        BlastRadiusRaw: 30.442234425508587,
        ReviewPriorityRaw: 69.7,
        DirectCount: 0,
        TransitiveCount: 0,
      }],
    });
    render(<MemoryRouter><BlastRadiusPanel detail={detail} /></MemoryRouter>);
    fireEvent.click(screen.getByText('Math Mode'));
    expect(screen.getAllByText(/saturating combine/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/add this hunk's file-level signals/i)).toBeInTheDocument();
    expect(screen.getByText(/√\(30\.4\)/)).toBeInTheDocument();
  });

  it('keeps the legacy linear steps for unmarked artifacts', () => {
    const detail = makeDetail({
      BlastRadiusRaw: 30.702042046643918,
      BlastRadiusNorm: 100,
      MaxBlastRadiusRaw: 30.702042046643918,
      ReviewPriorityRaw: 69.7,
      ReviewPriorityNorm: 100,
      MaxReviewPriorityRaw: 69.7,
      Combined: 100,
      Signals: [signal('graph', 0.25980762113533157)],
      Symbols: [{
        QualifiedName: 'svc.f',
        Name: 'f',
        Label: 'Function',
        Method: 'calls',
        Signals: [signal('graph', 30.442234425508587), signal('code-metrics', 69.7)],
        BlastRadiusRaw: 30.442234425508587,
        ReviewPriorityRaw: 69.7,
        DirectCount: 0,
        TransitiveCount: 0,
      }],
    });
    render(<MemoryRouter><BlastRadiusPanel detail={detail} /></MemoryRouter>);
    fireEvent.click(screen.getByText('Math Mode'));
    expect(screen.getByText(/add every subtotal together/i)).toBeInTheDocument();
    expect(screen.queryByText(/saturating combine/i)).not.toBeInTheDocument();
  });

  it('renders the post-scoring adjustment step', () => {
    const raw = Math.sqrt(12) + 8;
    const detail = makeDetail({
      SymbolCombine: 'sqrt',
      BlastRadiusRaw: raw,
      BlastRadiusNorm: 100,
      MaxBlastRadiusRaw: raw,
      Combined: 60,
      Symbols: [{
        QualifiedName: 'svc.readAadhaar',
        Name: 'readAadhaar',
        Label: 'Method',
        Method: 'calls',
        Signals: [signal('graph', 12), signal('graph', 8)],
        BlastRadiusRaw: 20,
        ReviewPriorityRaw: 0,
        DirectCount: 0,
        TransitiveCount: 0,
      }],
    });
    render(<MemoryRouter><BlastRadiusPanel detail={detail} /></MemoryRouter>);
    fireEvent.click(screen.getByText('Math Mode'));
    expect(screen.getAllByText(/apply post-scoring adjustments/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/cross-repo bonuses and their roll-up effects/i).length).toBeGreaterThan(0);
  });
});
