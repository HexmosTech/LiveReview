import apiClient from './apiClient';
import {
  PullRequestDetail,
  PullRequestsFilters,
  PullRequestsListResponse,
  PullRequestsWithRepoListResponse,
  CreateReviewFromPullRequestResponse,
} from '../types/explore';

/**
 * Get a paginated, unified list of pull/merge requests across every
 * repository the org's connectors have discovered (GET /api/v1/pull-requests).
 */
export const getPullRequests = async (filters: PullRequestsFilters = {}): Promise<PullRequestsWithRepoListResponse> => {
  const params = new URLSearchParams();
  if (filters.repositoryId) params.append('repository_id', filters.repositoryId);
  if (filters.connectorId) params.append('connector_id', filters.connectorId);
  if (filters.provider) params.append('provider', filters.provider);
  if (filters.state && filters.state !== 'all') params.append('state', filters.state);
  if (filters.search) params.append('search', filters.search);
  if (filters.page) params.append('page', filters.page.toString());
  if (filters.perPage) params.append('per_page', filters.perPage.toString());

  const queryString = params.toString();
  const endpoint = `/api/v1/pull-requests${queryString ? `?${queryString}` : ''}`;
  return apiClient.get<PullRequestsWithRepoListResponse>(endpoint);
};

/**
 * Get pull/merge requests for one specific repository.
 */
export const getPullRequestsForRepo = async (
  repoId: number,
  filters: PullRequestsFilters = {}
): Promise<PullRequestsListResponse> => {
  const params = new URLSearchParams();
  if (filters.state && filters.state !== 'all') params.append('state', filters.state);
  if (filters.search) params.append('search', filters.search);
  if (filters.page) params.append('page', filters.page.toString());
  if (filters.perPage) params.append('per_page', filters.perPage.toString());

  const queryString = params.toString();
  const endpoint = `/api/v1/repositories/${repoId}/pull-requests${queryString ? `?${queryString}` : ''}`;
  return apiClient.get<PullRequestsListResponse>(endpoint);
};

/**
 * Get a single pull/merge request, including its LiveReview review history.
 */
export const getPullRequest = async (repoId: number, prId: number): Promise<PullRequestDetail> => {
  return apiClient.get<PullRequestDetail>(`/api/v1/repositories/${repoId}/pull-requests/${prId}`);
};

/**
 * Trigger a new review against a known pull_requests row.
 */
export const triggerReviewForPullRequest = async (pullRequestId: number): Promise<CreateReviewFromPullRequestResponse> => {
  return apiClient.post<CreateReviewFromPullRequestResponse>('/api/v1/reviews', { pull_request_id: pullRequestId });
};

/**
 * Human-friendly label + badge color for a PR/MR state.
 */
export function getPRStateColor(state: string): string {
  switch (state) {
    case 'open':
      return 'bg-green-500';
    case 'merged':
      return 'bg-purple-500';
    case 'closed':
      return 'bg-slate-500';
    default:
      return 'bg-gray-500';
  }
}

export function getPRStateText(state: string): string {
  switch (state) {
    case 'open':
      return 'Open';
    case 'merged':
      return 'Merged';
    case 'closed':
      return 'Closed';
    default:
      return state.charAt(0).toUpperCase() + state.slice(1);
  }
}
