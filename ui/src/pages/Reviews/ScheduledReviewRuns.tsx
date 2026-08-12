import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ColumnDef,
  SortingState,
  ColumnFiltersState,
  PaginationState,
  OnChangeFn,
  useReactTable,
  getCoreRowModel,
} from '@tanstack/react-table';
import { SiGithub } from 'react-icons/si';
import toast from 'react-hot-toast';
import { Button, Icons, MultiSelectPanel, Toggle, Tooltip } from '../../components/UIPrimitives';
import { ClientTable } from '../../components/DataTable/ClientTable';
import { SortIcon, SortableHeaderLabel, HeaderFilterPopover } from '../../components/DataTable/HeaderControls';
import { EditScheduleModal } from '../../components/reviews/cronbuilder/EditScheduleModal';
import { getLocalCronText } from '../../components/reviews/cronbuilder/cronTimezone';
import { getRepository } from '../../api/repositories';
import { getScheduledReviewConfigs, getScheduledReviewRuns, setScheduledReview } from '../../api/scheduledReviews';
import { Repository } from '../../types/explore';
import { ScheduledReviewConfig, ScheduledReviewRun, ScheduledReviewRunOutcome, ScheduledReviewRunsFilters } from '../../types/reviews';

const pageSizeOptions = [20, 50, 100];
const DEFAULT_CRON = '0 9 * * *';
const RUNS_COLUMN_WIDTHS = ['20%', '18%', '30%', '12%', '20%'];

const OUTCOME_STYLES: Record<ScheduledReviewRunOutcome, string> = {
  reviewed: 'bg-transparent text-blue-400 border-blue-500',
  no_changes: 'bg-transparent text-slate-400 border-slate-600',
  failed: 'bg-red-900/30 text-red-300 border-slate-400',
  skipped_unsupported_provider: 'bg-amber-900/30 text-amber-300 border-slate-400',
  quota_blocked: 'bg-amber-900/30 text-amber-300 border-slate-400',
};

const OUTCOME_LABELS: Record<ScheduledReviewRunOutcome, string> = {
  reviewed: 'Reviewed',
  no_changes: 'No Code Changes',
  failed: 'Failed',
  skipped_unsupported_provider: 'Unsupported Provider',
  quota_blocked: 'Quota Blocked',
};

const OUTCOME_DESCRIPTIONS: Record<ScheduledReviewRunOutcome, string> = {
  reviewed: 'The scheduler found new commits and completed an AI review.',
  no_changes: 'No new code changes since the last run (no new commits, or commits with no diff) - nothing to review.',
  failed: 'The scheduler ran but hit an error.',
  skipped_unsupported_provider: 'Scheduled reviews currently only support GitHub repositories.',
  quota_blocked: "Skipped because the organization's AI usage quota was exceeded at the time of this run.",
};

const HeaderStat: React.FC<{ label: string; value: string; className?: string }> = ({ label, value, className }) => (
  <div className="shrink-0">
    <p className="text-[11px] text-slate-400 whitespace-nowrap">{label}</p>
    <p className={`text-sm text-white whitespace-nowrap ${className || ''}`}>{value}</p>
  </div>
);

const shortSha = (sha?: string): string => (sha ? sha.substring(0, 7) : '');

const formatDateTime = (iso: string): string => {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return '—';
  return d.toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' });
};

const formatDuration = (startedAt: string, completedAt?: string): string => {
  if (!completedAt) return '—';
  const ms = new Date(completedAt).getTime() - new Date(startedAt).getTime();
  if (!(ms >= 0)) return '—';
  const totalSeconds = Math.round(ms / 1000);
  if (totalSeconds < 60) return `${totalSeconds}s`;
  return `${Math.floor(totalSeconds / 60)}m ${totalSeconds % 60}s`;
};

const ScheduledReviewRuns: React.FC = () => {
  const { repositoryId } = useParams<{ repositoryId: string }>();
  const navigate = useNavigate();
  const repoId = Number(repositoryId);

  const [repository, setRepository] = useState<Repository | null>(null);
  const [config, setConfig] = useState<ScheduledReviewConfig | null>(null);
  const [toggleBusy, setToggleBusy] = useState(false);
  const [editingSchedule, setEditingSchedule] = useState(false);
  const [modalSaving, setModalSaving] = useState(false);
  const [runs, setRuns] = useState<ScheduledReviewRun[]>([]);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [sorting, setSorting] = useState<SortingState>([{ id: 'started_at', desc: true }]);
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const [pagination, setPagination] = useState<PaginationState>({ pageIndex: 0, pageSize: pageSizeOptions[0] });

  const buildFetchParams = (
    sortingState: SortingState,
    filtersState: ColumnFiltersState,
    paginationState: PaginationState
  ): ScheduledReviewRunsFilters => {
    const sortEntry = sortingState[0];
    const outcome = filtersState.find((f) => f.id === 'outcome')?.value as string | undefined;
    return {
      page: paginationState.pageIndex + 1,
      perPage: paginationState.pageSize,
      outcome: outcome || undefined,
      order: sortEntry ? (sortEntry.desc ? 'desc' : 'asc') : undefined,
    };
  };

  const fetchRuns = useCallback(async (params: ScheduledReviewRunsFilters) => {
    if (!repoId) return;
    try {
      setLoading(true);
      setError(null);
      const response = await getScheduledReviewRuns(repoId, params);
      setRuns(response.runs || []);
      setTotal(response.total || 0);
      setTotalPages(response.total_pages || 1);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch run history');
    } finally {
      setLoading(false);
    }
  }, [repoId]);

  useEffect(() => {
    if (!repoId) return;
    getRepository(repoId).then((repo) => {
      setRepository(repo);
      getScheduledReviewConfigs(repo.connector_id)
        .then((configs) => setConfig(configs.find((c) => c.repository_id === repoId) || null))
        .catch(() => setConfig(null));
    }).catch(() => setRepository(null));
    fetchRuns(buildFetchParams(sorting, columnFilters, pagination));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [repoId]);

  const applyToggle = async (enabled: boolean) => {
    if (!repository) return;
    setToggleBusy(true);
    try {
      const cfg = await setScheduledReview(repository.connector_id, {
        repository_id: repository.id,
        enabled,
        cron_expression: config?.cron_expression || DEFAULT_CRON,
      });
      setConfig(cfg);
    } catch {
      toast.error(`Failed to update schedule for ${repository.full_name}`);
    } finally {
      setToggleBusy(false);
    }
  };

  const handleSaveSchedule = async (cronExpression: string) => {
    if (!repository) return;
    setModalSaving(true);
    try {
      const cfg = await setScheduledReview(repository.connector_id, {
        repository_id: repository.id,
        enabled: true,
        cron_expression: cronExpression,
      });
      setConfig(cfg);
      toast.success(`Schedule saved for ${repository.full_name}`);
      setEditingSchedule(false);
    } catch {
      toast.error(`Failed to save schedule for ${repository.full_name}`);
    } finally {
      setModalSaving(false);
    }
  };

  const handleSortingChange: OnChangeFn<SortingState> = (updater) => {
    const next = typeof updater === 'function' ? updater(sorting) : updater;
    const nextPagination = { ...pagination, pageIndex: 0 };
    setSorting(next);
    setPagination(nextPagination);
    fetchRuns(buildFetchParams(next, columnFilters, nextPagination));
  };

  const handleColumnFiltersChange: OnChangeFn<ColumnFiltersState> = (updater) => {
    const next = typeof updater === 'function' ? updater(columnFilters) : updater;
    setColumnFilters(next);
    const nextPagination = { ...pagination, pageIndex: 0 };
    setPagination(nextPagination);
    fetchRuns(buildFetchParams(sorting, next, nextPagination));
  };

  const handlePaginationChange: OnChangeFn<PaginationState> = (updater) => {
    const next = typeof updater === 'function' ? updater(pagination) : updater;
    setPagination(next);
    fetchRuns(buildFetchParams(sorting, columnFilters, next));
  };

  const githubBaseUrl = useMemo(() => {
    if (!repository || !repository.provider.toLowerCase().startsWith('github')) return null;
    return `https://github.com/${repository.full_name}`;
  }, [repository]);

  const columns = useMemo<ColumnDef<ScheduledReviewRun>[]>(() => [
    {
      id: 'started_at',
      header: ({ column }) => (
        <div className="flex items-center justify-center gap-2">
          <SortableHeaderLabel label="Run Time" onToggle={column.getToggleSortingHandler()} />
          <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
        </div>
      ),
      cell: ({ row }) => <div className="text-center text-white text-sm">{formatDateTime(row.original.started_at)}</div>,
    },
    {
      id: 'outcome',
      enableSorting: false,
      header: ({ column }) => (
        <div className="flex items-center justify-center gap-2">
          <span className="font-semibold text-slate-300 uppercase tracking-wide text-xs">Outcome</span>
          <HeaderFilterPopover>
            <MultiSelectPanel
              label="Outcomes"
              value={(column.getFilterValue() as string) ?? ''}
              onChange={(v) => column.setFilterValue(v || undefined)}
              options={[
                { value: 'reviewed', label: 'Reviewed' },
                { value: 'no_changes', label: 'No Code Changes' },
                { value: 'failed', label: 'Failed' },
                { value: 'skipped_unsupported_provider', label: 'Unsupported Provider' },
                { value: 'quota_blocked', label: 'Quota Blocked' },
              ]}
            />
          </HeaderFilterPopover>
        </div>
      ),
      cell: ({ row }) => {
        const outcome = row.original.outcome;
        const tooltip = row.original.error_message
          ? `${OUTCOME_DESCRIPTIONS[outcome]} ${row.original.error_message}`
          : OUTCOME_DESCRIPTIONS[outcome];
        return (
          <div className="flex justify-center">
            <Tooltip content={tooltip}>
              <span className={`inline-flex items-center text-xs uppercase tracking-wide rounded-full px-2.5 py-1 border cursor-help ${OUTCOME_STYLES[outcome]}`}>
                {OUTCOME_LABELS[outcome]}
              </span>
            </Tooltip>
          </div>
        );
      },
    },
    {
      id: 'commits',
      enableSorting: false,
      header: () => <div className="text-center font-semibold text-slate-300 uppercase tracking-wide text-xs">Commits</div>,
      cell: ({ row }) => {
        const r = row.original;
        // base_sha == head_sha means literally nothing new happened - a compare link would just diff a commit against itself.
        if (!r.base_sha || !r.head_sha || r.base_sha === r.head_sha) {
          return <div className="text-center text-slate-500 text-sm">—</div>;
        }
        const label = `${r.commit_count > 0 ? `${r.commit_count} · ` : ''}${shortSha(r.base_sha)}…${shortSha(r.head_sha)}`;
        if (githubBaseUrl) {
          return (
            <div className="flex justify-center">
              <a
                href={`${githubBaseUrl}/compare/${r.base_sha}...${r.head_sha}`}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1.5 text-white hover:text-blue-300 text-sm font-mono"
              >
                <SiGithub className="w-4 h-4 shrink-0" />
                {label}
              </a>
            </div>
          );
        }
        return <div className="text-center text-white text-sm font-mono">{label}</div>;
      },
    },
    {
      id: 'duration',
      enableSorting: false,
      header: () => <div className="text-center font-semibold text-slate-300 uppercase tracking-wide text-xs">Duration</div>,
      cell: ({ row }) => <div className="text-center text-white text-sm">{formatDuration(row.original.started_at, row.original.completed_at)}</div>,
    },
    {
      id: 'actions',
      enableSorting: false,
      header: () => <div className="text-center font-semibold text-slate-300 uppercase tracking-wide text-xs">Actions</div>,
      cell: ({ row }) => {
        const r = row.original;
        if (r.outcome !== 'reviewed' || !r.review_id) {
          return <div className="text-center text-slate-500 text-sm">—</div>;
        }
        return (
          <div className="flex justify-center">
            <Button
              as={Link}
              to={`/reviews/${r.review_id}`}
              variant="outline"
              size="sm"
              className="border-slate-400 text-white hover:bg-white/10 hover:border-white whitespace-nowrap"
            >
              View Review
            </Button>
          </div>
        );
      },
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [githubBaseUrl]);

  const table = useReactTable({
    data: runs,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getRowId: (row) => String(row.id),
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
      <div className="mb-4">
        <Button variant="ghost" onClick={() => navigate('/reviews/scheduled')}>
          ← Back
        </Button>
      </div>

      <div className="bg-slate-800 rounded-lg border border-slate-700 mb-6 px-4 py-3 flex items-center gap-6 overflow-x-auto">
        <div className="flex items-center gap-2 shrink-0">
          <div className="text-slate-400"><Icons.Git /></div>
          <p className="text-lg font-semibold text-white whitespace-nowrap">{repository?.full_name || 'Run History'}</p>
        </div>
        <HeaderStat label="Branch" value={repository?.default_branch || '—'} />
        <HeaderStat label="Schedule" value={config ? getLocalCronText(config.cron_expression).value : '—'} />
        <HeaderStat label="Last Run" value={config?.last_run_at ? formatDateTime(config.last_run_at) : 'Never'} />
        <HeaderStat label="Next Run" value={config?.next_run_at ? formatDateTime(config.next_run_at) : '—'} />
        <div className="ml-auto shrink-0 flex items-center gap-3">
          <Button variant="outline" size="sm" className="whitespace-nowrap" onClick={() => setEditingSchedule(true)}>
            Edit Schedule
          </Button>
          <Toggle
            checked={config?.enabled ?? false}
            isLoading={toggleBusy}
            onChange={applyToggle}
            aria-label={`Toggle scheduled review for ${repository?.full_name || 'this repository'}`}
          />
        </div>
      </div>

      {editingSchedule && (
        <EditScheduleModal
          subtitle={repository?.full_name || ''}
          initialCronExpression={config?.cron_expression || DEFAULT_CRON}
          onClose={() => setEditingSchedule(false)}
          onSave={handleSaveSchedule}
          saving={modalSaving}
        />
      )}

      <h2 className="text-xl font-semibold text-white mb-3">Run History ({total})</h2>

      <ClientTable
        table={table}
        columnWidths={RUNS_COLUMN_WIDTHS}
        loading={loading}
        loadingLabel="Loading run history..."
        error={error}
        onRetry={() => fetchRuns(buildFetchParams(sorting, columnFilters, pagination))}
        isEmpty={total === 0 && columnFilters.length === 0}
        empty={{
          title: 'No runs yet',
          description: 'This repo has no scheduled-review run history. Runs are recorded starting from when scheduling was enabled.',
        }}
        pageSizeOptions={pageSizeOptions}
        manualTotal={total}
      />
    </div>
  );
};

export default ScheduledReviewRuns;
