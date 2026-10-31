import apiClient from './apiClient';

// ---- types mirroring internal/api/* and internal/policyengine ---------------

export interface AutoApprovePolicy {
  id: number;
  org_id: number;
  repository_id?: number;
  mode: 'inherit' | 'custom' | 'always_approve' | 'never_approve';
  min_author_level: number;
  eligibility_whitelist_user_ids: number[];
  risk_threshold_percentile: number;
  require_llm_confidence: number;
  blocking_ruleset_ids: number[];
  landing_delay_seconds: number;
  daily_approve_cap?: number;
  restrict_outcome_reporting_to_admins: boolean;
  force_approve_min_level: number;
}

export interface AutoApprovePolicyUpsert {
  repository_id?: number;
  mode?: string;
  min_author_level?: number;
  eligibility_whitelist_user_ids?: number[];
  risk_threshold_percentile?: number;
  require_llm_confidence?: number;
  blocking_ruleset_ids?: number[];
  landing_delay_seconds?: number;
  daily_approve_cap?: number;
  restrict_outcome_reporting_to_admins?: boolean;
  force_approve_min_level?: number;
}

export interface PolicyLogStage {
  stage: string;
  verdict: string;
  detail: Record<string, unknown>;
}

export interface PolicyLogResponse {
  stages: PolicyLogStage[];
  blocked: boolean;
  force_approve_min_level: number;
  features: number[];
  feature_names: { Name: string; Category: string }[];
  model_version: number;
  score: number;
}

export interface RiskModelInfo {
  version: number;
  training_samples: number;
  is_seed: boolean;
}

export interface RiskModelResponse {
  model: RiskModelInfo;
  overrides: Record<string, number>;
  scores: number[];
}

export interface OutcomeRow {
  review_id: number;
  outcome_type: string;
  source: string;
  reason: string;
  created_at: string;
}

export interface UserIdentity {
  id: number;
  org_id: number;
  user_id: number;
  kind: 'email' | 'username';
  value: string;
}

export interface SimulateRow {
  review_id: number;
  would_approve: boolean;
  blocked_at?: string;
  score?: number;
}

export interface SimulateSummary {
  scored_count: number;
  skipped_count: number;
  saturated_high: number;
  saturated_low: number;
  score_min?: number;
  score_max?: number;
  score_mean?: number;
}

export interface ScoreHistogramBucket {
  range_start: number;
  range_end: number;
  count: number;
}

export interface ScoreDistribution {
  buckets: ScoreHistogramBucket[];
  cutoff: number;
  percentile: number;
  total_scored: number;
}

export interface DecisionTrendPoint {
  day: string;
  approved: number;
  blocked: number;
}

export interface DecisionsSummary {
  approved: number;
  blocked_at_eligibility: number;
  blocked_at_static: number;
  blocked_at_risk: number;
  blocked_at_llm: number;
  overridden: number;
  total_decided: number;
  override_rate: number;
  trend: DecisionTrendPoint[];
}

export interface CalibrationCell {
  decision: 'approved' | 'blocked';
  outcome_type: 'revert' | 'incident' | 'confirmed_safe';
  count: number;
}

export interface Calibration {
  cells: CalibrationCell[];
  total_outcomes: number;
  false_approve_rate?: number;
  low_volume_warning: boolean;
}

export interface AutoApproveInsights {
  window_days: number;
  score_distribution: ScoreDistribution;
  decisions: DecisionsSummary;
  calibration: Calibration;
}

// ---- auto-approve policy CRUD ----------------------------------------------

export const listAutoApprovePolicies = (): Promise<{ rows: AutoApprovePolicy[] }> =>
  apiClient.get('/auto-approve-policies');

export const createAutoApprovePolicy = (body: AutoApprovePolicyUpsert): Promise<{ policy: AutoApprovePolicy }> =>
  apiClient.post('/auto-approve-policies', body);

export const updateAutoApprovePolicy = (id: number, body: AutoApprovePolicyUpsert): Promise<{ policy: AutoApprovePolicy }> =>
  apiClient.put(`/auto-approve-policies/${id}`, body);

export const deleteAutoApprovePolicy = (id: number): Promise<{ status: string }> =>
  apiClient.delete(`/auto-approve-policies/${id}`);

export const getResolvedPolicy = (repositoryId?: number): Promise<{ policy: AutoApprovePolicy | null; configured: boolean }> =>
  apiClient.get(`/auto-approve-policies/resolved${repositoryId ? `?repository_id=${repositoryId}` : ''}`);

export const simulatePolicy = (body: {
  repository_id?: number;
  policy: AutoApprovePolicyUpsert;
  review_ids?: number[];
  limit?: number;
}): Promise<{ rows: SimulateRow[]; summary: SimulateSummary }> => apiClient.post('/auto-approve-policies/simulate', body);

// ---- review policy actions -------------------------------------------------

export const getReviewPolicyLog = (reviewId: number): Promise<PolicyLogResponse> =>
  apiClient.get(`/reviews/${reviewId}/policy-log`);

export const forceApproveReview = (reviewId: number, reason: string): Promise<{ status: string }> =>
  apiClient.post(`/reviews/${reviewId}/force-approve`, { reason });

export const markReviewOutcome = (reviewId: number, outcomeType: string, reason: string): Promise<{ status: string }> =>
  apiClient.post(`/reviews/${reviewId}/outcomes`, { outcome_type: outcomeType, reason });

// ---- risk model ------------------------------------------------------------

export const getRiskModel = (): Promise<RiskModelResponse> => apiClient.get('/risk-model');

export const putRiskModelOverrides = (overrides: Record<string, number>): Promise<{ overrides: Record<string, number> }> =>
  apiClient.put('/risk-model/overrides', { overrides });

export const retrainRiskModel = (): Promise<{ result: { promoted: boolean; training_count: number; holdout_auc: number; reason: string } }> =>
  apiClient.post('/risk-model/retrain', {});

// ---- auto-approve insights (Reports -> Auto Approve) -----------------------

export const getAutoApproveInsights = (windowDays: 7 | 30 | 90 = 30): Promise<{ insights: AutoApproveInsights }> =>
  apiClient.get(`/auto-approve-insights?window=${windowDays}`);

// ---- outcomes --------------------------------------------------------------

export const listReviewOutcomes = (orgId: number): Promise<{ rows: OutcomeRow[]; total: number; manual_count: number; webhook_count: number }> =>
  apiClient.get(`/orgs/${orgId}/review-outcomes`);

// ---- identities ------------------------------------------------------------

export const listUserIdentities = (orgId: number, userId: number): Promise<{ rows: UserIdentity[] }> =>
  apiClient.get(`/orgs/${orgId}/users/${userId}/identities`);

export const addUserIdentity = (orgId: number, userId: number, kind: string, value: string): Promise<UserIdentity> =>
  apiClient.post(`/orgs/${orgId}/users/${userId}/identities`, { kind, value });

export const removeUserIdentity = (orgId: number, userId: number, identityId: number): Promise<{ status: string }> =>
  apiClient.delete(`/orgs/${orgId}/users/${userId}/identities/${identityId}`);

export const listOrgIdentities = (orgId: number): Promise<{ rows: UserIdentity[] }> =>
  apiClient.get(`/orgs/${orgId}/identities`);
