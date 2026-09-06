import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ColumnDef, useReactTable, getCoreRowModel, getFilteredRowModel, getSortedRowModel, getPaginationRowModel } from '@tanstack/react-table';
import { LuSearch } from 'react-icons/lu';
import apiClient from '../../api/apiClient';
import { Button, Icons, Input, Badge } from '../../components/UIPrimitives';
import { ClientTable } from '../../components/DataTable/ClientTable';
import { SortIcon, SortableHeaderLabel, HeaderFilterPopover, TruncatedWithTooltip } from '../../components/DataTable/HeaderControls';
import { useToast } from '../../components/NotificationToast';
import { ConfirmModal } from '../../components/ConfirmModal';
import { formatRelativeTime } from '../../api/reviews';
import { CIRuleset, Breadcrumb } from './shared';

const COLUMN_WIDTHS = ['26%', '26%', '22%', '12%', '14%'];
const pageSizeOptions = [20, 50, 100];
const TEXT_MAX = 80;

const truncate = (text: string, max: number): string =>
  text.length > max ? `${text.slice(0, max - 1)}…` : text;

const CiRulesetsList: React.FC = () => {
  const navigate = useNavigate();
  const { showToast, ToastContainer } = useToast();

  const [rulesets, setRulesets] = useState<CIRuleset[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<CIRuleset | null>(null);

  const loadRulesets = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await apiClient.get<{ rows?: CIRuleset[] }>('/ci-rulesets');
      setRulesets(res.rows || []);
    } catch (err: any) {
      setError(err?.message || 'Failed to load CI/CD rulesets');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadRulesets();
  }, [loadRulesets]);

  const doDelete = async () => {
    if (!deleteTarget) return;
    try {
      await apiClient.delete(`/ci-rulesets/${deleteTarget.id}`);
      showToast('Ruleset deleted', 'success');
      setDeleteTarget(null);
      loadRulesets();
    } catch (err: any) {
      showToast(err?.message || 'Delete failed', 'error');
    }
  };

  const columns = useMemo<ColumnDef<CIRuleset>[]>(() => [
    {
      id: 'name',
      accessorFn: (r) => r.name,
      // Searches name/description/jq expression together, one combined
      // free-text search box, same convention as the Reviews list.
      filterFn: (row, _columnId, filterValue: string) => {
        const q = (filterValue || '').trim().toLowerCase();
        if (!q) return true;
        const r = row.original;
        return [r.name, r.description, r.jq_expr]
          .filter((v): v is string => Boolean(v))
          .some((v) => v.toLowerCase().includes(q));
      },
      header: ({ column }) => (
        <div className="flex items-center justify-between gap-2">
          <SortableHeaderLabel label="Ruleset" onToggle={column.getToggleSortingHandler()} />
          <div className="flex items-center gap-2">
            <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
            <HeaderFilterPopover icon={LuSearch} label="Search rulesets">
              <Input
                placeholder="Search name, description, or jq..."
                value={(column.getFilterValue() as string) ?? ''}
                onChange={(e) => column.setFilterValue(e.target.value || undefined)}
                icon={<Icons.Search />}
                aria-label="Search rulesets"
                className="text-sm"
              />
            </HeaderFilterPopover>
          </div>
        </div>
      ),
      cell: ({ row }) => {
        const r = row.original;
        return (
          <div className="flex items-center gap-2 min-w-0">
            <TruncatedWithTooltip text={r.name} max={TEXT_MAX}>
              <span className="text-white text-sm font-semibold truncate min-w-0">{truncate(r.name, TEXT_MAX)}</span>
            </TruncatedWithTooltip>
            <Badge variant="info" className="text-xs flex-shrink-0">#{r.id}</Badge>
          </div>
        );
      },
    },
    {
      id: 'description',
      accessorFn: (r) => r.description || '',
      enableColumnFilter: false,
      header: ({ column }) => (
        <div className="flex items-center justify-between gap-2">
          <SortableHeaderLabel label="Description" onToggle={column.getToggleSortingHandler()} />
          <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
        </div>
      ),
      cell: ({ row }) => {
        const description = row.original.description || '';
        if (!description) return <span className="text-slate-500 text-sm">—</span>;
        return (
          <TruncatedWithTooltip text={description} max={TEXT_MAX}>
            <p className="text-sm text-slate-300 truncate">{truncate(description, TEXT_MAX)}</p>
          </TruncatedWithTooltip>
        );
      },
    },
    {
      id: 'jq_expr',
      accessorFn: (r) => r.jq_expr,
      enableColumnFilter: false,
      enableSorting: false,
      header: () => <span className="font-semibold text-slate-300 uppercase tracking-wide text-xs">jq expression</span>,
      cell: ({ row }) => (
        <code className="block text-xs bg-slate-900/70 border border-slate-700 rounded px-2 py-1 text-blue-200 font-mono truncate">
          {row.original.jq_expr}
        </code>
      ),
    },
    {
      id: 'updated',
      accessorFn: (r) => r.updated_at,
      enableColumnFilter: false,
      header: ({ column }) => (
        <div className="flex items-center justify-between gap-2">
          <SortableHeaderLabel label="Updated" onToggle={column.getToggleSortingHandler()} />
          <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
        </div>
      ),
      cell: ({ row }) => <span className="text-white text-sm">{formatRelativeTime(row.original.updated_at)}</span>,
    },
    {
      id: 'actions',
      enableSorting: false,
      enableColumnFilter: false,
      header: () => <span className="font-semibold text-slate-300 uppercase tracking-wide text-xs">Actions</span>,
      cell: ({ row }) => {
        const r = row.original;
        return (
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={(e) => { e.stopPropagation(); navigate(`/ci-rulesets/${r.id}/integration`); }}
              className="border-slate-400 text-white hover:bg-white/10 hover:border-white text-sm cursor-pointer"
            >
              Get code
            </Button>
            <Button variant="ghost" size="sm" onClick={(e) => { e.stopPropagation(); navigate(`/ci-rulesets/${r.id}/edit`); }}>
              Edit
            </Button>
            <Button variant="danger" size="sm" onClick={(e) => { e.stopPropagation(); setDeleteTarget(r); }}>
              Delete
            </Button>
          </div>
        );
      },
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [navigate]);

  const table = useReactTable({
    data: rulesets,
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
      <ToastContainer />
      <Breadcrumb items={[{ label: 'Reviews', to: '/reviews' }, { label: 'CI/CD Gates' }]} />
      <div className="flex items-center justify-between mb-8 gap-4">
        <div>
          <h1 className="text-3xl font-bold text-white mb-2">CI/CD Gates</h1>
          <p className="text-slate-300">Write a jq rule against a review's findings, save it, and call one URL from any CI/CD pipeline to block or allow the build.</p>
        </div>
        <Button as={Link} to="/ci-rulesets/new" variant="primary" icon={<Icons.Add />}>
          New Ruleset
        </Button>
      </div>

      <ClientTable
        table={table}
        columnWidths={COLUMN_WIDTHS}
        loading={loading}
        loadingLabel="Loading rulesets..."
        error={error}
        onRetry={loadRulesets}
        isEmpty={rulesets.length === 0}
        empty={{
          title: 'No CI/CD rulesets yet',
          description: 'Create one to start gating merges on review findings (e.g. block on any critical or security issue).',
          action: <Button as={Link} to="/ci-rulesets/new" variant="primary" icon={<Icons.Add />}>New Ruleset</Button>,
        }}
        pageSizeOptions={pageSizeOptions}
        onRowClick={(r) => navigate(`/ci-rulesets/${r.id}/edit`)}
      />

      <ConfirmModal
        show={!!deleteTarget}
        title="Delete ruleset"
        message={`Delete "${deleteTarget?.name}"? Any CI/CD pipeline still calling it will start getting 404s.`}
        confirmText="Delete"
        type="danger"
        onConfirm={doDelete}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
};

export default CiRulesetsList;
