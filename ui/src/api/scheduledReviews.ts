import apiClient from './apiClient';
import { ScheduledReviewConfig, SetScheduledReviewRequest } from '../types/reviews';

/**
 * List scheduled-review configs for every repo under a connector.
 */
export const getScheduledReviewConfigs = async (connectorId: number): Promise<ScheduledReviewConfig[]> => {
  return apiClient.get<ScheduledReviewConfig[]>(`/api/v1/connectors/${connectorId}/scheduled-reviews`);
};

/**
 * Enable/disable scheduled review for a single repo under a connector. The backend currently
 * ignores everything but {project_path, enabled} — it always applies a fixed 24h interval.
 */
export const setScheduledReview = async (
  connectorId: number,
  request: SetScheduledReviewRequest
): Promise<ScheduledReviewConfig> => {
  return apiClient.put<ScheduledReviewConfig>(`/api/v1/connectors/${connectorId}/scheduled-reviews`, request);
};
