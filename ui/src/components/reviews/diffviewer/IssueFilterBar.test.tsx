import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import IssueFilterBar from './IssueFilterBar';
import {
  buildFilterFacets,
  createDefaultIssueFilters,
  toggleConfidenceFilter,
  toggleSeverityFilter,
  toggleToolFilter,
} from './issueFilters';
import { DiffReviewFile } from '../../../types/reviews';

jest.mock('./VoteButtons', () => () => <div data-testid="vote-buttons" />);

describe('IssueFilterBar chip styling', () => {
  const mockFiles: DiffReviewFile[] = [
    {
      file_path: 'src/app.py',
      hunks: [],
      comments: [
        {
          line: 10,
          content: 'SQL injection risk',
          severity: 'critical',
          confidence: 'high',
          type: 'risk',
          category: 'Security',
          source: 'ai',
        },
        {
          line: 15,
          content: 'Missing error check',
          severity: 'warning',
          confidence: 'medium',
          type: 'bug',
          category: 'Reliability',
          source: 'ai',
        },
        {
          line: 20,
          content: 'Unused variable',
          severity: 'warning',
          confidence: 'high',
          type: 'code_smell',
          category: 'Code Quality',
          tool_name: 'ruff',
          source: 'tool',
        },
        {
          line: 30,
          content: 'Minor note',
          severity: 'info',
          confidence: 'high',
          type: 'best_practice',
          category: 'Secret Scanning',
          tool_name: 'gitleaks',
          source: 'tool',
        },
      ],
    },
  ];

  it('renders unselected filter chips as fully dull with focus:outline-none and active chips without bright glare', () => {
    let filters = createDefaultIssueFilters();
    filters = toggleSeverityFilter(filters, 'info');
    filters = toggleConfidenceFilter(filters, 'medium');
    filters = toggleToolFilter(filters, 'ruff');

    const facets = buildFilterFacets(mockFiles, filters);

    render(
      <IssueFilterBar
        reviewId={12557}
        filters={filters}
        facets={facets}
        onToggleSeverity={jest.fn()}
        onToggleConfidence={jest.fn()}
        onToggleType={jest.fn()}
        onToggleCategory={jest.fn()}
        onToggleSubcategory={jest.fn()}
        onToggleTool={jest.fn()}
        onReset={jest.fn()}
        onCopyVisibleIssues={jest.fn()}
        copyStatus="idle"
      />
    );

    // Unselected Info severity chip must be dull neutral slate (no sky highlight, no click ring)
    const infoButton = screen.getByRole('button', { name: /Info/i });
    expect(infoButton).toHaveAttribute('aria-pressed', 'false');
    expect(infoButton).toHaveClass('focus:outline-none', 'border-slate-700/70', 'bg-slate-900/50', 'text-slate-400');
    expect(infoButton.className).not.toContain('sky-');
    expect(infoButton.querySelector('span:last-child')).toHaveClass('bg-slate-800/80', 'text-slate-400');

    // Active Critical severity chip is clearly enabled without harsh glare
    const criticalButton = screen.getByRole('button', { name: /Critical/i });
    expect(criticalButton).toHaveAttribute('aria-pressed', 'true');
    expect(criticalButton).toHaveClass('focus:outline-none', 'border-red-400/50', 'bg-red-500/[0.15]', 'text-red-200');
    expect(criticalButton.querySelector('span:last-child')).toHaveClass('bg-white/10');

    // Unselected Medium confidence chip is dull neutral slate
    const mediumButton = screen.getByRole('button', { name: /^Medium/i });
    expect(mediumButton).toHaveAttribute('aria-pressed', 'false');
    expect(mediumButton).toHaveClass('focus:outline-none', 'bg-slate-900/50', 'text-slate-400');

    // Active High confidence chip uses clearly visible blue without #eef6ff glare
    const highButton = screen.getByRole('button', { name: /^High/i });
    expect(highButton).toHaveAttribute('aria-pressed', 'true');
    expect(highButton).toHaveClass('focus:outline-none', 'border-blue-400/50', 'bg-blue-500/[0.16]', 'text-blue-200');
    expect(highButton.className).not.toContain('#eef6ff');
    expect(highButton).not.toHaveClass('bg-blue-500/20');

    // Unselected static tool chip is dull
    const ruffButton = screen.getByRole('button', { name: /ruff/i });
    expect(ruffButton).toHaveAttribute('aria-pressed', 'false');
    expect(ruffButton).toHaveClass('focus:outline-none', 'bg-slate-900/50', 'text-slate-400');

    // Active AI Review chip uses clear enabled styling
    const aiButton = screen.getByRole('button', { name: /AI Review/i });
    expect(aiButton).toHaveAttribute('aria-pressed', 'true');
    expect(aiButton).toHaveClass('focus:outline-none', 'bg-blue-500/[0.16]', 'text-blue-200');
  });
});
