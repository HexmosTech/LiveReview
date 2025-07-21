import apiClient from './apiClient';

export interface TriggerReviewRequest {
  url: string;
}

export interface TriggerReviewResponse {
  message: string;
  url: string;
  reviewId: string;
}

/**
 * Trigger a new code review from a URL
 * @param request The request with URL to trigger review for
 * @returns Promise with review trigger response
 */
export const triggerReview = async (request: TriggerReviewRequest): Promise<TriggerReviewResponse> => {
  try {
    return await apiClient.post<TriggerReviewResponse>('/api/v1/trigger-review', request);
  } catch (error) {
    console.error('Error triggering review:', error);
    throw error;
  }
};
