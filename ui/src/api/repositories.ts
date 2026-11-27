import apiClient from './apiClient';
import { Repository, RepositoriesFilters, RepositoriesListResponse } from '../types/explore';

/**
 * Get a paginated list of repositories across all connectors, with optional
 * filters for connector, provider, and search.
 */
export const getRepositories = async (filters: RepositoriesFilters = {}): Promise<RepositoriesListResponse> => {
  const params = new URLSearchParams();
  if (filters.connectorId) params.append('connector_id', filters.connectorId);
  if (filters.provider) params.append('provider', filters.provider);
  if (filters.search) params.append('search', filters.search);
  if (filters.syncStatus) params.append('sync_status', filters.syncStatus);
  if (filters.hasOpenPRs) params.append('has_open_prs', filters.hasOpenPRs);
  if (filters.sort) params.append('sort', filters.sort);
  if (filters.order) params.append('order', filters.order);
  if (filters.page) params.append('page', filters.page.toString());
  if (filters.perPage) params.append('per_page', filters.perPage.toString());

  const queryString = params.toString();
  const endpoint = `/api/v1/repositories${queryString ? `?${queryString}` : ''}`;
  return apiClient.get<RepositoriesListResponse>(endpoint);
};

/**
 * Get a single repository by ID.
 */
export const getRepository = async (repoId: number): Promise<Repository> => {
  return apiClient.get<Repository>(`/api/v1/repositories/${repoId}`);
};

/**
 * Trigger an immediate PR/MR sync for a single repository.
 */
export const syncRepositoryPullRequests = async (repoId: number): Promise<{ status: string; repository_id: number }> => {
  return apiClient.post<{ status: string; repository_id: number }>(`/api/v1/repositories/${repoId}/sync`, {});
};

/**
 * Re-run repository discovery for a whole connector, upserting any new
 * repositories and queueing a PR/MR backfill sync for each.
 */
export const syncConnectorRepositories = async (connectorId: string): Promise<{ status: string; connector_id: number }> => {
  return apiClient.post<{ status: string; connector_id: number }>(`/api/v1/connectors/${connectorId}/repositories/sync`, {});
};

export interface RepositoryCache {
  enabled: boolean;
  cached: boolean;
  bytes: number;
  last_used?: string;
}

/** GET /api/v1/repositories/:repoId/cache - this repo's server-side repo cache entry. */
export const getRepositoryCache = async (repoId: number): Promise<RepositoryCache> => {
  return apiClient.get<RepositoryCache>(`/api/v1/repositories/${repoId}/cache`);
};

/** DELETE /api/v1/repositories/:repoId/cache - clears it (owners/super admins; 409 while a review uses it). */
export const clearRepositoryCache = async (repoId: number): Promise<{ message: string }> => {
  return apiClient.delete<{ message: string }>(`/api/v1/repositories/${repoId}/cache`);
};

export interface RelatedRepository {
  relation_id: number;
  repository_id: number;
  provider: string;
  full_name: string;
  web_url: string;
  created_at: string;
}

/** GET /api/v1/repositories/:repoId/related - repos linked for cross-repo blast-radius context. */
export const getRelatedRepositories = async (repoId: number): Promise<RelatedRepository[]> => {
  return apiClient.get<RelatedRepository[]>(`/api/v1/repositories/${repoId}/related`);
};

/** POST /api/v1/repositories/:repoId/related - links relatedRepoId as related (owners/super admins). */
export const addRelatedRepository = async (repoId: number, relatedRepoId: number): Promise<{ message: string }> => {
  return apiClient.post<{ message: string }>(`/api/v1/repositories/${repoId}/related`, { related_repository_id: relatedRepoId });
};

/** DELETE /api/v1/repositories/:repoId/related/:relationId - removes the link (owners/super admins). */
export const removeRelatedRepository = async (repoId: number, relationId: number): Promise<{ message: string }> => {
  return apiClient.delete<{ message: string }>(`/api/v1/repositories/${repoId}/related/${relationId}`);
};

export type RepoToolMode = 'inherit' | 'custom';

export interface RepoToolView {
  id: number;
  name: string;
  description: string;
  lambda_arn: string;
  multiplier: number;
  use_case: string;
  category?: string;
  docs_url?: string;
  enabled: boolean;
  org_enabled: boolean;
  repo_enabled?: boolean;
  config_json?: {
    include?: string[];
    exclude?: string[];
  };
}

export interface RepoToolRuleConfig {
  include?: string[];
  exclude?: string[];
}

export interface RepositoryToolsResponse {
  repository_id: number;
  mode: RepoToolMode;
  tools: RepoToolView[];
}

/** GET /api/v1/repositories/:repoId/tools - repository PR-only tool settings merged with org defaults. */
export const getRepositoryTools = async (repoId: number): Promise<RepositoryToolsResponse> => {
  return apiClient.get<RepositoryToolsResponse>(`/api/v1/repositories/${repoId}/tools`);
};

/** PUT /api/v1/repositories/:repoId/tools - saves PR-only tool mode ('inherit' or 'custom'), tool overrides, and optional include/exclude globs. */
export const updateRepositoryTools = async (
  repoId: number,
  payload: {
    mode: RepoToolMode;
    tools?: Record<string, boolean>;
    tool_configs?: Record<string, RepoToolRuleConfig>;
  },
): Promise<RepositoryToolsResponse> => {
  return apiClient.put<RepositoryToolsResponse>(`/api/v1/repositories/${repoId}/tools`, payload);
};
