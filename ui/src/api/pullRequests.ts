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
 * Resolve which org a PR belongs to (404 if missing or the user isn't a member).
 * Used by the PR comment's "Start a review" link before the org is known. pull_request_id
 * is the id to use in that org (another org's copy of the same PR when orgs share a
 * GitHub App installation).
 */
export interface PullRequestOrg {
  org_id: number;
  /** The PR's id in that org. Missing when `connect` is set. */
  pull_request_id?: number;
  /**
   * Set when none of the user's orgs has this PR yet but org_id has its GitHub installation available, not connected:
   * connecting it there brings the PR in.
   */
  connect?: { connector_id: number; account_login: string; can_connect: boolean };
}

export const getPullRequestOrg = async (pullRequestId: number): Promise<PullRequestOrg> => {
  return apiClient.get<PullRequestOrg>(`/api/v1/pull-requests/${pullRequestId}/org`);
};

/**
 * Start a review from the PR comment link: reuses one still running from the last
 * 10 minutes, and runs the approval policy check when it finishes.
 */
export const startOrReuseReviewForPullRequest = async (pullRequestId: number): Promise<CreateReviewFromPullRequestResponse> => {
  return apiClient.post<CreateReviewFromPullRequestResponse>('/api/v1/reviews', {
    pull_request_id: pullRequestId,
    from_pr_link: true,
  });
};

/**
 * Human-friendly label for a PR/MR state.
 */
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
