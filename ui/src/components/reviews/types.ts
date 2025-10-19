export type ReviewEventType =
  | 'log'
  | 'status'
  | 'batch'
  | 'artifact'
  | 'completion'
  | 'retry'
  | 'json_repair'
  | 'timeout'
  | 'started'
  | 'progress'
  | 'batch_complete'
  | 'error'
  | 'completed';

export type ReviewEventSeverity = 'info' | 'success' | 'warning' | 'warn' | 'error' | 'debug';

export interface ReviewEventDetails {
  batchId?: string;
  status?: string;
  fileCount?: number;
  tokenEstimate?: number;
  message?: string;
  attempt?: number;
  delay?: string;
  filename?: string;
  responseTime?: string;
  errorMessage?: string;
  resultSummary?: string;
  commentCount?: number;
  repairStats?: {
    originalSize?: number;
    repairedSize?: number;
    commentsLost?: number;
    fieldsRecovered?: number;
    repairTime?: string;
    repairStrategies?: string[];
  };
  [key: string]: unknown;
}

export interface ReviewEvent {
  id: string;
  timestamp: string;
  eventType: ReviewEventType;
  message: string;
  details?: ReviewEventDetails;
  severity: ReviewEventSeverity;
}
