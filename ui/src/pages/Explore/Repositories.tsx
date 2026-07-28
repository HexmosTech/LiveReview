import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { ColumnDef } from '@tanstack/react-table';
import toast from 'react-hot-toast';
import { Button, Icons } from '../../components/UIPrimitives';
import { DataTable } from '../../components/DataTable/DataTable';
import ExploreTabs from './ExploreTabs';
import { getRepositories, syncRepositoryPullRequests, syncConnectorRepositories } from '../../api/repositories';
import { getConnectors, ConnectorResponse } from '../../api/connectors';
import { formatRelativeTime } from '../../api/reviews';
import { Repository, RepositoriesFilters } from '../../types/explore';

const perPageOptions = [20, 50, 100];

const providerLabel = (provider: string): string => {
  const normalized = provider.toLowerCase();
  if (normalized.startsWith('github')) return 'GitHub';
  if (normalized.startsWith('gitlab')) return 'GitLab';
  if (normalized.startsWith('bitbucket')) return 'Bitbucket';
  if (normalized.startsWith('gitea')) return 'Gitea';
  if (normalized.startsWith('azuredevops')) return 'Azure DevOps';
  return provider;
};

const ProviderIcon: React.FC<{ provider: string }> = ({ provider }) => {
  const normalized = provider.toLowerCase();
  if (normalized.startsWith('github')) return <Icons.GitHub />;
  if (normalized.startsWith('gitlab')) return <Icons.GitLab />;
  if (normalized.startsWith('bitbucket')) return <Icons.Bitbucket />;
  if (normalized.startsWith('gitea')) return <Icons.Gitea />;
  if (normalized.startsWith('azuredevops')) return <Icons.AzureDevOps />;
  return null;
};

const syncStatusBadge = (status: string): { label: string; className: string } => {
  switch (status) {
    case 'ok':
      return { label: 'Synced', className: 'bg-green-900/60 text-green-200 border border-green-600/40' };
    case 'error':
      return { label: 'Sync error', className: 'bg-red-900/60 text-red-200 border border-red-600/40' };
    default:
      return { label: 'Pending', className: 'bg-slate-700 text-slate-300 border border-slate-600' };
  }
};

const Repositories: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [connectors, setConnectors] = useState<ConnectorResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(1);
  const [syncingRepoId, setSyncingRepoId] = useState<number | null>(null);
  const [syncingAll, setSyncingAll] = useState(false);

  const [filters, setFilters] = useState<RepositoriesFilters>({ page: 1, perPage: 20 });
  const [searchQuery, setSearchQuery] = useState('');
  const [connectorFilter, setConnectorFilter] = useState('');
  const [providerFilter, setProviderFilter] = useState('');

  useEffect(() => {
    const initialFilters: RepositoriesFilters = {
      page: parseInt(searchParams.get('page') || '1', 10),
      perPage: parseInt(searchParams.get('per_page') || '20', 10),
      connectorId: searchParams.get('connector_id') || undefined,
      provider: searchParams.get('provider') || undefined,
      search: searchParams.get('search') || undefined,
    };
    setFilters(initialFilters);
    setSearchQuery(initialFilters.search || '');
    setConnectorFilter(initialFilters.connectorId || '');
    setProviderFilter(initialFilters.provider || '');
  }, [searchParams]);

  useEffect(() => {
    getConnectors()
      .then(setConnectors)
      .catch((err) => console.error('Failed to load connectors:', err));
  }, []);

  const fetchRepositories = useCallback(async (requestFilters?: RepositoriesFilters) => {
    try {
      setLoading(true);
      setError(null);
      const response = await getRepositories(requestFilters || filters);
      setRepositories(response.repositories || []);
      setTotal(response.total || 0);
      setTotalPages(response.total_pages || 1);
    } catch (err) {
      console.error('Error fetching repositories:', err);
      setError(err instanceof Error ? err.message : 'Failed to fetch repositories');
    } finally {
      setLoading(false);
    }
  }, [filters]);

  useEffect(() => {
    fetchRepositories(filters);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters]);

  const updateFilters = useCallback((newFilters: Partial<RepositoriesFilters>) => {
    const updated = { ...filters, ...newFilters };
    if (!newFilters.page) updated.page = 1;
    setFilters(updated);

    const params = new URLSearchParams();
    if (updated.page && updated.page > 1) params.set('page', updated.page.toString());
    if (updated.perPage && updated.perPage !== 20) params.set('per_page', updated.perPage.toString());
    if (updated.connectorId) params.set('connector_id', updated.connectorId);
    if (updated.provider) params.set('provider', updated.provider);
    if (updated.search) params.set('search', updated.search);
    setSearchParams(params);
  }, [filters, setSearchParams]);

  const handleSearch = () => updateFilters({ search: searchQuery || undefined });
  const handleConnectorFilter = (value: string) => {
    setConnectorFilter(value);
    updateFilters({ connectorId: value || undefined });
  };
  const handleProviderFilter = (value: string) => {
    setProviderFilter(value);
    updateFilters({ provider: value || undefined });
  };
  const clearFilters = () => {
    setSearchQuery('');
    setConnectorFilter('');
    setProviderFilter('');
    setSearchParams(new URLSearchParams());
    setFilters({ page: 1, perPage: 20 });
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
      setTimeout(() => fetchRepositories(), 2000);
    } finally {
      setSyncingAll(false);
    }
  };

  const columns = useMemo<ColumnDef<Repository>[]>(() => [
    {
      id: 'repository',
      header: 'Repository',
      cell: ({ row }) => {
        const repo = row.original;
        return (
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-2">
              <span className="text-slate-400"><ProviderIcon provider={repo.provider} /></span>
              <span className="text-white font-semibold">{repo.full_name}</span>
              {repo.is_private && (
                <span className="text-[10px] uppercase tracking-wide px-1.5 py-0.5 rounded bg-slate-700 text-slate-300 border border-slate-600">
                  Private
                </span>
              )}
              <a
                href={repo.web_url}
                target="_blank"
                rel="noopener noreferrer"
                onClick={(e) => e.stopPropagation()}
                className="text-blue-400 hover:text-blue-300 text-xs font-medium underline underline-offset-2"
              >
                Open
              </a>
            </div>
            {repo.description && (
              <div className="text-xs text-slate-400 truncate max-w-md" title={repo.description}>
                {repo.description}
              </div>
            )}
          </div>
        );
      },
    },
    {
      id: 'provider',
      header: 'Provider',
      cell: ({ row }) => <span className="text-slate-300">{providerLabel(row.original.provider)}</span>,
    },
    {
      id: 'default_branch',
      header: 'Default Branch',
      cell: ({ row }) => <span className="text-slate-400 font-mono text-sm">{row.original.default_branch || '—'}</span>,
    },
    {
      id: 'sync_status',
      header: 'Sync Status',
      cell: ({ row }) => {
        const repo = row.original;
        const badge = syncStatusBadge(repo.last_sync_status);
        return (
          <div className="flex flex-col gap-1">
            <span className={`inline-flex w-fit items-center px-2 py-0.5 rounded-full text-xs font-medium ${badge.className}`}>
              {badge.label}
            </span>
            <span className="text-xs text-slate-500">
              {repo.last_synced_at ? formatRelativeTime(repo.last_synced_at) : 'Never synced'}
            </span>
          </div>
        );
      },
    },
    {
      id: 'actions',
      header: 'Actions',
      cell: ({ row }) => {
        const repo = row.original;
        return (
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={(e) => {
                e.stopPropagation();
                navigate(`/explore/merge-requests?repository_id=${repo.id}`);
              }}
              className="border-slate-600 text-slate-300 hover:text-white hover:border-slate-500"
            >
              View PRs
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={(e) => handleSync(repo, e)}
              disabled={syncingRepoId === repo.id}
              className="border-slate-600 text-slate-300 hover:text-white hover:border-slate-500"
              icon={<Icons.Refresh />}
            >
              {syncingRepoId === repo.id ? 'Syncing…' : 'Sync'}
            </Button>
          </div>
        );
      },
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [syncingRepoId, navigate]);

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

      <div className="bg-slate-800 rounded-lg p-6 mb-6 border border-slate-700">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <div className="md:col-span-2">
            <div className="flex">
              <input
                type="text"
                placeholder="Search repositories..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
                className="flex-1 px-3 py-2 bg-slate-700 border border-slate-600 rounded-l-md text-white placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              />
              <Button onClick={handleSearch} variant="primary" className="rounded-l-none px-4">
                <Icons.Search />
              </Button>
            </div>
          </div>
          <div>
            <select
              value={connectorFilter}
              onChange={(e) => handleConnectorFilter(e.target.value)}
              className="w-full px-3 py-2 bg-slate-700 border border-slate-600 rounded-md text-white focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            >
              <option value="">All Connectors</option>
              {connectors.map((c) => (
                <option key={c.id} value={c.id}>{c.connection_name}</option>
              ))}
            </select>
          </div>
          <div>
            <select
              value={providerFilter}
              onChange={(e) => handleProviderFilter(e.target.value)}
              className="w-full px-3 py-2 bg-slate-700 border border-slate-600 rounded-md text-white focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            >
              <option value="">All Providers</option>
              <option value="github">GitHub</option>
              <option value="gitlab">GitLab</option>
            </select>
          </div>
        </div>

        {(searchQuery || connectorFilter || providerFilter) && (
          <div className="flex items-center justify-between mt-4 pt-4 border-t border-slate-700">
            <div className="flex items-center space-x-2 text-sm text-slate-300">
              <span>Active filters:</span>
              {searchQuery && <span className="bg-blue-600 px-2 py-1 rounded text-white">Search: "{searchQuery}"</span>}
              {connectorFilter && (
                <span className="bg-blue-600 px-2 py-1 rounded text-white">
                  Connector: {connectors.find((c) => c.id.toString() === connectorFilter)?.connection_name || connectorFilter}
                </span>
              )}
              {providerFilter && <span className="bg-blue-600 px-2 py-1 rounded text-white">Provider: {providerLabel(providerFilter)}</span>}
            </div>
            <Button onClick={clearFilters} variant="ghost" className="text-slate-400 hover:text-white">
              Clear all
            </Button>
          </div>
        )}
      </div>

      <DataTable
        data={repositories}
        columns={columns}
        getRowId={(repo) => repo.id}
        loading={loading}
        loadingLabel="Loading repositories..."
        error={error}
        onRetry={() => fetchRepositories()}
        empty={{
          title: 'No repositories found',
          description: connectors.length > 0
            ? 'Your connector may not have synced yet, or your filters exclude everything. Try syncing.'
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
        pagination={{
          page: filters.page || 1,
          perPage: filters.perPage || 20,
          total,
          totalPages,
          onPageChange: (page) => updateFilters({ page }),
          onPerPageChange: (perPage) => updateFilters({ perPage }),
          perPageOptions,
        }}
      />
    </div>
  );
};

export default Repositories;
