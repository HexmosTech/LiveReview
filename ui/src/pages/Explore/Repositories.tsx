import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ColumnDef,
  SortingState,
  ColumnFiltersState,
  PaginationState,
  OnChangeFn,
  useReactTable,
  getCoreRowModel,
} from '@tanstack/react-table';
import toast from 'react-hot-toast';
import { LuSearch } from 'react-icons/lu';
import { SiGitlab } from 'react-icons/si';
import { Button, Icons, Tooltip, Input, MultiSelectPanel } from '../../components/UIPrimitives';
import { ClientTable } from '../../components/DataTable/ClientTable';
import { SortIcon, SortableHeaderLabel, HeaderFilterPopover, multiSelectFilterFn, TruncatedWithTooltip } from '../../components/DataTable/HeaderControls';
import ExploreTabs from './ExploreTabs';
import { getRepositories, syncRepositoryPullRequests, syncConnectorRepositories } from '../../api/repositories';
import { getConnectors, ConnectorResponse } from '../../api/connectors';
import { formatRelativeTime } from '../../api/reviews';
import { Repository, RepositoriesFilters, RepositoriesSort } from '../../types/explore';

const pageSizeOptions = [20, 50, 100];

// Real provider values coming back from the API are compound strings like
// "github-com", "github-enterprise", "gitlab-com", "gitlab-self-hosted"
// (see internal/jobqueue/repo_sync_worker.go), not plain "github"/"gitlab".
// The Provider column's checkbox filter only offers the short keys, so
// filtering/sorting must normalize to this same short key first - filtering
// on the raw value directly meant an exact-match check against 'github'
// never matched "github-com", so unchecking any box hid rows it shouldn't
// have (and left ones it should have hidden).
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
  // GitLab's own brand icon (Icons.GitLab) renders monochrome, inheriting
  // whatever text color its container uses - rendered directly here with
  // GitLab's actual brand orange instead, scoped to this page rather than
  // recoloring the shared Icons.GitLab (used monochrome elsewhere in the app).
  if (normalized.startsWith('gitlab')) return <SiGitlab className="w-5 h-5" style={{ color: '#FC6D26' }} />;
  if (normalized.startsWith('bitbucket')) return <Icons.Bitbucket />;
  if (normalized.startsWith('gitea')) return <Icons.Gitea />;
  if (normalized.startsWith('azuredevops')) return <Icons.AzureDevOps />;
  return null;
};

// Plain hex/rgba values, applied via inline style rather than Tailwind
// classes - the shared Badge component's own default variant class
// (bg-gray-100 text-gray-800) kept winning over className overrides
// regardless of Tailwind override tricks, so color here is set directly and
// unconditionally instead of fighting that cascade.
const syncStatusBadge = (status: string): { label: string; color: string; borderColor: string; icon: React.FC } => {
  switch (status) {
    case 'ok':
      return { label: 'Synced', color: '#4ade80', borderColor: 'rgba(34,197,94,0.35)', icon: Icons.Ready };
    case 'error':
      return { label: 'Sync error', color: '#f87171', borderColor: 'rgba(239,68,68,0.35)', icon: Icons.NotReady };
    default:
      return { label: 'Pending', color: '#94a3b8', borderColor: 'rgba(100,116,139,0.35)', icon: Icons.Clock };
  }
};

// Matches the "Open in new tab" icon already used in RecentActivity.tsx.
const ExternalLinkIcon: React.FC<{ className?: string }> = ({ className = 'w-4 h-4' }) => (
  <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
  </svg>
);

const LockIcon: React.FC<{ className?: string }> = ({ className = 'w-3.5 h-3.5' }) => (
  <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-12V7a4 4 0 10-8 0v2h8z" />
  </svg>
);

const REPO_NAME_MAX = 60;
const DESCRIPTION_MAX = 90;

const truncate = (text: string, max: number): string => (text.length > max ? `${text.slice(0, max)}…` : text);

const REPOSITORIES_COLUMN_WIDTHS = ['32%', '17%', '17%', '17%', '17%'];

// How long to wait after the last keystroke in the Repository search box
// before actually firing the (now server-side) request - typing shouldn't
// send one request per character.
const SEARCH_DEBOUNCE_MS = 300;

const Repositories: React.FC = () => {
  const navigate = useNavigate();

  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(1);
  const [connectors, setConnectors] = useState<ConnectorResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [syncingRepoId, setSyncingRepoId] = useState<number | null>(null);
  const [syncingAll, setSyncingAll] = useState(false);

  // Sorting/filtering/pagination are now server-driven - these three are the
  // single source of truth TanStack's column header controls (sort icons,
  // search input, checkbox filters) read from and write to, and every change
  // to any of them re-fetches the current page from the API instead of
  // TanStack recomputing rows from an already-loaded client-side array.
  const [sorting, setSorting] = useState<SortingState>([]);
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const [pagination, setPagination] = useState<PaginationState>({ pageIndex: 0, pageSize: pageSizeOptions[0] });
  const searchDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    getConnectors()
      .then(setConnectors)
      .catch((err) => console.error('Failed to load connectors:', err));
  }, []);

  const buildFetchParams = (
    sortingState: SortingState,
    filtersState: ColumnFiltersState,
    paginationState: PaginationState
  ): RepositoriesFilters => {
    const sortEntry = sortingState[0];
    const getFilter = (id: string) => filtersState.find((f) => f.id === id)?.value as string | undefined;
    return {
      page: paginationState.pageIndex + 1,
      perPage: paginationState.pageSize,
      search: getFilter('repository') || undefined,
      provider: getFilter('provider') || undefined,
      syncStatus: getFilter('sync_status') || undefined,
      sort: sortEntry ? (sortEntry.id as RepositoriesSort) : undefined,
      order: sortEntry ? (sortEntry.desc ? 'desc' : 'asc') : undefined,
    };
  };

  const fetchRepositories = useCallback(async (params: RepositoriesFilters) => {
    try {
      setLoading(true);
      setError(null);
      const response = await getRepositories(params);
      setRepositories(response.repositories || []);
      setTotal(response.total || 0);
      setTotalPages(response.total_pages || 1);
    } catch (err) {
      console.error('Error fetching repositories:', err);
      setError(err instanceof Error ? err.message : 'Failed to fetch repositories');
    } finally {
      setLoading(false);
    }
  }, []);

  // Initial load.
  useEffect(() => {
    fetchRepositories(buildFetchParams(sorting, columnFilters, pagination));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const refetchCurrent = useCallback(() => {
    fetchRepositories(buildFetchParams(sorting, columnFilters, pagination));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fetchRepositories, sorting, columnFilters, pagination]);

  const handleSortingChange: OnChangeFn<SortingState> = (updater) => {
    const next = typeof updater === 'function' ? updater(sorting) : updater;
    const nextPagination = { ...pagination, pageIndex: 0 };
    setSorting(next);
    setPagination(nextPagination);
    fetchRepositories(buildFetchParams(next, columnFilters, nextPagination));
  };

  const handleColumnFiltersChange: OnChangeFn<ColumnFiltersState> = (updater) => {
    const next = typeof updater === 'function' ? updater(columnFilters) : updater;
    setColumnFilters(next);
    const nextPagination = { ...pagination, pageIndex: 0 };

    const prevSearch = columnFilters.find((f) => f.id === 'repository')?.value;
    const nextSearch = next.find((f) => f.id === 'repository')?.value;
    if (prevSearch !== nextSearch) {
      // Free-text search: debounce the actual request, but let the input's
      // own value (bound to column.getFilterValue()) update immediately via
      // the setColumnFilters call above, so typing itself stays responsive.
      if (searchDebounceRef.current) clearTimeout(searchDebounceRef.current);
      searchDebounceRef.current = setTimeout(() => {
        setPagination(nextPagination);
        fetchRepositories(buildFetchParams(sorting, next, nextPagination));
      }, SEARCH_DEBOUNCE_MS);
      return;
    }

    // Checkbox filters (Provider/Sync Status): discrete clicks, fetch right away.
    setPagination(nextPagination);
    fetchRepositories(buildFetchParams(sorting, next, nextPagination));
  };

  const handlePaginationChange: OnChangeFn<PaginationState> = (updater) => {
    const next = typeof updater === 'function' ? updater(pagination) : updater;
    setPagination(next);
    fetchRepositories(buildFetchParams(sorting, columnFilters, next));
  };

  const handleSync = async (repo: Repository, e: React.MouseEvent) => {
    e.stopPropagation();
    setSyncingRepoId(repo.id);
    try {
      await syncRepositoryPullRequests(repo.id);
      // Optimistically flip to "pending" so the user sees the click registered;
      // the next fetch (or a manual refresh) will reflect the real status once
      // the background sync job completes.
      setRepositories((prev) => prev.map((r) => (r.id === repo.id ? { ...r, last_sync_status: 'pending' } : r)));
    } catch (err) {
      console.error('Failed to trigger repository sync:', err);
    } finally {
      setSyncingRepoId(null);
    }
  };

  const handleSyncAllConnectors = async () => {
    if (connectors.length === 0) {
      toast.error('No connectors found. Connect a Git provider first.');
      return;
    }
    setSyncingAll(true);
    try {
      const results = await Promise.allSettled(
        connectors.map((c) => syncConnectorRepositories(c.id.toString()))
      );
      const failed = results.filter((r) => r.status === 'rejected').length;
      if (failed === 0) {
        toast.success(`Repository sync started for ${connectors.length} connector(s). New repositories and PRs will appear shortly.`);
      } else {
        toast.error(`Sync started, but failed for ${failed} of ${connectors.length} connector(s).`);
      }
      // Give the backfill a moment to at least upsert repositories (PR data
      // arrives asynchronously via the job queue) before refreshing the list.
      setTimeout(refetchCurrent, 2000);
    } finally {
      setSyncingAll(false);
    }
  };

  const columns = useMemo<ColumnDef<Repository>[]>(() => [
    {
      id: 'repository',
      accessorKey: 'full_name',
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
      cell: ({ row }) => {
        const repo = row.original;
        const displayName = truncate(repo.full_name, REPO_NAME_MAX);
        const segments = displayName.split('/');
        const leaf = segments.pop() as string;
        const pathPrefix = segments.join('/');
        const displayDescription = repo.description ? truncate(repo.description, DESCRIPTION_MAX) : '';
        return (
          <div className="flex flex-col justify-center gap-1 min-w-0 min-h-[44px]">
            <div className="flex items-center gap-2 min-w-0">
              <span className="min-w-0 truncate text-white">
                <TruncatedWithTooltip text={repo.full_name} max={REPO_NAME_MAX}>
                  {pathPrefix && <span>{pathPrefix}/</span>}
                  <span className="font-semibold">{leaf}</span>
                </TruncatedWithTooltip>
              </span>
              {repo.is_private && (
                <Tooltip content="Private">
                  <span className="flex-shrink-0 text-slate-400" aria-label="Private repository">
                    <LockIcon />
                  </span>
                </Tooltip>
              )}
              <Tooltip content="Open in provider">
                <a
                  href={repo.web_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  onClick={(e) => e.stopPropagation()}
                  className="flex-shrink-0 text-slate-400 hover:text-blue-300"
                  aria-label="Open in provider"
                >
                  <ExternalLinkIcon />
                </a>
              </Tooltip>
            </div>
            {displayDescription && (
              <div className="min-w-0 text-sm text-slate-400 truncate">
                <TruncatedWithTooltip text={repo.description as string} max={DESCRIPTION_MAX}>
                  {displayDescription}
                </TruncatedWithTooltip>
              </div>
            )}
          </div>
        );
      },
    },
    {
      id: 'provider',
      accessorFn: (repo) => normalizeProvider(repo.provider),
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
      id: 'default_branch',
      accessorKey: 'default_branch',
      enableColumnFilter: false,
      header: ({ column }) => (
        <div className="flex items-center justify-between gap-2">
          <SortableHeaderLabel label="Default Branch" onToggle={column.getToggleSortingHandler()} />
          <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
        </div>
      ),
      cell: ({ row }) => (
        <div className="text-center text-white text-sm">{row.original.default_branch || '—'}</div>
      ),
    },
    {
      id: 'sync_status',
      accessorKey: 'last_sync_status',
      filterFn: multiSelectFilterFn,
      header: ({ column }) => (
        <div className="flex items-center justify-between gap-2">
          <SortableHeaderLabel label="Sync Status" onToggle={column.getToggleSortingHandler()} />
          <div className="flex items-center gap-2">
            <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
            <HeaderFilterPopover>
              <MultiSelectPanel
                label="Sync Statuses"
                value={(column.getFilterValue() as string) ?? ''}
                onChange={(v) => column.setFilterValue(v || undefined)}
                options={[
                  { value: 'ok', label: 'Synced' },
                  { value: 'pending', label: 'Pending' },
                  { value: 'error', label: 'Sync error' },
                ]}
              />
            </HeaderFilterPopover>
          </div>
        </div>
      ),
      cell: ({ row }) => {
        const repo = row.original;
        const badge = syncStatusBadge(repo.last_sync_status);
        const BadgeIcon = badge.icon;
        return (
          <div className="flex items-center justify-center">
            <Tooltip content={repo.last_synced_at ? formatRelativeTime(repo.last_synced_at) : 'Never synced'}>
              <span
                className="inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-sm font-medium w-fit"
                style={{ backgroundColor: 'transparent', color: badge.color, border: `1px solid ${badge.borderColor}` }}
              >
                <BadgeIcon />
                {badge.label}
              </span>
            </Tooltip>
          </div>
        );
      },
    },
    {
      id: 'actions',
      enableSorting: false,
      enableColumnFilter: false,
      header: () => <span className="font-semibold text-slate-300 uppercase tracking-wide text-xs">Actions</span>,
      cell: ({ row }) => {
        const repo = row.original;
        const isSyncing = syncingRepoId === repo.id;
        return (
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={(e) => {
                e.stopPropagation();
                navigate(`/explore/merge-requests?repository_id=${repo.id}`);
              }}
              className="border-slate-400 text-white hover:bg-white/10 hover:border-white w-32 shrink-0 whitespace-nowrap text-sm"
            >
              View PRs{repo.open_pr_count > 0 ? ` (${repo.open_pr_count})` : ''}
            </Button>
            <Tooltip content={isSyncing ? 'Syncing…' : 'Sync PR/MR data for this repository'}>
              <Button
                variant="ghost"
                size="sm"
                onClick={(e) => handleSync(repo, e)}
                disabled={isSyncing}
                aria-label="Sync repository"
                className="text-slate-500 hover:text-slate-300 px-2 text-sm cursor-pointer"
              >
                <span className={isSyncing ? 'animate-spin' : ''}><Icons.Refresh /></span>
              </Button>
            </Tooltip>
            <Tooltip content="Repository settings (coming soon)">
              <Button
                variant="ghost"
                size="sm"
                disabled
                aria-label="Repository settings"
                className="text-slate-500 px-2 cursor-not-allowed text-sm"
              >
                <Icons.Settings />
              </Button>
            </Tooltip>
          </div>
        );
      },
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [syncingRepoId, navigate]);

  const table = useReactTable({
    data: repositories,
    columns,
    getCoreRowModel: getCoreRowModel(),
    // Server-side: TanStack just renders whatever page of already-
    // filtered/sorted rows the API returned, instead of computing
    // filtered/sorted/paginated row models itself from a client-side array.
    manualFiltering: true,
    manualSorting: true,
    manualPagination: true,
    pageCount: totalPages,
    state: { sorting, columnFilters, pagination },
    onSortingChange: handleSortingChange,
    onColumnFiltersChange: handleColumnFiltersChange,
    onPaginationChange: handlePaginationChange,
  });

  return (
    <div className="container mx-auto px-4 py-8">
      <div className="mb-2 flex items-start justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold text-white mb-2">Explore</h1>
          <p className="text-slate-300">Browse every repository accessible across your connected Git providers</p>
        </div>
        <Button
          variant="outline"
          onClick={handleSyncAllConnectors}
          disabled={syncingAll}
          className="border-slate-600 text-slate-300 hover:text-white hover:border-slate-500 whitespace-nowrap"
          icon={<Icons.Refresh />}
          title="Discover new repositories and sync PR/MR data across all connectors"
        >
          {syncingAll ? 'Syncing…' : 'Sync All Connectors'}
        </Button>
      </div>

      <ExploreTabs active="repositories" />

      <div className="mt-6">
        <ClientTable
          table={table}
          columnWidths={REPOSITORIES_COLUMN_WIDTHS}
          loading={loading}
          loadingLabel="Loading repositories..."
          error={error}
          onRetry={refetchCurrent}
          // Don't hide the filter controls when a filter causes zero results.
          isEmpty={total === 0 && columnFilters.length === 0}
          empty={{
            title: 'No repositories found',
            description: connectors.length > 0
              ? 'Your connector may not have synced yet. Try syncing.'
              : 'Connect a Git provider to see repositories here.',
            action: connectors.length > 0 ? (
              <Button
                variant="primary"
                onClick={handleSyncAllConnectors}
                disabled={syncingAll}
                icon={<Icons.Refresh />}
              >
                {syncingAll ? 'Syncing…' : 'Sync All Connectors'}
              </Button>
            ) : undefined,
          }}
          pageSizeOptions={pageSizeOptions}
          manualTotal={total}
        />
      </div>
    </div>
  );
};

export default Repositories;
