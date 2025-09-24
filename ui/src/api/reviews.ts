import apiClient from './apiClient';
import { 
  Review, 
  ReviewsListResponse, 
  ReviewsFilters, 
  ReviewEventsResponse, 
  ReviewSummary 
} from '../types/reviews';

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
    return await apiClient.post<TriggerReviewResponse>('/api/v1/connectors/trigger-review', request);
  } catch (error) {
    console.error('Error triggering review:', error);
    throw error;
  }
};

/**
 * Get a paginated list of reviews with optional filters
 * @param filters Optional filters for status, provider, search, pagination
 * @returns Promise with reviews list response
 */
export const getReviews = async (filters: ReviewsFilters = {}): Promise<ReviewsListResponse> => {
  try {
    const params = new URLSearchParams();
    
    if (filters.status) params.append('status', filters.status);
    if (filters.provider) params.append('provider', filters.provider);
    if (filters.search) params.append('search', filters.search);
    if (filters.page) params.append('page', filters.page.toString());
    if (filters.perPage) params.append('per_page', filters.perPage.toString());

    const queryString = params.toString();
    const endpoint = `/api/v1/reviews${queryString ? `?${queryString}` : ''}`;
    
    return await apiClient.get<ReviewsListResponse>(endpoint);
  } catch (error) {
    console.error('Error fetching reviews:', error);
    throw error;
  }
};

/**
 * Get a single review by ID
 * @param reviewId The ID of the review to fetch
 * @returns Promise with review data
 */
export const getReview = async (reviewId: number): Promise<Review> => {
  try {
    return await apiClient.get<Review>(`/api/v1/reviews/${reviewId}`);
  } catch (error) {
    console.error('Error fetching review:', error);
    throw error;
  }
};

/**
 * Get events for a review with optional pagination
 * @param reviewId The ID of the review
 * @param since Optional timestamp to get events since
 * @param limit Optional limit for number of events
 * @returns Promise with review events response
 */
export const getReviewEvents = async (
  reviewId: number, 
  since?: string, 
  limit?: number
): Promise<ReviewEventsResponse> => {
  try {
    const params = new URLSearchParams();
    if (since) params.append('since', since);
    if (limit) params.append('limit', limit.toString());

    const queryString = params.toString();
    const endpoint = `/api/v1/reviews/${reviewId}/events${queryString ? `?${queryString}` : ''}`;
    
    return await apiClient.get<ReviewEventsResponse>(endpoint);
  } catch (error) {
    console.error('Error fetching review events:', error);
    throw error;
  }
};

/**
 * Get events of a specific type for a review
 * @param reviewId The ID of the review
 * @param eventType The type of events to fetch
 * @param limit Optional limit for number of events
 * @returns Promise with review events response
 */
export const getReviewEventsByType = async (
  reviewId: number, 
  eventType: string, 
  limit?: number
): Promise<ReviewEventsResponse> => {
  try {
    const params = new URLSearchParams();
    if (limit) params.append('limit', limit.toString());

    const queryString = params.toString();
    const endpoint = `/api/v1/reviews/${reviewId}/events/${eventType}${queryString ? `?${queryString}` : ''}`;
    
    return await apiClient.get<ReviewEventsResponse>(endpoint);
  } catch (error) {
    console.error('Error fetching review events by type:', error);
    throw error;
  }
};

/**
 * Get a summary of review progress and statistics
 * @param reviewId The ID of the review
 * @returns Promise with review summary
 */
export const getReviewSummary = async (reviewId: number): Promise<ReviewSummary> => {
  try {
    return await apiClient.get<ReviewSummary>(`/api/v1/reviews/${reviewId}/summary`);
  } catch (error) {
    console.error('Error fetching review summary:', error);
    throw error;
  }
};

// Utility functions for UI components

/**
 * Format relative time from timestamp
 */
export function formatRelativeTime(timestamp: string): string {
  const now = new Date();
  const time = new Date(timestamp);
  const diffMs = now.getTime() - time.getTime();
  
  if (diffMs < 0) return 'In the future'; // Handle edge case
  
  const diffSeconds = Math.floor(diffMs / 1000);
  const diffMinutes = Math.floor(diffSeconds / 60);
  const diffHours = Math.floor(diffMinutes / 60);
  const diffDays = Math.floor(diffHours / 24);
  const diffWeeks = Math.floor(diffDays / 7);
  const diffMonths = Math.floor(diffDays / 30);
  const diffYears = Math.floor(diffDays / 365);

  if (diffSeconds < 30) return 'Just now';
  if (diffSeconds < 60) return `${diffSeconds}s ago`;
  if (diffMinutes < 60) return `${diffMinutes}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  if (diffWeeks < 4) return `${diffWeeks}w ago`;
  if (diffMonths < 12) return `${diffMonths}mo ago`;
  return `${diffYears}y ago`;
}

/**
 * Get color class for review status badges
 */
export function getStatusColor(status: string): string {
  switch (status) {
    case 'created':
    case 'pending':
      return 'bg-yellow-500';
    case 'in_progress':
      return 'bg-blue-500';
    case 'completed':
      return 'bg-green-500';
    case 'failed':
      return 'bg-red-500';
    default:
      return 'bg-gray-500';
  }
}

/**
 * Get formatted text for review status
 */
export function getStatusText(status: string): string {
  switch (status) {
    case 'created':
      return 'Created';
    case 'in_progress':
      return 'In Progress';
    case 'completed':
      return 'Completed';
    case 'failed':
      return 'Failed';
    default:
      return status.charAt(0).toUpperCase() + status.slice(1);
  }
}
