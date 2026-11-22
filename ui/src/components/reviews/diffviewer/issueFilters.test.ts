import {
  buildFilterFacets,
  createDefaultIssueFilters,
  toggleToolFilter,
} from './issueFilters';
import { DiffReviewFile } from '../../../types/reviews';

describe('issueFilters - static tools facets', () => {
  it('only shows tools with findings (> 0), omits 0-count tools, and sets all shown tools to active by default', () => {
    const mockFiles: DiffReviewFile[] = [
      {
        file_path: 'src/main.py',
        hunks: [],
        comments: [
          {
            line: 10,
            content: 'Unused import',
            severity: 'warning',
            confidence: 'high',
            category: 'Code Quality',
            tool_name: 'ruff',
            source: 'tool',
          },
          {
            line: 20,
            content: 'Hardcoded secret',
            severity: 'critical',
            confidence: 'high',
            category: 'Secret Scanning',
            tool_name: 'gitleaks',
            source: 'tool',
          },
        ],
      },
    ];

    const filters = createDefaultIssueFilters();
    const facets = buildFilterFacets(mockFiles, filters);

    // Only tools with findings are present
    const ruffOption = facets.tools.find((t) => t.value === 'ruff');
    expect(ruffOption).toBeDefined();
    expect(ruffOption?.count).toBe(1);
    expect(ruffOption?.active).toBe(true);

    const gitleaksOption = facets.tools.find((t) => t.value === 'gitleaks');
    expect(gitleaksOption).toBeDefined();
    expect(gitleaksOption?.count).toBe(1);
    expect(gitleaksOption?.active).toBe(true);

    // Tools with 0 findings must NOT be shown
    const openapiOption = facets.tools.find((t) => t.value === 'openapi');
    expect(openapiOption).toBeUndefined();

    const trufflehogOption = facets.tools.find((t) => t.value === 'trufflehog');
    expect(trufflehogOption).toBeUndefined();

    const golangciOption = facets.tools.find((t) => t.value === 'golangci-lint');
    expect(golangciOption).toBeUndefined();
  });

  it('correctly deactivates only toggled tools when toggleToolFilter is called', () => {
    const mockFiles: DiffReviewFile[] = [
      {
        file_path: 'src/main.py',
        hunks: [],
        comments: [
          {
            line: 10,
            content: 'Unused import',
            severity: 'warning',
            category: 'Code Quality',
            tool_name: 'ruff',
            source: 'tool',
          },
          {
            line: 20,
            content: 'Hardcoded secret',
            severity: 'critical',
            category: 'Secret Scanning',
            tool_name: 'gitleaks',
            source: 'tool',
          },
        ],
      },
    ];

    let filters = createDefaultIssueFilters();
    filters = toggleToolFilter(filters, 'ruff');

    const facets = buildFilterFacets(mockFiles, filters);
    const ruffOption = facets.tools.find((t) => t.value === 'ruff');
    expect(ruffOption?.active).toBe(false);

    // Other tools with findings remain active
    const gitleaksOption = facets.tools.find((t) => t.value === 'gitleaks');
    expect(gitleaksOption?.active).toBe(true);
  });
});
