import apiClient from './apiClient';

export type FeedbackVoteType = 'up' | 'down';
export type FeedbackSourceType = 'comment' | 'pr_level' | 'slideshow' | 'general';

export interface SubmitFeedbackRequest {
  review_id?: number;
  ai_comment_id?: number;
  vote_type: FeedbackVoteType;
  source_type: FeedbackSourceType;
  tags?: string[];
  feedback_text?: string;
  comment_content?: string;
  code_excerpt?: string;
  file_path?: string;
  severity?: string;
}

export interface SubmitFeedbackResponse {
  id: number;
  created_at: string;
}

export interface ImpactStat {
  label: string;
  value: number;
  tooltip: string;
}

export interface ImpactStatsResponse {
  total_reviews: number;
  issues_found: number;
  bugs_caught: number;
  critical: number;
  errors: number;
  warnings: number;
  info: number;
}

/**
 * Submits an up/down vote (see internal/api/feedback_handler.go's
 * SubmitFeedback). Ownership is enforced server-side: only the review's
 * original creator (matched by email) can vote on it.
 */
export const submitFeedback = async (request: SubmitFeedbackRequest): Promise<SubmitFeedbackResponse> => {
  return apiClient.post<SubmitFeedbackResponse>('/api/v1/feedback', request);
};

/** Retracts a previously-submitted vote, used when switching up<->down. */
export const retractFeedback = async (feedbackId: number): Promise<void> => {
  await apiClient.patch<void>(`/api/v1/feedback/${feedbackId}/retract`, {});
};

let _cachedStats: ImpactStat[] | null = null;
let _statsFetching = false;
const _statsCallbacks: ((stats: ImpactStat[]) => void)[] = [];

export function getImpactStats(onReady: (stats: ImpactStat[]) => void): void {
  if (_cachedStats) { onReady(_cachedStats); return; }
  _statsCallbacks.push(onReady);
  if (_statsFetching) return;
  _statsFetching = true;
  apiClient.get<ImpactStatsResponse>('/api/v1/feedback/impact-stats')
    .then((data) => {
      _cachedStats = [
        { label: 'Total Reviews', value: data.total_reviews, tooltip: 'Total completed reviews' },
        { label: 'Issues Found', value: data.issues_found, tooltip: 'Sum of all severity issues' },
        { label: 'Bugs Caught Pre-Prod', value: data.bugs_caught, tooltip: 'Critical + Error issues' },
        { label: 'Critical', value: data.critical, tooltip: 'Critical severity issues' },
        { label: 'Errors', value: data.errors, tooltip: 'Error severity issues' },
        { label: 'Warnings', value: data.warnings, tooltip: 'Warning severity issues' },
        { label: 'Info', value: data.info, tooltip: 'Info severity comments' },
      ];
      _statsCallbacks.splice(0).forEach((cb) => cb(_cachedStats!));
    })
    .catch(() => { _statsFetching = false; _statsCallbacks.length = 0; });
}
