import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ColumnDef, useReactTable, getCoreRowModel } from '@tanstack/react-table';
import apiClient from '../../api/apiClient';
import { Button, Icons, Input, Badge } from '../../components/UIPrimitives';
import { ClientTable } from '../../components/DataTable/ClientTable';
import { useToast } from '../../components/NotificationToast';
import { ConfirmModal } from '../../components/ConfirmModal';
import { formatRelativeTime } from '../../api/reviews';
import { CIRuleset, Breadcrumb } from './shared';

const COLUMN_WIDTHS = ['22%', '24%', '24%', '12%', '18%'];

const CiRulesetsList: React.FC = () => {
  const navigate = useNavigate();
  const { showToast, ToastContainer } = useToast();

  const [rulesets, setRulesets] = useState<CIRuleset[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
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

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return rulesets;
    return rulesets.filter((r) => r.name.toLowerCase().includes(q) || r.description.toLowerCase().includes(q) || r.jq_expr.toLowerCase().includes(q));
  }, [rulesets, search]);

  const columns = useMemo<ColumnDef<CIRuleset>[]>(() => [
    {
      id: 'name',
      header: () => <span className="font-semibold text-slate-300 uppercase tracking-wide text-xs">Ruleset</span>,
      cell: ({ row }) => {
        const r = row.original;
        return (
          <div className="flex items-center gap-2 min-w-0">
            <span className="text-white text-sm font-medium truncate">{r.name}</span>
            <Badge variant="info" className="text-xs flex-shrink-0">#{r.id}</Badge>
          </div>
        );
      },
    },
    {
      id: 'description',
      header: () => <span className="font-semibold text-slate-300 uppercase tracking-wide text-xs">Description</span>,
      cell: ({ row }) => (
        <p className="text-sm text-slate-300 truncate">{row.original.description || <span className="text-slate-500">—</span>}</p>
      ),
    },
    {
      id: 'jq_expr',
      header: () => <span className="font-semibold text-slate-300 uppercase tracking-wide text-xs">jq expression</span>,
      cell: ({ row }) => (
        <code className="block text-xs bg-slate-900/70 border border-slate-700 rounded px-2 py-1 text-blue-200 font-mono truncate">
          {row.original.jq_expr}
        </code>
      ),
    },
    {
      id: 'updated',
      header: () => <span className="font-semibold text-slate-300 uppercase tracking-wide text-xs">Updated</span>,
      cell: ({ row }) => <span className="text-white text-sm">{formatRelativeTime(row.original.updated_at)}</span>,
    },
    {
      id: 'actions',
      enableSorting: false,
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
    data: filtered,
    columns,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    pageCount: 1,
    state: { pagination: { pageIndex: 0, pageSize: filtered.length || 1 } },
    onPaginationChange: () => undefined,
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

      {rulesets.length > 0 && (
        <div className="mb-4 max-w-sm">
          <Input placeholder="Search rulesets…" value={search} onChange={(e) => setSearch(e.target.value)} icon={<Icons.Search />} />
        </div>
      )}

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
        pageSizeOptions={[filtered.length || 1]}
        onRowClick={(r) => navigate(`/ci-rulesets/${r.id}/edit`)}
        manualTotal={filtered.length}
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
