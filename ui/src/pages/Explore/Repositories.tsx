import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import ProviderIcon from '../../components/ProviderIcon';
import { useNavigate, Link } from 'react-router-dom';
import { useAppDispatch } from '../../store/configureStore';
import { add as addNotification } from '../../store/Notifications/slice';
import {
  ColumnDef,
  SortingState,
  ColumnFiltersState,
  PaginationState,
  OnChangeFn,
  useReactTable,
  getCoreRowModel,
} from '@tanstack/react-table';
import { notify } from '../../utils/notify';
import { LuSearch } from 'react-icons/lu';
import { Alert, Button, Icons, Tooltip, Input, MultiSelectPanel } from '../../components/UIPrimitives';
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

// ─── Org-level connector error alert helpers ─────────────────────────────────

/** Error kinds we surface as connector-level alerts. */
type ConnectorErrorKind = 'auth' | 'mismatch';

interface ConnectorAlert {
  connector: import('../../api/connectors').ConnectorResponse;
  kind: ConnectorErrorKind;
  /** Number of repos in this connector with this error kind. */
  repoCount: number;
}

/** Return the LiveReview wiki URL for a given connector provider. */
function wikiUrlForProvider(provider: string): string {
  const norm = (provider || '').toLowerCase();
  if (norm.startsWith('gitlab-self') || norm === 'gitlab-self-hosted') {
    return 'https://github.com/HexmosTech/LiveReview/wiki/Self%E2%80%90Hosted-GitLab';
  }
  if (norm.startsWith('gitlab')) return 'https://github.com/HexmosTech/LiveReview/wiki/Gitlab';
  if (norm.startsWith('github')) return 'https://github.com/HexmosTech/LiveReview/wiki/Github';
  if (norm.startsWith('bitbucket')) return 'https://github.com/HexmosTech/LiveReview/wiki/BitBucket';
  return 'https://github.com/HexmosTech/LiveReview/wiki';
}

/** Scan the currently-loaded repository page and return one alert per
 *  affected connector. Multiple repos on the same connector collapse into
 *  a single alert so we never spam N banners for N repos. */
function computeConnectorAlerts(
  repos: import('../../types/explore').Repository[],
  connectors: import('../../api/connectors').ConnectorResponse[]
): ConnectorAlert[] {
  const connectorMap = new Map(connectors.map((c) => [c.id, c]));
  // connector_id → { auth count, mismatch count }
  const counts = new Map<number, { auth: number; mismatch: number }>();

  for (const repo of repos) {
    if (repo.last_sync_status !== 'error' || !repo.last_sync_error) continue;
    const err = repo.last_sync_error.toLowerCase();
    const isForbidden =
      err.includes('forbidden') ||
      err.includes('403') ||
      err.includes('access forbidden');
    const isAuth =
      !isForbidden &&
      (err.includes('auth') ||
        err.includes('token') ||
        err.includes('401') ||
        err.includes('unauthorized') ||
        err.includes('credential'));
    const isMismatch =
      err.includes('mismatch') ||
      err.includes('does not match') ||
      err.includes('configuration error') ||
      err.includes('incompatible');
    if (!isAuth && !isMismatch) continue;

    const existing = counts.get(repo.connector_id) ?? { auth: 0, mismatch: 0 };
    if (isAuth) existing.auth += 1;
    if (isMismatch) existing.mismatch += 1;
    counts.set(repo.connector_id, existing);
  }

  const alerts: ConnectorAlert[] = [];
  for (const [connectorId, { auth, mismatch }] of counts) {
    const connector = connectorMap.get(connectorId);
    if (!connector) continue;
    // Auth errors take priority; if both exist show auth.
    if (auth > 0) alerts.push({ connector, kind: 'auth', repoCount: auth });
    else if (mismatch > 0) alerts.push({ connector, kind: 'mismatch', repoCount: mismatch });
  }
  return alerts;
}



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

const REPOSITORIES_COLUMN_WIDTHS = ['35%', '16%', '16%', '16%', '17%'];

// How long to wait after the last keystroke in the Repository search box
// before actually firing the (now server-side) request - typing shouldn't
// send one request per character.
const SEARCH_DEBOUNCE_MS = 300;

const Repositories: React.FC = () => {
  const navigate = useNavigate();
  const dispatch = useAppDispatch();

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

  // Synchronize any connector-level sync errors detected on repositories to the Notifications store
  useEffect(() => {
    if (loading || repositories.length === 0 || connectors.length === 0) return;
    const alerts = computeConnectorAlerts(repositories, connectors);
    alerts.forEach((alert) => {
      const pName = providerLabel(alert.connector.provider);
      const wiki = wikiUrlForProvider(alert.connector.provider);
      const title = alert.kind === 'auth'
        ? `Your ${pName} provider credential expired, please renew it.`
        : `Provider Mismatch: ${pName} (${alert.connector.connection_name})`;
      const message = alert.kind === 'auth'
        ? `Connector "${alert.connector.connection_name}": ${alert.repoCount} ${alert.repoCount === 1 ? 'repository' : 'repositories'} could not sync due to expired or invalid credentials.`
        : `${alert.repoCount} ${alert.repoCount === 1 ? 'repository' : 'repositories'} have a mismatched provider configured.`;

      dispatch(
        addNotification({
          dedupeKey: `connector-${alert.kind}-${alert.connector.id}`,
          severity: 'error',
          title,
          message,
          source: 'connector',
          toast: false,
          actions: [
            { label: 'Renew Credentials', actionId: `/git/connector/${alert.connector.id}` },
            { label: `${pName} Setup Guide`, actionId: wiki },
          ],
        })
      );
    });
  }, [loading, repositories, connectors, dispatch]);

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
      notify.error('No connectors found. Connect a Git provider first.');
      return;
    }
    setSyncingAll(true);
    try {
      const results = await Promise.allSettled(
        connectors.map((c) => syncConnectorRepositories(c.id.toString()))
      );

      const failedEntries: { connector: ConnectorResponse; error: unknown }[] = [];
      results.forEach((res, idx) => {
        if (res.status === 'rejected') {
          failedEntries.push({ connector: connectors[idx], error: res.reason });
        }
      });

      const failedCount = failedEntries.length;
      const successCount = connectors.length - failedCount;

      if (successCount > 0) {
        notify.success(
          successCount === 1
            ? 'Sync started. Updating repositories and pull requests...'
            : `Sync started for all ${successCount} connectors. Updating repositories and pull requests...`
        );
      }

      if (failedCount > 0) {
        // Dispatch one clear notification per failed connector naming the provider
        failedEntries.forEach(({ connector, error: rawErr }) => {
          const pName = providerLabel(connector.provider);
          const wiki = wikiUrlForProvider(connector.provider);
          const errMsg = rawErr instanceof Error ? rawErr.message : String(rawErr || '');
          const errLower = errMsg.toLowerCase();
          const isForbidden = errLower.includes('403') || errLower.includes('forbidden');
          const isAuth =
            !isForbidden &&
            (errLower.includes('auth') ||
              errLower.includes('token') ||
              errLower.includes('401') ||
              errLower.includes('unauthorized') ||
              errLower.includes('credential') ||
              errLower.includes('bad credentials'));

          const kind = isAuth ? 'auth' : 'sync-error';
          const title = isAuth
            ? `${pName} credential expired — please renew it`
            : `Sync failed: ${pName} (${connector.connection_name})`;

          const message = isAuth
            ? `Connector "${connector.connection_name}": failed to authenticate with ${pName}. Please renew your access token.`
            : `Sync failed for ${pName} (${connector.connection_name}): ${errMsg || 'network or server error'}`;

          dispatch(
            addNotification({
              dedupeKey: `connector-${kind}-${connector.id}`,
              severity: 'error',
              title,
              message,
              source: 'connector',
              toast: true,
              actions: [
                { label: 'Renew Credentials', actionId: `/git/connector/${connector.id}` },
                { label: `${pName} Setup Guide`, actionId: wiki },
              ],
            })
          );
        });
      }

      // Give the backfill a moment to at least upsert repositories (PR data
      // arrives asynchronously via the job queue) before refreshing the list.
      setTimeout(() => {
        refetchCurrent();
        window.dispatchEvent(new Event('connector-sync-finished'));
      }, 2000);
    } finally {
      setSyncingAll(false);
    }
  };

  const columns = useMemo<ColumnDef<Repository>[]>(() => [
    {
      id: 'repository',
      size: 320,
      // Has its own link to the repo on the provider, so the row link stays out of the cell's content.
      meta: { rowLink: false },
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
      size: 160,
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
      size: 160,
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
      size: 160,
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
        const tooltipContent = repo.last_sync_status === 'error'
          ? (repo.last_sync_error || 'Sync failed: an error occurred during repository sync')
          : (repo.last_synced_at ? `Synced ${formatRelativeTime(repo.last_synced_at)}` : 'Never synced');

        return (
          <div className="flex items-center justify-center">
            <Tooltip content={tooltipContent}>
              <span
                className="inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-sm font-medium w-fit cursor-help"
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
      size: 220,
      meta: { rowLink: false },
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
            <Tooltip content="Repository settings">
              <Button
                variant="ghost"
                size="sm"
                onClick={(e) => {
                  e.stopPropagation();
                  navigate(`/explore/repositories/${repo.id}/settings`);
                }}
                aria-label="Repository settings"
                className="text-slate-500 hover:text-slate-300 px-2 cursor-pointer text-sm"
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
    columnResizeMode: 'onChange',
    enableColumnResizing: true,
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
          enableColumnResizing
          getRowHref={(repo) => `/explore/merge-requests?repository_id=${repo.id}`}
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
