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
  mrTitle?: string;
  friendlyName?: string;
  aiSummaryTitle?: string;
  authorName?: string;
  authorUsername?: string;
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

export interface ReviewAccountingOperation {
  operationType: string;
  triggerSource: string;
  operationId: string;
  idempotencyKey: string;
  billableLoc: number;
  accountedAt: string;
  provider?: string;
  model?: string;
  pricingVersion?: string;
  inputTokens?: number;
  outputTokens?: number;
  costUsd?: number;
  metadata?: string;
}

export interface ReviewAccounting {
  reviewId: number;
  totalBillableLoc: number;
  accountedOperations: number;
  tokenTrackedOperations: number;
  lastAccountedAt?: string;
  totalInputTokens?: number;
  totalOutputTokens?: number;
  totalCostUsd?: number;
  latestOperation?: ReviewAccountingOperation;
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
  ai_execution_mode?: string;
  ai_execution_source?: string;
}

// API Error interface
export interface APIError {
  error: string;
  message?: string;
  status?: number;
}