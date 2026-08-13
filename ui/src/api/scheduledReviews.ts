import apiClient from './apiClient';
import { ScheduledReviewConfig, SetScheduledReviewRequest, ScheduledReviewRunsListResponse, ScheduledReviewRunsFilters } from '../types/reviews';

/** List scheduled-review configs for every repo under a connector. */
export const getScheduledReviewConfigs = async (connectorId: number): Promise<ScheduledReviewConfig[]> => {
  return apiClient.get<ScheduledReviewConfig[]>(`/api/v1/connectors/${connectorId}/scheduled-reviews`);
};

/** Enable/disable/(re)schedule a repo's scheduled review; cron_expression is UTC (CronBuilder converts local time). */
export const setScheduledReview = async (
  connectorId: number,
  request: SetScheduledReviewRequest
): Promise<ScheduledReviewConfig> => {
  return apiClient.put<ScheduledReviewConfig>(`/api/v1/connectors/${connectorId}/scheduled-reviews`, request);
};

/** Run history for a repo's schedule - one row per scheduler attempt, whether or not it produced a review. */
export const getScheduledReviewRuns = async (
  repositoryId: number,
  filters: ScheduledReviewRunsFilters = {}
): Promise<ScheduledReviewRunsListResponse> => {
  const params = new URLSearchParams();
  if (filters.page) params.append('page', filters.page.toString());
  if (filters.perPage) params.append('per_page', filters.perPage.toString());
  if (filters.outcome) params.append('outcome', filters.outcome);
  if (filters.order) params.append('order', filters.order);

  const queryString = params.toString();
  const endpoint = `/api/v1/repositories/${repositoryId}/scheduled-review-runs${queryString ? `?${queryString}` : ''}`;
  return apiClient.get<ScheduledReviewRunsListResponse>(endpoint);
};
