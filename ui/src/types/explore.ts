// TypeScript interfaces for the unified Explore (Repositories / Merge
// Requests) API, matching the backend Go structs in
// internal/api/repositories_pr_routes.go. Field names are snake_case to
// mirror the JSON these endpoints emit directly (same convention as
// connectors.ts's ConnectorResponse).

export interface Repository {
  id: number;
  org_id: number;
  connector_id: number;
  provider: string;
  provider_repo_id: string;
  full_name: string;
  name: string;
  web_url: string;
  default_branch?: string;
  is_private: boolean;
  description?: string;
  last_synced_at?: string;
  last_sync_status: 'pending' | 'ok' | 'error';
  last_sync_error?: string;
  open_pr_count: number;
  /** Most recent PR/MR provider_updated_at across this repo's pull requests. */
  last_activity_at?: string;
  created_at: string;
  updated_at: string;
}

export type RepositoriesSort = 'last_activity' | 'open_pr_count' | 'repository' | 'provider' | 'default_branch' | 'sync_status' | 'schedule';

export interface RepositoriesListResponse {
  repositories: Repository[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export interface RepositoriesFilters {
  /** Comma-separated list of connector IDs (multi-select). */
  connectorId?: string;
  /** Comma-separated list of providers (multi-select). */
  provider?: string;
  search?: string;
  /** Comma-separated list of 'pending' | 'ok' | 'error' (multi-select). */
  syncStatus?: string;
  hasOpenPRs?: 'true' | 'false';
  /** Defaults to 'last_activity' server-side when omitted. */
  sort?: RepositoriesSort;
  /** Defaults server-side to 'desc' for open_pr_count/last_activity, 'asc' for every other sort key. */
  order?: 'asc' | 'desc';
  page?: number;
  perPage?: number;
}

export type PullRequestState = 'open' | 'closed' | 'merged';

export interface PullRequest {
  id: number;
  repository_id: number;
  org_id: number;
  provider: string;
  provider_pr_id: string;
  number: number;
  title: string;
  description?: string;
  state: PullRequestState;
  author_username?: string;
  author_name?: string;
  author_avatar_url?: string;
  source_branch?: string;
  target_branch?: string;
  web_url: string;
  provider_created_at?: string;
  provider_updated_at?: string;
  last_synced_at: string;
  last_synced_source: 'webhook' | 'poll' | 'backfill';
  created_at: string;
  updated_at: string;
}

// A pull request as returned by the unified, cross-repository listing -
// includes just enough of the owning repository to render without a
// follow-up lookup.
export interface PullRequestWithRepo extends PullRequest {
  repository_full_name: string;
  repository_web_url: string;
}

export interface PullRequestsListResponse {
  pull_requests: PullRequest[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export interface PullRequestsWithRepoListResponse {
  pull_requests: PullRequestWithRepo[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export interface PullRequestReviewSummary {
  id: number;
  status: string;
  trigger_type: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
}

export interface PullRequestDetail extends PullRequest {
  reviews: PullRequestReviewSummary[];
}

export interface PullRequestsFilters {
  repositoryId?: string;
  connectorId?: string;
  /** Comma-separated list of providers (multi-select). */
  provider?: string;
  /** Comma-separated list of 'open' | 'closed' | 'merged', or 'all' (multi-select). */
  state?: string;
  search?: string;
  page?: number;
  perPage?: number;
}

export interface CreateReviewFromPullRequestResponse {
  message: string;
  url: string;
  reviewId: string;
  pull_request_id: number;
}
