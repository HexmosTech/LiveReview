import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { ColumnDef } from '@tanstack/react-table';
import toast from 'react-hot-toast';
import { Button, Icons } from '../../components/UIPrimitives';
import { DataTable } from '../../components/DataTable/DataTable';
import ExploreTabs from './ExploreTabs';
import { getPullRequests, getPRStateColor, getPRStateText, triggerReviewForPullRequest } from '../../api/pullRequests';
import { formatRelativeTime } from '../../api/reviews';
import { PullRequestState, PullRequestsFilters, PullRequestWithRepo } from '../../types/explore';

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

const MergeRequests: React.FC = () => {
  const [searchParams, setSearchParams] = useSearchParams();

  const [pullRequests, setPullRequests] = useState<PullRequestWithRepo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(1);
  const [triggeringId, setTriggeringId] = useState<number | null>(null);

  const [filters, setFilters] = useState<PullRequestsFilters>({ page: 1, perPage: 20, state: 'open' });
  const [searchQuery, setSearchQuery] = useState('');
  const [stateFilter, setStateFilter] = useState<PullRequestState | 'all'>('open');
  const [providerFilter, setProviderFilter] = useState('');
  const [repositoryFilterName, setRepositoryFilterName] = useState<string | null>(null);

  useEffect(() => {
    const initialFilters: PullRequestsFilters = {
      page: parseInt(searchParams.get('page') || '1', 10),
      perPage: parseInt(searchParams.get('per_page') || '20', 10),
      repositoryId: searchParams.get('repository_id') || undefined,
      provider: searchParams.get('provider') || undefined,
      state: (searchParams.get('state') as PullRequestState | 'all') || 'open',
      search: searchParams.get('search') || undefined,
    };
    setFilters(initialFilters);
    setSearchQuery(initialFilters.search || '');
    setStateFilter(initialFilters.state || 'open');
    setProviderFilter(initialFilters.provider || '');
  }, [searchParams]);

  const fetchPullRequests = useCallback(async (requestFilters?: PullRequestsFilters) => {
    try {
      setLoading(true);
      setError(null);
      const response = await getPullRequests(requestFilters || filters);
      setPullRequests(response.pull_requests || []);
      setTotal(response.total || 0);
      setTotalPages(response.total_pages || 1);
      if ((requestFilters || filters).repositoryId && response.pull_requests?.length) {
        setRepositoryFilterName(response.pull_requests[0].repository_full_name);
      } else if (!(requestFilters || filters).repositoryId) {
        setRepositoryFilterName(null);
      }
    } catch (err) {
      console.error('Error fetching pull requests:', err);
      setError(err instanceof Error ? err.message : 'Failed to fetch pull/merge requests');
    } finally {
      setLoading(false);
    }
  }, [filters]);

  useEffect(() => {
    fetchPullRequests(filters);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters]);

  const updateFilters = useCallback((newFilters: Partial<PullRequestsFilters>) => {
    const updated = { ...filters, ...newFilters };
    if (!newFilters.page) updated.page = 1;
    setFilters(updated);

    const params = new URLSearchParams();
    if (updated.page && updated.page > 1) params.set('page', updated.page.toString());
    if (updated.perPage && updated.perPage !== 20) params.set('per_page', updated.perPage.toString());
    if (updated.repositoryId) params.set('repository_id', updated.repositoryId);
    if (updated.provider) params.set('provider', updated.provider);
    if (updated.state && updated.state !== 'all') params.set('state', updated.state);
    if (updated.search) params.set('search', updated.search);
    setSearchParams(params);
  }, [filters, setSearchParams]);

  const handleSearch = () => updateFilters({ search: searchQuery || undefined });
  const handleStateFilter = (value: PullRequestState | 'all') => {
    setStateFilter(value);
    updateFilters({ state: value });
  };
  const handleProviderFilter = (value: string) => {
    setProviderFilter(value);
    updateFilters({ provider: value || undefined });
  };
  const clearRepositoryFilter = () => {
    setRepositoryFilterName(null);
    updateFilters({ repositoryId: undefined });
  };
  const clearFilters = () => {
    setSearchQuery('');
    setStateFilter('open');
    setProviderFilter('');
    setRepositoryFilterName(null);
    setSearchParams(new URLSearchParams());
    setFilters({ page: 1, perPage: 20, state: 'open' });
  };

  const handleTriggerReview = async (pr: PullRequestWithRepo, e: React.MouseEvent) => {
    e.stopPropagation();
    setTriggeringId(pr.id);
    try {
      await triggerReviewForPullRequest(pr.id);
      toast.success(`Review triggered for #${pr.number}`);
    } catch (err) {
      console.error('Failed to trigger review:', err);
      toast.error(err instanceof Error ? err.message : 'Failed to trigger review');
    } finally {
      setTriggeringId(null);
    }
  };

  const columns = useMemo<ColumnDef<PullRequestWithRepo>[]>(() => [
    {
      id: 'title',
      header: 'Merge Request',
      cell: ({ row }) => {
        const pr = row.original;
        return (
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-2">
              <span className="text-slate-400"><ProviderIcon provider={pr.provider} /></span>
              <span className="text-white font-semibold">{pr.title || `#${pr.number}`}</span>
              <a
                href={pr.web_url}
                target="_blank"
                rel="noopener noreferrer"
                onClick={(e) => e.stopPropagation()}
                className="text-blue-400 hover:text-blue-300 text-xs font-medium underline underline-offset-2"
              >
                #{pr.number}
              </a>
            </div>
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-slate-400">
              <span>{pr.repository_full_name}</span>
              {pr.source_branch && pr.target_branch && (
                <span className="font-mono">{pr.source_branch} → {pr.target_branch}</span>
              )}
            </div>
          </div>
        );
      },
    },
    {
      id: 'state',
      header: 'State',
      cell: ({ row }) => (
        <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium text-white ${getPRStateColor(row.original.state)}`}>
          {getPRStateText(row.original.state)}
        </span>
      ),
    },
    {
      id: 'author',
      header: 'Author',
      cell: ({ row }) => {
        const pr = row.original;
        return <span className="text-slate-300">{pr.author_name || pr.author_username || '—'}</span>;
      },
    },
    {
      id: 'updated',
      header: 'Updated',
      cell: ({ row }) => (
        <span className="text-slate-400 text-sm">
          {formatRelativeTime(row.original.provider_updated_at || row.original.last_synced_at)}
        </span>
      ),
    },
    {
      id: 'actions',
      header: 'Actions',
      cell: ({ row }) => {
        const pr = row.original;
        return (
          <Button
            variant="outline"
            size="sm"
            onClick={(e) => handleTriggerReview(pr, e)}
            disabled={triggeringId === pr.id}
            className="border-slate-600 text-slate-300 hover:text-white hover:border-slate-500"
          >
            {triggeringId === pr.id ? 'Triggering…' : 'Trigger Review'}
          </Button>
        );
      },
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [triggeringId]);

  return (
    <div className="container mx-auto px-4 py-8">
      <div className="mb-2">
        <h1 className="text-3xl font-bold text-white mb-2">Explore</h1>
        <p className="text-slate-300">Browse merge/pull requests across every connected repository</p>
      </div>

      <ExploreTabs active="merge-requests" />

      <div className="bg-slate-800 rounded-lg p-6 mb-6 border border-slate-700">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <div className="md:col-span-2">
            <div className="flex">
              <input
                type="text"
                placeholder="Search title, author, or repository..."
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
              value={stateFilter}
              onChange={(e) => handleStateFilter(e.target.value as PullRequestState | 'all')}
              className="w-full px-3 py-2 bg-slate-700 border border-slate-600 rounded-md text-white focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            >
              <option value="all">All States</option>
              <option value="open">Open</option>
              <option value="closed">Closed</option>
              <option value="merged">Merged</option>
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

        {(searchQuery || stateFilter !== 'open' || providerFilter || repositoryFilterName) && (
          <div className="flex items-center justify-between mt-4 pt-4 border-t border-slate-700">
            <div className="flex items-center space-x-2 text-sm text-slate-300">
              <span>Active filters:</span>
              {searchQuery && <span className="bg-blue-600 px-2 py-1 rounded text-white">Search: "{searchQuery}"</span>}
              {stateFilter !== 'open' && <span className="bg-blue-600 px-2 py-1 rounded text-white">State: {stateFilter === 'all' ? 'All' : getPRStateText(stateFilter)}</span>}
              {providerFilter && <span className="bg-blue-600 px-2 py-1 rounded text-white">Provider: {providerLabel(providerFilter)}</span>}
              {repositoryFilterName && (
                <span className="bg-blue-600 px-2 py-1 rounded text-white flex items-center gap-1">
                  Repository: {repositoryFilterName}
                  <button onClick={clearRepositoryFilter} className="ml-1 hover:text-slate-200" aria-label="Clear repository filter">×</button>
                </span>
              )}
            </div>
            <Button onClick={clearFilters} variant="ghost" className="text-slate-400 hover:text-white">
              Clear all
            </Button>
          </div>
        )}
      </div>

      <DataTable
        data={pullRequests}
        columns={columns}
        getRowId={(pr) => pr.id}
        loading={loading}
        loadingLabel="Loading merge requests..."
        error={error}
        onRetry={() => fetchPullRequests()}
        empty={{
          title: 'No merge requests found',
          description: 'Try a different filter, or sync a repository from the Repositories tab.',
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

export default MergeRequests;
