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
