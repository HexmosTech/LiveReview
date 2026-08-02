import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ColumnDef, useReactTable, getCoreRowModel, getFilteredRowModel, getSortedRowModel, getPaginationRowModel } from '@tanstack/react-table';
import { LuSearch, LuTerminal } from 'react-icons/lu';
import { SiGitlab } from 'react-icons/si';
import { Button, Icons, Input, MultiSelectPanel } from '../../components/UIPrimitives';
import { ClientTable } from '../../components/DataTable/ClientTable';
import { SortIcon, SortableHeaderLabel, HeaderFilterPopover, multiSelectFilterFn, TruncatedWithTooltip } from '../../components/DataTable/HeaderControls';
import { getReviews, formatRelativeTime, getStatusText } from '../../api/reviews';
import { Review } from '../../types/reviews';

const pageSizeOptions = [20, 50, 100];

// One page's worth of rows for the "fetch everything" loop below. Reviews
// accumulate indefinitely (unlike repos/PRs, bounded by repo count), so this
// is a real trade-off: instant client-side sort/filter/search in exchange
// for loading the org's whole review history up front instead of one
// server-paginated page at a time. The backend allows per_page up to 1000
// (internal/api/server.go), used here to keep the request count low.
const FETCH_ALL_PAGE_SIZE = 500;

const TITLE_MAX = 80;
const truncate = (text: string, max: number): string => (text.length > max ? `${text.slice(0, max)}…` : text);

const toTitleCase = (value: string): string => value.replace(/\b\w/g, (char) => char.toUpperCase());

// review.provider is literally "cli" for CLI-triggered reviews
// (internal/jobqueue/review_worker.go), not a real git provider - treated
// as its own "source" alongside github/gitlab/etc.
const normalizeSource = (provider?: string): string => {
  const normalized = (provider || '').toLowerCase();
  if (normalized === 'cli') return 'cli';
  if (normalized.startsWith('github')) return 'github';
  if (normalized.startsWith('gitlab')) return 'gitlab';
  if (normalized.startsWith('bitbucket')) return 'bitbucket';
  if (normalized.startsWith('gitea')) return 'gitea';
  if (normalized.startsWith('azuredevops')) return 'azuredevops';
  return normalized;
};

const sourceLabel = (provider?: string): string => {
  switch (normalizeSource(provider)) {
    case 'cli': return 'CLI';
    case 'github': return 'GitHub';
    case 'gitlab': return 'GitLab';
    case 'bitbucket': return 'Bitbucket';
    case 'gitea': return 'Gitea';
    case 'azuredevops': return 'Azure DevOps';
    default: return provider ? toTitleCase(provider) : '—';
  }
};

const SourceIcon: React.FC<{ provider?: string }> = ({ provider }) => {
  switch (normalizeSource(provider)) {
    case 'cli': return <LuTerminal size={18} />;
    case 'github': return <Icons.GitHub />;
    case 'gitlab': return <SiGitlab className="w-5 h-5" style={{ color: '#FC6D26' }} />;
    case 'bitbucket': return <Icons.Bitbucket />;
    case 'gitea': return <Icons.Gitea />;
    case 'azuredevops': return <Icons.AzureDevOps />;
    default: return null;
  }
};

// GitHub: /owner/repo/pull/123 -> owner ; GitLab: /owner/repo/-/merge_requests/123 -> owner
// Bitbucket: /workspace/repo/pull-requests/123 -> workspace
const extractAuthorFromUrl = (url: string): string | null => {
  try {
    const pathParts = new URL(url).pathname.split('/').filter(Boolean);
    if (pathParts.length >= 2) return pathParts[0];
  } catch {
    // ignore malformed URLs
  }
  return null;
};

const extractMRInfo = (url: string): string => {
  try {
    const pathParts = new URL(url).pathname.split('/').filter(Boolean);
    if (pathParts.includes('pull') && pathParts.length >= 4) {
      return `PR #${pathParts[pathParts.indexOf('pull') + 1]}`;
    }
    if (pathParts.includes('merge_requests') && pathParts.length >= 4) {
      return `MR !${pathParts[pathParts.indexOf('merge_requests') + 1]}`;
    }
    if (pathParts.includes('pull-requests') && pathParts.length >= 4) {
      return `PR #${pathParts[pathParts.indexOf('pull-requests') + 1]}`;
    }
    return 'MR/PR';
  } catch {
    return 'MR/PR';
  }
};

const getExecutionBadge = (review: Review): { label: string; color: string; borderColor: string } | null => {
  const rawMode = typeof review.metadata?.ai_execution_mode === 'string' ? review.metadata.ai_execution_mode.toLowerCase() : '';
  const rawPlanCode = typeof review.metadata?.plan_code === 'string' ? review.metadata.plan_code.toLowerCase() : '';

  if (rawMode === 'hosted_auto' || rawPlanCode === 'team_32usd' || rawPlanCode === 'team') {
    return { label: 'Auto', color: '#6ee7b7', borderColor: 'rgba(16,185,129,0.35)' };
  }
  if (rawMode === 'byok_required' || rawMode === 'byok_override' || rawMode === 'byok_optional' || rawPlanCode === 'free_30k' || rawPlanCode === 'free') {
    return { label: 'BYOK', color: '#fcd34d', borderColor: 'rgba(245,158,11,0.35)' };
  }
  return null;
};

// Transparent-outline + colored text, same visual language as the Sync
// Status / PR State badges on the other Explore tables.
const reviewStatusBadge = (status: string): { color: string; borderColor: string } => {
  switch (status) {
    case 'completed':
      return { color: '#4ade80', borderColor: 'rgba(34,197,94,0.35)' };
    case 'failed':
      return { color: '#f87171', borderColor: 'rgba(239,68,68,0.35)' };
    case 'in_progress':
      return { color: '#60a5fa', borderColor: 'rgba(59,130,246,0.35)' };
    default:
      return { color: '#fcd34d', borderColor: 'rgba(245,158,11,0.35)' };
  }
};

const getCleanRepository = (review: Review): string =>
  review.repository ? review.repository.replace(/^https?:\/\//, '').replace(/\/$/, '') : '';

const getRepoShortName = (cleanedRepository: string): string => {
  const repoSegments = cleanedRepository.split('/').filter(Boolean);
  return repoSegments.length ? repoSegments[repoSegments.length - 1] : '';
};

/** Same "what should the primary title read as" logic for a review, used by
 * both the sort accessor and the cell renderer so they stay consistent. */
const getPrimaryTitle = (review: Review): string => {
  const rawMrDescriptor = review.prMrUrl ? extractMRInfo(review.prMrUrl) : '';
  const mrDescriptor = rawMrDescriptor && rawMrDescriptor !== 'MR/PR' ? rawMrDescriptor : '';
  const repoShort = getRepoShortName(getCleanRepository(review));

  if (review.triggerType === 'cli_diff') {
    return review.aiSummaryTitle?.trim() || review.friendlyName?.trim() || repoShort || 'CLI Review';
  }
  const mrTitleCandidate = review.mrTitle?.trim();
  return mrTitleCandidate && mrTitleCandidate.length > 0 ? mrTitleCandidate : (mrDescriptor || repoShort || 'Code Review');
};

const REVIEWS_COLUMN_WIDTHS = ['24%', '16%', '12%', '14%', '12%', '12%', '10%'];

const Reviews: React.FC = () => {
  const navigate = useNavigate();

  const [reviews, setReviews] = useState<Review[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchReviews = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const first = await getReviews({ page: 1, perPage: FETCH_ALL_PAGE_SIZE });
      let all = first.reviews || [];
      const totalPages = first.totalPages || 1;
      if (totalPages > 1) {
        const rest = await Promise.all(
          Array.from({ length: totalPages - 1 }, (_, i) => getReviews({ page: i + 2, perPage: FETCH_ALL_PAGE_SIZE }))
        );
        for (const page of rest) all = all.concat(page.reviews || []);
      }
      setReviews(all);
    } catch (err) {
      console.error('Error fetching reviews:', err);
      setError(err instanceof Error ? err.message : 'Failed to fetch reviews');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchReviews();
  }, [fetchReviews]);

  // Light auto-refresh while any review is non-terminal (created/in_progress).
  useEffect(() => {
    const hasActive = reviews.some((r) => r.status === 'created' || r.status === 'in_progress');
    if (!hasActive) return;
    const id = setInterval(() => fetchReviews(), 15000); // 15s cadence to avoid hammering
    return () => clearInterval(id);
  }, [reviews, fetchReviews]);

  const handleViewReview = useCallback((review: Review) => {
    navigate(`/reviews/${review.id}`);
  }, [navigate]);

  const columns = useMemo<ColumnDef<Review>[]>(() => [
    {
      id: 'review',
      accessorFn: (review) => getPrimaryTitle(review),
      // Searches title/repo/branch/URL/author together, same combined
      // free-text search the old top search bar offered (it searched
      // "repositories or URLs"; author is included too since it's visible
      // in this same column).
      filterFn: (row, _columnId, filterValue: string) => {
        const q = (filterValue || '').trim().toLowerCase();
        if (!q) return true;
        const r = row.original;
        return [r.repository, r.branch, r.prMrUrl, r.mrTitle, r.friendlyName, r.aiSummaryTitle, r.authorName, r.authorUsername]
          .filter((v): v is string => Boolean(v))
          .some((v) => v.toLowerCase().includes(q));
      },
      header: ({ column }) => (
        <div className="flex items-center justify-between gap-2">
          <SortableHeaderLabel label="Review" onToggle={column.getToggleSortingHandler()} />
          <div className="flex items-center gap-2">
            <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
            <HeaderFilterPopover icon={LuSearch} label="Search reviews">
              <Input
                placeholder="Search repositories, URLs, or author..."
                value={(column.getFilterValue() as string) ?? ''}
                onChange={(e) => column.setFilterValue(e.target.value || undefined)}
                icon={<Icons.Search />}
                aria-label="Search reviews"
                className="text-sm"
              />
            </HeaderFilterPopover>
          </div>
        </div>
      ),
      cell: ({ row }) => {
        const review = row.original;
        const rawMrDescriptor = review.prMrUrl ? extractMRInfo(review.prMrUrl) : '';
        const mrDescriptor = rawMrDescriptor && rawMrDescriptor !== 'MR/PR' ? rawMrDescriptor : '';
        const branchLabel = review.branch?.trim();
        const metaChips = [branchLabel ? `Branch: ${branchLabel}` : '', mrDescriptor]
          .filter((v): v is string => Boolean(v));
        const primaryTitle = getPrimaryTitle(review);
        const displayTitle = truncate(primaryTitle, TITLE_MAX);

        return (
          <div className="flex flex-col justify-center gap-1 min-w-0 min-h-[44px]">
            <div className="flex items-center gap-2 min-w-0">
              <span className="min-w-0 truncate text-white font-semibold">
                <TruncatedWithTooltip text={primaryTitle} max={TITLE_MAX}>
                  {displayTitle}
                </TruncatedWithTooltip>
              </span>
              {review.prMrUrl && (
                <a
                  href={review.prMrUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  onClick={(e) => e.stopPropagation()}
                  className="flex-shrink-0 text-blue-400 hover:text-blue-300 text-xs font-medium underline underline-offset-2"
                >
                  Open PR/MR
                </a>
              )}
            </div>
            {metaChips.length > 0 && (
              <div className="min-w-0 text-sm text-slate-400 truncate">{metaChips.join(' · ')}</div>
            )}
          </div>
        );
      },
    },
    {
      id: 'repository',
      accessorFn: (review) => getCleanRepository(review),
      filterFn: 'includesString',
      header: ({ column }) => (
        <div className="flex items-center justify-between gap-2">
          <SortableHeaderLabel label="Repository" onToggle={column.getToggleSortingHandler()} />
          <div className="flex items-center gap-2">
            <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
            <HeaderFilterPopover icon={LuSearch} label="Search repositories">
              <Input
                placeholder="Search repositories..."
                value={(column.getFilterValue() as string) ?? ''}
                onChange={(e) => column.setFilterValue(e.target.value || undefined)}
                icon={<Icons.Search />}
                aria-label="Search repositories"
                className="text-sm"
              />
            </HeaderFilterPopover>
          </div>
        </div>
      ),
      cell: ({ getValue }) => {
        const cleanedRepository = getValue() as string;
        return (
          <TruncatedWithTooltip text={cleanedRepository} max={TITLE_MAX}>
            <div className="min-w-0 truncate text-white text-sm">{truncate(cleanedRepository, TITLE_MAX) || '—'}</div>
          </TruncatedWithTooltip>
        );
      },
    },
    {
      id: 'source',
      accessorFn: (review) => normalizeSource(review.provider),
      filterFn: multiSelectFilterFn,
      header: ({ column }) => (
        <div className="flex items-center justify-between gap-2">
          <SortableHeaderLabel label="Source" onToggle={column.getToggleSortingHandler()} />
          <div className="flex items-center gap-2">
            <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
            <HeaderFilterPopover>
              <MultiSelectPanel
                label="Sources"
                value={(column.getFilterValue() as string) ?? ''}
                onChange={(v) => column.setFilterValue(v || undefined)}
                options={[
                  { value: 'github', label: 'GitHub' },
                  { value: 'gitlab', label: 'GitLab' },
                  { value: 'bitbucket', label: 'Bitbucket' },
                  { value: 'gitea', label: 'Gitea' },
                  { value: 'azuredevops', label: 'Azure DevOps' },
                  { value: 'cli', label: 'CLI' },
                ]}
              />
            </HeaderFilterPopover>
          </div>
        </div>
      ),
      cell: ({ row }) => (
        <div className="flex items-center gap-1.5 text-white">
          <SourceIcon provider={row.original.provider} />
          {sourceLabel(row.original.provider)}
        </div>
      ),
    },
    {
      id: 'status',
      accessorKey: 'status',
      filterFn: multiSelectFilterFn,
      header: ({ column }) => (
        <div className="flex items-center justify-between gap-2">
          <SortableHeaderLabel label="Status" onToggle={column.getToggleSortingHandler()} />
          <div className="flex items-center gap-2">
            <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
            <HeaderFilterPopover>
              <MultiSelectPanel
                label="Statuses"
                value={(column.getFilterValue() as string) ?? ''}
                onChange={(v) => column.setFilterValue(v || undefined)}
                options={[
                  { value: 'created', label: 'Created' },
                  { value: 'in_progress', label: 'In Progress' },
                  { value: 'completed', label: 'Completed' },
                  { value: 'failed', label: 'Failed' },
                ]}
              />
            </HeaderFilterPopover>
          </div>
        </div>
      ),
      cell: ({ row }) => {
        const review = row.original;
        const badge = reviewStatusBadge(review.status);
        const executionBadge = getExecutionBadge(review);
        return (
          <div className="flex flex-wrap items-center gap-1.5">
            <span
              className="inline-flex items-center rounded-md px-2 py-0.5 text-sm font-medium w-fit"
              style={{ backgroundColor: 'transparent', color: badge.color, border: `1px solid ${badge.borderColor}` }}
            >
              {getStatusText(review.status)}
            </span>
            {executionBadge && (
              <span
                className="inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium w-fit"
                style={{ backgroundColor: 'transparent', color: executionBadge.color, border: `1px solid ${executionBadge.borderColor}` }}
              >
                {executionBadge.label}
              </span>
            )}
          </div>
        );
      },
    },
    {
      id: 'author',
      accessorFn: (review) => {
        const fallbackAuthorFromUrl = review.prMrUrl ? extractAuthorFromUrl(review.prMrUrl) : null;
        return review.authorName?.trim() || review.authorUsername?.trim() || review.userEmail?.trim() || fallbackAuthorFromUrl?.trim() || 'System';
      },
      enableColumnFilter: false,
      header: ({ column }) => (
        <div className="flex items-center justify-between gap-2">
          <SortableHeaderLabel label="Author" onToggle={column.getToggleSortingHandler()} />
          <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
        </div>
      ),
      cell: ({ getValue }) => <div className="text-white text-sm">{getValue() as string}</div>,
    },
    {
      id: 'last_activity',
      accessorFn: (review) => review.completedAt || review.startedAt || review.createdAt,
      enableColumnFilter: false,
      header: ({ column }) => (
        <div className="flex items-center justify-between gap-2">
          <SortableHeaderLabel label="Last Activity" onToggle={column.getToggleSortingHandler()} />
          <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
        </div>
      ),
      cell: ({ getValue }) => <div className="text-white text-sm">{formatRelativeTime(getValue() as string)}</div>,
    },
    {
      id: 'details',
      enableSorting: false,
      enableColumnFilter: false,
      header: () => <span className="font-semibold text-slate-300 uppercase tracking-wide text-xs">Details</span>,
      cell: ({ row }) => (
        <Button
          variant="outline"
          size="sm"
          onClick={(e) => {
            e.stopPropagation();
            handleViewReview(row.original);
          }}
          className="border-slate-400 text-white hover:bg-white/10 hover:border-white text-sm cursor-pointer"
        >
          View Details
        </Button>
      ),
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [handleViewReview]);

  const table = useReactTable({
    data: reviews,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    initialState: {
      pagination: { pageSize: pageSizeOptions[0] },
    },
  });

  return (
    <div className="container mx-auto px-4 py-8">
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-3xl font-bold text-white mb-2">Code Review Previews</h1>
          <p className="text-slate-300">Manage and monitor your AI-powered code review sessions</p>
        </div>
        <Button
          as={Link}
          to="/reviews/new"
          variant="primary"
          icon={<Icons.Add />}
          title="Safe preview - no comments posted"
        >
          New Review
        </Button>
      </div>

      <ClientTable
        table={table}
        columnWidths={REVIEWS_COLUMN_WIDTHS}
        loading={loading}
        loadingLabel="Loading reviews..."
        error={error}
        onRetry={() => fetchReviews()}
        isEmpty={reviews.length === 0}
        empty={{
          title: 'No review previews found',
          description: 'Get started by creating your first preview session (safe - no comments posted)',
          action: (
            <Button as={Link} to="/reviews/new" variant="primary" icon={<Icons.Add />}>
              New Review
            </Button>
          ),
        }}
        pageSizeOptions={pageSizeOptions}
        onRowClick={handleViewReview}
      />
    </div>
  );
};

export default Reviews;
