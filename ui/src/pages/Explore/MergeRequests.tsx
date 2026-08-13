import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { ColumnDef, useReactTable, getCoreRowModel, getFilteredRowModel, getSortedRowModel, getPaginationRowModel } from '@tanstack/react-table';
import { notify } from '../../utils/notify';
import { LuSearch } from 'react-icons/lu';
import { SiGitlab } from 'react-icons/si';
import { Button, Icons, Tooltip, Input, MultiSelectPanel } from '../../components/UIPrimitives';
import { ClientTable } from '../../components/DataTable/ClientTable';
import { SortIcon, SortableHeaderLabel, HeaderFilterPopover, multiSelectFilterFn, TruncatedWithTooltip } from '../../components/DataTable/HeaderControls';
import ExploreTabs from './ExploreTabs';
import { getPullRequests, getPRStateText, triggerReviewForPullRequest } from '../../api/pullRequests';
import { formatRelativeTime } from '../../api/reviews';
import { PullRequestWithRepo } from '../../types/explore';

const pageSizeOptions = [20, 50, 100];

// Same normalization as Explore > Repositories - real provider values are
// compound strings like "github-com"/"gitlab-self-hosted", not plain
// "github"/"gitlab", so filtering/sorting must key off the short prefix.
const normalizeProvider = (provider: string): string => {
  const normalized = provider.toLowerCase();
  if (normalized.startsWith('github')) return 'github';
  if (normalized.startsWith('gitlab')) return 'gitlab';
  if (normalized.startsWith('bitbucket')) return 'bitbucket';
  if (normalized.startsWith('gitea')) return 'gitea';
  if (normalized.startsWith('azuredevops')) return 'azuredevops';
  return normalized;
};

const providerLabel = (provider: string): string => {
  switch (normalizeProvider(provider)) {
    case 'github': return 'GitHub';
    case 'gitlab': return 'GitLab';
    case 'bitbucket': return 'Bitbucket';
    case 'gitea': return 'Gitea';
    case 'azuredevops': return 'Azure DevOps';
    default: return provider;
  }
};

const ProviderIcon: React.FC<{ provider: string }> = ({ provider }) => {
  const normalized = provider.toLowerCase();
  if (normalized.startsWith('github')) return <Icons.GitHub />;
  if (normalized.startsWith('gitlab')) return <SiGitlab className="w-5 h-5" style={{ color: '#FC6D26' }} />;
  if (normalized.startsWith('bitbucket')) return <Icons.Bitbucket />;
  if (normalized.startsWith('gitea')) return <Icons.Gitea />;
  if (normalized.startsWith('azuredevops')) return <Icons.AzureDevOps />;
  return null;
};

// Transparent-outline + colored text/icon, same visual language as the
// Sync Status badge on Explore > Repositories - GitHub's own PR state
// coloring (green while open, purple once merged, red once closed).
const prStateBadge = (state: string): { label: string; color: string; borderColor: string } => {
  switch (state) {
    case 'open':
      return { label: getPRStateText(state), color: '#4ade80', borderColor: 'rgba(34,197,94,0.35)' };
    case 'merged':
      return { label: getPRStateText(state), color: '#c084fc', borderColor: 'rgba(168,85,247,0.35)' };
    default:
      return { label: getPRStateText(state), color: '#f87171', borderColor: 'rgba(239,68,68,0.35)' };
  }
};

// Same shape as GitHub's octicon "git-pull-request" glyph, colored by PR
// state the way GitHub colors it.
const PR_STATE_ICON_COLOR: Record<string, string> = {
  open: 'text-green-500',
  merged: 'text-purple-500',
  closed: 'text-red-500',
};

const PullRequestStateIcon: React.FC<{ state: string; className?: string }> = ({ state, className = 'w-4 h-4' }) => (
  <svg
    className={`${className} ${PR_STATE_ICON_COLOR[state] || 'text-slate-400'}`}
    viewBox="0 0 16 16"
    fill="currentColor"
  >
    <path d="M7.177 3.073L9.573.677A.25.25 0 0110 .854v4.792a.25.25 0 01-.427.177L7.177 3.427a.25.25 0 010-.354zM3.75 2.5a.75.75 0 100 1.5.75.75 0 000-1.5zm-2.25.75a2.25 2.25 0 113 2.122v5.256a2.251 2.251 0 11-1.5 0V5.372A2.25 2.25 0 011.5 3.25zM11 2.5h-1V4h1a1 1 0 011 1v5.628a2.251 2.251 0 101.5 0V5A2.5 2.5 0 0011 2.5zm1 10.25a.75.75 0 111.5 0 .75.75 0 01-1.5 0zM3.75 12a.75.75 0 100 1.5.75.75 0 000-1.5z" />
  </svg>
);

const TITLE_MAX = 80;
const truncate = (text: string, max: number): string => (text.length > max ? `${text.slice(0, max)}…` : text);

// One page's worth of rows for the "fetch everything" loop below - large
// enough to keep the request count low for realistic org sizes while
// staying under the backend's per_page cap (200). Note: unlike repositories
// (bounded by repo count), an org's total PR history can be large - this
// fetches every state (open/closed/merged), not just the open ones the old
// server-filtered default used to load, so it's a real trade-off for very
// PR-heavy orgs in exchange for instant client-side sort/filter.
const FETCH_ALL_PAGE_SIZE = 200;

const MERGE_REQUESTS_COLUMN_WIDTHS = ['30%', '12%', '12%', '15%', '15%', '16%'];

const MergeRequests: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const repositoryId = searchParams.get('repository_id') || undefined;

  const [pullRequests, setPullRequests] = useState<PullRequestWithRepo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [triggeringId, setTriggeringId] = useState<number | null>(null);
  // In-memory only (not persisted) - reverts to "Trigger Review" on a page
  // refresh or re-visit, since we have no reliable way to know a triggered
  // review's live status from here.
  const [triggeredReviewIds, setTriggeredReviewIds] = useState<Record<number, string>>({});
  const [repositoryFilterName, setRepositoryFilterName] = useState<string | null>(null);

  // repository_id stays a server-side fetch scope (driven by the URL, e.g.
  // from a repo's "View PRs" button) rather than a client column filter -
  // it decides *which* PRs get loaded at all, same as Explore > Repositories
  // treats its own fetch scope. Search/state/provider/sort are all handled
  // client-side by TanStack once loaded, same pattern as that page.
  const fetchPullRequests = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const first = await getPullRequests({ repositoryId, page: 1, perPage: FETCH_ALL_PAGE_SIZE });
      let all = first.pull_requests || [];
      const totalPages = first.total_pages || 1;
      if (totalPages > 1) {
        const rest = await Promise.all(
          Array.from({ length: totalPages - 1 }, (_, i) => getPullRequests({ repositoryId, page: i + 2, perPage: FETCH_ALL_PAGE_SIZE }))
        );
        for (const page of rest) all = all.concat(page.pull_requests || []);
      }
      setPullRequests(all);
      setRepositoryFilterName(repositoryId && all.length > 0 ? all[0].repository_full_name : null);
    } catch (err) {
      console.error('Error fetching pull requests:', err);
      setError(err instanceof Error ? err.message : 'Failed to fetch pull/merge requests');
    } finally {
      setLoading(false);
    }
  }, [repositoryId]);

  useEffect(() => {
    fetchPullRequests();
  }, [fetchPullRequests]);

  const clearRepositoryFilter = () => {
    setSearchParams(new URLSearchParams());
  };

  const handleTriggerReview = async (pr: PullRequestWithRepo, e: React.MouseEvent) => {
    e.stopPropagation();
    setTriggeringId(pr.id);
    try {
      const response = await triggerReviewForPullRequest(pr.id);
      setTriggeredReviewIds((prev) => ({ ...prev, [pr.id]: response.reviewId }));
      notify.success(`Review triggered for #${pr.number}`);
    } catch (err) {
      console.error('Failed to trigger review:', err);
      notify.error(err instanceof Error ? err.message : 'Failed to trigger review');
    } finally {
      setTriggeringId(null);
    }
  };

  const columns = useMemo<ColumnDef<PullRequestWithRepo>[]>(() => [
    {
      id: 'title',
      // Searches title + author + repository together, same combined
      // free-text search the old top search bar offered.
      accessorFn: (pr) => `${pr.title} ${pr.author_name || pr.author_username || ''} ${pr.repository_full_name}`.toLowerCase(),
      filterFn: 'includesString',
      header: ({ column }) => (
        <div className="flex items-center justify-between gap-2">
          <SortableHeaderLabel label="Merge Request" onToggle={column.getToggleSortingHandler()} />
          <div className="flex items-center gap-2">
            <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
            <HeaderFilterPopover icon={LuSearch} label="Search merge requests">
              <Input
                placeholder="Search title, author, or repository..."
                value={(column.getFilterValue() as string) ?? ''}
                onChange={(e) => column.setFilterValue(e.target.value || undefined)}
                icon={<Icons.Search />}
                aria-label="Search merge requests"
                className="text-sm"
              />
            </HeaderFilterPopover>
          </div>
        </div>
      ),
      cell: ({ row }) => {
        const pr = row.original;
        const displayTitle = truncate(pr.title || `#${pr.number}`, TITLE_MAX);
        return (
          <div className="flex flex-col justify-center gap-1 min-w-0 min-h-[44px]">
            <div className="flex items-center gap-2 min-w-0">
              <Tooltip content={getPRStateText(pr.state)}>
                <span className="flex-shrink-0"><PullRequestStateIcon state={pr.state} /></span>
              </Tooltip>
              <span className="min-w-0 truncate text-white font-semibold">
                <TruncatedWithTooltip text={pr.title || `#${pr.number}`} max={TITLE_MAX}>
                  {displayTitle}
                </TruncatedWithTooltip>
              </span>
              <a
                href={pr.web_url}
                target="_blank"
                rel="noopener noreferrer"
                onClick={(e) => e.stopPropagation()}
                className="flex-shrink-0 text-blue-400 hover:text-blue-300 text-xs font-medium underline underline-offset-2"
              >
                #{pr.number}
              </a>
            </div>
            {pr.source_branch && pr.target_branch && (
              <div className="min-w-0 text-sm text-slate-400 truncate font-mono">
                {pr.source_branch} → {pr.target_branch}
              </div>
            )}
          </div>
        );
      },
    },
    {
      id: 'provider',
      accessorFn: (pr) => normalizeProvider(pr.provider),
      filterFn: multiSelectFilterFn,
      header: ({ column }) => (
        <div className="flex items-center justify-between gap-2">
          <SortableHeaderLabel label="Provider" onToggle={column.getToggleSortingHandler()} />
          <div className="flex items-center gap-2">
            <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
            <HeaderFilterPopover>
              <MultiSelectPanel
                label="Providers"
                value={(column.getFilterValue() as string) ?? ''}
                onChange={(v) => column.setFilterValue(v || undefined)}
                options={[
                  { value: 'github', label: 'GitHub' },
                  { value: 'gitlab', label: 'GitLab' },
                  { value: 'bitbucket', label: 'Bitbucket' },
                  { value: 'gitea', label: 'Gitea' },
                  { value: 'azuredevops', label: 'Azure DevOps' },
                ]}
              />
            </HeaderFilterPopover>
          </div>
        </div>
      ),
      cell: ({ row }) => (
        <div className="flex items-center gap-1.5 text-white">
          <ProviderIcon provider={row.original.provider} />
          {providerLabel(row.original.provider)}
        </div>
      ),
    },
    {
      id: 'state',
      accessorKey: 'state',
      filterFn: multiSelectFilterFn,
      header: ({ column }) => (
        <div className="flex items-center justify-between gap-2">
          <SortableHeaderLabel label="State" onToggle={column.getToggleSortingHandler()} />
          <div className="flex items-center gap-2">
            <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
            <HeaderFilterPopover>
              <MultiSelectPanel
                label="States"
                value={(column.getFilterValue() as string) ?? ''}
                onChange={(v) => column.setFilterValue(v || undefined)}
                options={[
                  { value: 'open', label: 'Open' },
                  { value: 'closed', label: 'Closed' },
                  { value: 'merged', label: 'Merged' },
                ]}
              />
            </HeaderFilterPopover>
          </div>
        </div>
      ),
      cell: ({ row }) => {
        const badge = prStateBadge(row.original.state);
        return (
          <div className="flex items-center justify-center">
            <span
              className="inline-flex items-center rounded-md px-2 py-0.5 text-sm font-medium w-fit"
              style={{ backgroundColor: 'transparent', color: badge.color, border: `1px solid ${badge.borderColor}` }}
            >
              {badge.label}
            </span>
          </div>
        );
      },
    },
    {
      id: 'author',
      accessorFn: (pr) => pr.author_name || pr.author_username || '',
      enableColumnFilter: false,
      header: ({ column }) => (
        <div className="flex items-center justify-between gap-2">
          <SortableHeaderLabel label="Author" onToggle={column.getToggleSortingHandler()} />
          <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
        </div>
      ),
      cell: ({ row }) => (
        <div className="text-white text-sm">{row.original.author_name || row.original.author_username || '—'}</div>
      ),
    },
    {
      id: 'updated',
      accessorFn: (pr) => pr.provider_updated_at || pr.last_synced_at || '',
      enableColumnFilter: false,
      header: ({ column }) => (
        <div className="flex items-center justify-between gap-2">
          <SortableHeaderLabel label="Updated" onToggle={column.getToggleSortingHandler()} />
          <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
        </div>
      ),
      cell: ({ row }) => (
        <div className="text-white text-sm">
          {formatRelativeTime(row.original.provider_updated_at || row.original.last_synced_at)}
        </div>
      ),
    },
    {
      id: 'actions',
      enableSorting: false,
      enableColumnFilter: false,
      header: () => <span className="font-semibold text-slate-300 uppercase tracking-wide text-xs">Actions</span>,
      cell: ({ row }) => {
        const pr = row.original;
        const triggeredReviewId = triggeredReviewIds[pr.id];
        if (triggeredReviewId) {
          return (
            <Button
              variant="outline"
              size="sm"
              onClick={(e) => {
                e.stopPropagation();
                navigate(`/reviews/${triggeredReviewId}`);
              }}
              className="border-slate-400 text-white hover:bg-white/10 hover:border-white text-sm cursor-pointer"
            >
              See Logs
            </Button>
          );
        }
        return (
          <Button
            variant="outline"
            size="sm"
            onClick={(e) => handleTriggerReview(pr, e)}
            disabled={triggeringId === pr.id}
            className="border-slate-400 text-white hover:bg-white/10 hover:border-white text-sm cursor-pointer"
          >
            {triggeringId === pr.id ? 'Triggering…' : 'Trigger Review'}
          </Button>
        );
      },
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [triggeringId, triggeredReviewIds, navigate]);

  const table = useReactTable({
    data: pullRequests,
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
      <div className="mb-2">
        <h1 className="text-3xl font-bold text-white mb-2">Explore</h1>
        <p className="text-slate-300">Browse merge/pull requests across every connected repository</p>
      </div>

      <ExploreTabs active="merge-requests" />

      {repositoryFilterName && (
        <div className="mt-4 flex items-center gap-2 text-sm text-slate-300">
          <span>Showing PRs for <span className="font-semibold text-white">{repositoryFilterName}</span></span>
          <button onClick={clearRepositoryFilter} className="text-blue-400 hover:text-blue-300 cursor-pointer">
            Show all repositories
          </button>
        </div>
      )}

      <div className="mt-6">
        <ClientTable
          table={table}
          columnWidths={MERGE_REQUESTS_COLUMN_WIDTHS}
          loading={loading}
          loadingLabel="Loading merge requests..."
          error={error}
          onRetry={() => fetchPullRequests()}
          isEmpty={pullRequests.length === 0}
          empty={{
            title: 'No merge requests found',
            description: 'Try a different filter, or sync a repository from the Repositories tab.',
          }}
          pageSizeOptions={pageSizeOptions}
        />
      </div>
    </div>
  );
};

export default MergeRequests;
