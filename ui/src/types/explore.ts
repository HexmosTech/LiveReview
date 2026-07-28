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
  created_at: string;
  updated_at: string;
}

export interface RepositoriesListResponse {
  repositories: Repository[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export interface RepositoriesFilters {
  connectorId?: string;
  provider?: string;
  search?: string;
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
  provider?: string;
  state?: PullRequestState | 'all';
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
