import apiClient from './apiClient';
import { ScheduledReviewConfig, SetScheduledReviewRequest } from '../types/reviews';

/**
 * List scheduled-review configs for every repo under a connector.
 */
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
