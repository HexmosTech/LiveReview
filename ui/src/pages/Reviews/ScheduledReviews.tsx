import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ColumnDef,
  SortingState,
  ColumnFiltersState,
  PaginationState,
  RowSelectionState,
  OnChangeFn,
  useReactTable,
  getCoreRowModel,
} from '@tanstack/react-table';
import { notify } from '../../utils/notify';
import { LuSearch } from 'react-icons/lu';
import { SiGitlab } from 'react-icons/si';
import { PageHeader, Button, Icons, Toggle, Input, MultiSelectPanel, parseMultiFilterValue } from '../../components/UIPrimitives';
import { ClientTable } from '../../components/DataTable/ClientTable';
import { SortIcon, SortableHeaderLabel, HeaderFilterPopover, multiSelectFilterFn, TruncatedWithTooltip } from '../../components/DataTable/HeaderControls';
import { EditScheduleModal } from '../../components/reviews/cronbuilder/EditScheduleModal';
import { getLocalCronText } from '../../components/reviews/cronbuilder/cronTimezone';
import { getRepositories } from '../../api/repositories';
import { getScheduledReviewConfigs, setScheduledReview } from '../../api/scheduledReviews';
import { Repository, RepositoriesFilters, RepositoriesSort } from '../../types/explore';

const pageSizeOptions = [20, 50, 100];
const DEFAULT_CRON = '0 9 * * *';
const SEARCH_DEBOUNCE_MS = 300;
const SCHEDULED_REVIEWS_COLUMN_WIDTHS = ['4%', '23%', '9%', '8%', '8%', '10%', '10%', '24%', '4%'];

// Same normalization as Explore > Repositories - real provider values are compound strings like "github-com".
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

const REPO_NAME_MAX = 50;
const DESCRIPTION_MAX = 90;

const truncate = (text: string, max: number): string => (text.length > max ? `${text.slice(0, max)}…` : text);

interface RepoScheduleState {
  enabled: boolean;
  cronExpression: string;
  nextRunAt: string;
  lastRunAt?: string;
  toggleBusy: boolean;
}

const defaultScheduleState: RepoScheduleState = {
  enabled: false,
  cronExpression: DEFAULT_CRON,
  nextRunAt: '',
  lastRunAt: undefined,
  toggleBusy: false,
};

// last_run_at is absolute UTC - formatting without a `timeZone` override renders it in local time.
const formatLocalDateTime = (iso?: string): string => {
  if (!iso) return 'Never';
  const d = new Date(iso);
  if (isNaN(d.getTime())) return 'Never';
  return d.toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
};

const ScheduledReviews: React.FC = () => {
  const navigate = useNavigate();
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Default sort: enabled schedules first, most-recently-run first (see sort=schedule in ListRepositories).
  const [sorting, setSorting] = useState<SortingState>([{ id: 'schedule', desc: true }]);
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const [pagination, setPagination] = useState<PaginationState>({ pageIndex: 0, pageSize: pageSizeOptions[0] });
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});
  const searchDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const [scheduleByRepoId, setScheduleByRepoId] = useState<Record<number, RepoScheduleState>>({});
  const loadedConnectorIdsRef = useRef<Set<number>>(new Set());
  const [bulkBusy, setBulkBusy] = useState(false);
  // Client-side only - schedule state isn't part of /repositories, so it can't use columnFilters.
  const [scheduleStatusFilter, setScheduleStatusFilter] = useState('');

  const [editingRepos, setEditingRepos] = useState<Repository[] | null>(null);
  const [modalSaving, setModalSaving] = useState(false);

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

  useEffect(() => {
    fetchRepositories(buildFetchParams(sorting, columnFilters, pagination));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const refetchCurrent = useCallback(() => {
    fetchRepositories(buildFetchParams(sorting, columnFilters, pagination));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fetchRepositories, sorting, columnFilters, pagination]);

  // Pulls scheduled-review configs per connector on the current page, so toggles reflect real backend state.
  useEffect(() => {
    const connectorIds = Array.from(new Set(repositories.map((r) => r.connector_id))).filter(
      (id) => !loadedConnectorIdsRef.current.has(id)
    );
    if (connectorIds.length === 0) return;
    connectorIds.forEach((id) => loadedConnectorIdsRef.current.add(id));

    Promise.allSettled(connectorIds.map((id) => getScheduledReviewConfigs(id).then((configs) => ({ id, configs })))).then(
      (results) => {
        setScheduleByRepoId((prev) => {
          const next = { ...prev };
          for (const result of results) {
            if (result.status !== 'fulfilled') continue;
            const { id: connectorId, configs } = result.value;
            for (const repo of repositories) {
              if (repo.connector_id !== connectorId || next[repo.id]) continue;
              const match = configs.find((c) => c.repository_id === repo.id);
              next[repo.id] = match
                ? {
                    ...defaultScheduleState,
                    enabled: match.enabled,
                    cronExpression: match.cron_expression,
                    nextRunAt: match.next_run_at,
                    lastRunAt: match.last_run_at,
                  }
                : { ...defaultScheduleState };
            }
          }
          return next;
        });
      }
    );
  }, [repositories]);

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
      if (searchDebounceRef.current) clearTimeout(searchDebounceRef.current);
      searchDebounceRef.current = setTimeout(() => {
        setPagination(nextPagination);
        fetchRepositories(buildFetchParams(sorting, next, nextPagination));
      }, SEARCH_DEBOUNCE_MS);
      return;
    }

    setPagination(nextPagination);
    fetchRepositories(buildFetchParams(sorting, next, nextPagination));
  };

  const handlePaginationChange: OnChangeFn<PaginationState> = (updater) => {
    const next = typeof updater === 'function' ? updater(pagination) : updater;
    setPagination(next);
    fetchRepositories(buildFetchParams(sorting, columnFilters, next));
  };

  const applyToggle = async (repo: Repository, enabled: boolean) => {
    const current = scheduleByRepoId[repo.id] || defaultScheduleState;
    setScheduleByRepoId((prev) => ({
      ...prev,
      [repo.id]: { ...(prev[repo.id] || defaultScheduleState), toggleBusy: true },
    }));
    try {
      const cfg = await setScheduledReview(repo.connector_id, {
        repository_id: repo.id,
        enabled,
        cron_expression: current.cronExpression || DEFAULT_CRON,
      });
      setScheduleByRepoId((prev) => ({
        ...prev,
        [repo.id]: {
          ...(prev[repo.id] || defaultScheduleState),
          enabled: cfg.enabled,
          cronExpression: cfg.cron_expression,
          nextRunAt: cfg.next_run_at,
          lastRunAt: cfg.last_run_at,
          toggleBusy: false,
        },
      }));
    } catch {
      notify.error(`Failed to update schedule for ${repo.full_name}`);
      setScheduleByRepoId((prev) => ({
        ...prev,
        [repo.id]: { ...(prev[repo.id] || defaultScheduleState), toggleBusy: false },
      }));
    }
  };

  const selectedRepoIds = useMemo(
    () => Object.keys(rowSelection).filter((id) => rowSelection[id]).map((id) => Number(id)),
    [rowSelection]
  );
  const selectedRepos = useMemo(
    () => repositories.filter((r) => selectedRepoIds.includes(r.id)),
    [repositories, selectedRepoIds]
  );

  const bulkApply = async (enabled: boolean) => {
    if (selectedRepos.length === 0) return;
    setBulkBusy(true);
    const results = await Promise.allSettled(selectedRepos.map((repo) => applyToggle(repo, enabled)));
    const failed = results.filter((r) => r.status === 'rejected').length;
    setBulkBusy(false);
    setRowSelection({});
    if (failed === 0) {
      notify.success(`Scheduled review ${enabled ? 'enabled' : 'disabled'} for ${selectedRepos.length} repo(s).`);
    }
  };

  const handleSaveSchedule = async (cronExpression: string) => {
    if (!editingRepos || editingRepos.length === 0) return;
    setModalSaving(true);
    const results = await Promise.allSettled(
      editingRepos.map(async (repo) => {
        const cfg = await setScheduledReview(repo.connector_id, {
          repository_id: repo.id,
          enabled: true,
          cron_expression: cronExpression,
        });
        setScheduleByRepoId((prev) => ({
          ...prev,
          [repo.id]: {
            enabled: cfg.enabled,
            cronExpression: cfg.cron_expression,
            nextRunAt: cfg.next_run_at,
            lastRunAt: cfg.last_run_at,
            toggleBusy: false,
          },
        }));
      })
    );
    const failed = results.filter((r) => r.status === 'rejected').length;
    setModalSaving(false);
    setEditingRepos(null);
    if (editingRepos.length > 1) setRowSelection({});
    if (failed === 0) {
      notify.success(
        editingRepos.length === 1
          ? `Schedule saved for ${editingRepos[0].full_name}`
          : `Schedule saved for ${editingRepos.length} repositories`
      );
    } else {
      notify.error(`Failed to save schedule for ${failed} of ${editingRepos.length} repositor${editingRepos.length === 1 ? 'y' : 'ies'}`);
    }
  };

  const columns = useMemo<ColumnDef<Repository>[]>(() => [
    {
      id: 'select',
      enableSorting: false,
      enableColumnFilter: false,
      header: ({ table }) => (
        <input
          type="checkbox"
          checked={table.getIsAllPageRowsSelected()}
          onChange={table.getToggleAllPageRowsSelectedHandler()}
          className="h-4 w-4 rounded border-slate-500 bg-slate-800 text-blue-500"
          aria-label="Select all repositories on this page"
        />
      ),
      cell: ({ row }) => (
        <input
          type="checkbox"
          checked={row.getIsSelected()}
          onChange={row.getToggleSelectedHandler()}
          onClick={(e) => e.stopPropagation()}
          className="h-4 w-4 rounded border-slate-500 bg-slate-800 text-blue-500"
          aria-label={`Select ${row.original.full_name}`}
        />
      ),
    },
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
        const segments = repo.full_name.split('/');
        const leaf = segments.pop() as string;
        const pathPrefix = segments.join('/');
        const displayDescription = repo.description ? truncate(repo.description, DESCRIPTION_MAX) : '';
        return (
          <div className="flex flex-col justify-center gap-1 min-w-0 min-h-[44px]">
            <div className="min-w-0 truncate text-white">
              <TruncatedWithTooltip text={repo.full_name} max={REPO_NAME_MAX}>
                {pathPrefix && <span>{pathPrefix}/</span>}
                <span className="font-semibold">{leaf}</span>
              </TruncatedWithTooltip>
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
                  { value: 'gitlab', label: 'GitLab', disabled: true },
                  { value: 'bitbucket', label: 'Bitbucket', disabled: true },
                  { value: 'gitea', label: 'Gitea', disabled: true },
                  { value: 'azuredevops', label: 'Azure DevOps', disabled: true },
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
        <div className="text-center text-white text-sm whitespace-nowrap">{row.original.default_branch || '—'}</div>
      ),
    },
    {
      id: 'scheduled',
      enableSorting: false,
      enableColumnFilter: false,
      header: () => (
        <div className="flex items-center justify-between gap-2">
          <span className="font-semibold text-slate-300 uppercase tracking-wide text-xs">Scheduled</span>
          <HeaderFilterPopover>
            <MultiSelectPanel
              label="Schedule Status"
              value={scheduleStatusFilter}
              onChange={setScheduleStatusFilter}
              options={[
                { value: 'enabled', label: 'Enabled' },
                { value: 'not_scheduled', label: 'Not scheduled' },
              ]}
            />
          </HeaderFilterPopover>
        </div>
      ),
      cell: ({ row }) => {
        const repo = row.original;
        const state = scheduleByRepoId[repo.id];
        return (
          <div onClick={(e) => e.stopPropagation()} className="inline-flex">
            <Toggle
              checked={state?.enabled ?? false}
              isLoading={state?.toggleBusy}
              onChange={(checked) => applyToggle(repo, checked)}
              aria-label={`Toggle scheduled review for ${repo.full_name}`}
            />
          </div>
        );
      },
    },
    {
      id: 'schedule_summary',
      enableSorting: false,
      enableColumnFilter: false,
      header: () => <span className="font-semibold text-slate-300 uppercase tracking-wide text-xs">Schedule</span>,
      cell: ({ row }) => {
        const state = scheduleByRepoId[row.original.id];
        if (!state?.enabled) return <span className="text-sm text-slate-500 whitespace-nowrap">Not scheduled</span>;
        const summary = getLocalCronText(state.cronExpression).value;
        return (
          <span className="block text-sm text-slate-200 truncate" title={summary}>
            {summary}
          </span>
        );
      },
    },
    {
      id: 'last_run',
      enableSorting: false,
      enableColumnFilter: false,
      header: () => <span className="font-semibold text-slate-300 uppercase tracking-wide text-xs">Last Run</span>,
      cell: ({ row }) => {
        const state = scheduleByRepoId[row.original.id];
        return <span className="text-sm text-slate-400 whitespace-nowrap">{formatLocalDateTime(state?.lastRunAt)}</span>;
      },
    },
    {
      id: 'actions',
      enableSorting: false,
      enableColumnFilter: false,
      header: () => <span className="font-semibold text-slate-300 uppercase tracking-wide text-xs">Actions</span>,
      cell: ({ row }) => {
        const isScheduled = scheduleByRepoId[row.original.id]?.enabled ?? false;
        return (
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            className="whitespace-nowrap"
            onClick={(e) => {
              e.stopPropagation();
              setEditingRepos([row.original]);
            }}
          >
            Edit Schedule
          </Button>
          {isScheduled && (
            <Button
              variant="outline"
              size="sm"
              className="border-slate-400 text-white hover:bg-white/10 hover:border-white w-32 shrink-0 whitespace-nowrap text-sm"
              onClick={(e) => {
                e.stopPropagation();
                navigate(`/reviews?search=${encodeURIComponent(row.original.full_name)}&source=scheduled`);
              }}
            >
              View Reviews
            </Button>
          )}
        </div>
        );
      },
    },
    {
      id: 'chevron',
      enableSorting: false,
      enableColumnFilter: false,
      header: () => null,
      cell: () => (
        <div className="flex justify-end text-slate-500">
          <Icons.ChevronRight />
        </div>
      ),
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [scheduleByRepoId, scheduleStatusFilter, navigate]);

  // Local Scheduled-status filter only - schedule state isn't in /repositories, so this still can't be server-side; sort is server-side now via sort=schedule.
  const visibleRepositories = useMemo(() => {
    const selected = parseMultiFilterValue(scheduleStatusFilter);
    if (selected.length === 0) return repositories;
    return repositories.filter((repo) => {
      const key = scheduleByRepoId[repo.id]?.enabled ? 'enabled' : 'not_scheduled';
      return selected.includes(key);
    });
  }, [repositories, scheduleByRepoId, scheduleStatusFilter]);

  const table = useReactTable({
    data: visibleRepositories,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getRowId: (row) => String(row.id),
    manualFiltering: true,
    manualSorting: true,
    manualPagination: true,
    pageCount: totalPages,
    enableRowSelection: true,
    state: { sorting, columnFilters, pagination, rowSelection },
    onSortingChange: handleSortingChange,
    onColumnFiltersChange: handleColumnFiltersChange,
    onPaginationChange: handlePaginationChange,
    onRowSelectionChange: setRowSelection,
  });

  return (
    <div className="container mx-auto px-4 py-8">
      <PageHeader
        title="Scheduled Reviews"
        description="Automatically review a repository's default branch on a schedule you set."
      />

      {selectedRepos.length > 0 && (
        <div className="mb-4 flex items-center justify-between rounded-md border border-slate-700 bg-slate-800/60 px-4 py-2.5">
          <span className="text-sm text-slate-300">{selectedRepos.length} repositor{selectedRepos.length === 1 ? 'y' : 'ies'} selected</span>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" isLoading={bulkBusy} onClick={() => bulkApply(false)}>
              Disable
            </Button>
            <Button variant="outline" size="sm" onClick={() => setEditingRepos(selectedRepos)}>
              Edit Schedule
            </Button>
            <Button variant="outline" size="sm" isLoading={bulkBusy} onClick={() => bulkApply(true)}>
              Enable Scheduled Review
            </Button>
          </div>
        </div>
      )}

      <ClientTable
        table={table}
        columnWidths={SCHEDULED_REVIEWS_COLUMN_WIDTHS}
        loading={loading}
        loadingLabel="Loading repositories..."
        error={error}
        onRetry={refetchCurrent}
        isEmpty={total === 0 && columnFilters.length === 0}
        empty={{
          title: 'No repositories found',
          description: 'Connect a git provider to see repositories here.',
        }}
        pageSizeOptions={pageSizeOptions}
        manualTotal={total}
        onRowClick={(repo) => navigate(`/reviews/scheduled/${repo.id}/runs`)}
      />

      {editingRepos && editingRepos.length > 0 && (
        <EditScheduleModal
          subtitle={editingRepos.length === 1 ? editingRepos[0].full_name : `${editingRepos.length} repositories`}
          initialCronExpression={
            editingRepos.length === 1 ? scheduleByRepoId[editingRepos[0].id]?.cronExpression || DEFAULT_CRON : DEFAULT_CRON
          }
          onClose={() => setEditingRepos(null)}
          onSave={handleSaveSchedule}
          saving={modalSaving}
        />
      )}
    </div>
  );
};

export default ScheduledReviews;
