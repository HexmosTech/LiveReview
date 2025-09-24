// TypeScript interfaces for Reviews API, matching backend Go structs

export interface Review {
  id: number;
  repository: string;
  branch?: string;
  commitHash?: string;
  prMrUrl?: string;
  connectorId?: number;
  status: ReviewStatus;
  triggerType: string;
  userEmail?: string;
  provider?: string;
  createdAt: string;
  startedAt?: string;
  completedAt?: string;
  metadata?: Record<string, any>;
  orgId: number;
}

export type ReviewStatus = 'created' | 'in_progress' | 'completed' | 'failed';

export interface ReviewsListResponse {
  reviews: Review[];
  total: number;
  page: number;
  perPage: number;
  totalPages: number;
  hasNext: boolean;
  hasPrevious: boolean;
}

export interface ReviewEvent {
  id: number;
  reviewId: number;
  orgId: number;
  time: string;
  type: ReviewEventType;
  level?: ReviewEventLevel;
  batchId?: string;
  data: ReviewEventData;
}

export type ReviewEventType = 'status' | 'log' | 'batch' | 'artifact' | 'completion';
export type ReviewEventLevel = 'info' | 'warn' | 'error' | 'debug';

export interface ReviewEventData {
  // For "status" events
  status?: string;
  startedAt?: string;
  finishedAt?: string;
  durationMs?: number;

  // For "log" events
  message?: string;

  // For "batch" events
  tokenEstimate?: number;
  fileCount?: number;

  // For "artifact" events
  kind?: string;
  sizeBytes?: number;
  previewHead?: string;
  previewTail?: string;
  url?: string;

  // For "completion" events
  resultSummary?: string;
  commentCount?: number;
  errorSummary?: string;
}

export interface ReviewEventsResponse {
  events: ReviewEvent[];
  meta: {
    reviewId: number;
    count: number;
    limit: number;
    since?: string;
    eventType?: string;
  };
}

export interface ReviewSummary {
  reviewId: number;
  currentStatus: string;
  lastActivity: string;
  eventCounts: Record<string, number>;
  batchCount: number;
}

export interface ReviewsFilters {
  status?: ReviewStatus;
  provider?: string;
  search?: string;
  page?: number;
  perPage?: number;
}

export interface CreateReviewRequest {
  url: string;
}

export interface CreateReviewResponse {
  message: string;
  url: string;
  reviewId: string;
}

// API Error interface
export interface APIError {
  error: string;
  message?: string;
  status?: number;
}