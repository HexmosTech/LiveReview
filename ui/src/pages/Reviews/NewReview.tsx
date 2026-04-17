import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Card,
  PageHeader,
  Input,
  Button,
  Icons,
  Alert,
  Spinner,
  EmptyState
} from '../../components/UIPrimitives';
import { triggerReview, TriggerReviewRequest } from '../../api/reviews';
import { getConnectors, ConnectorResponse } from '../../api/connectors';
import { ErrorBoundary } from '../../components/ErrorBoundary';
import { UpgradePromptModal } from '../../components/Subscriptions';
import { SafetyBanner } from '../../components/SafetyBanner/SafetyBanner';
import apiClient from '../../api/apiClient';
import { QuotaExhaustedBanner } from '../../components/Dashboard/QuotaExhaustedBanner';
import { QuotaWarningBanner } from '../../components/Dashboard/QuotaWarningBanner';
import { useOrgContext } from '../../hooks/useOrgContext';

type QuotaStatusResponse = {
  can_trigger_reviews: boolean;
  envelope?: {
    blocked?: boolean;
    trial_readonly?: boolean;
    usage_percent?: number;
    threshold_state?: string;
    loc_used_month?: number;
    loc_limit_month?: number;
    loc_remaining_month?: number;
    billing_period_end?: string;
    upgrade_url?: string;
  };
};

const NewReview: React.FC = () => {
  const navigate = useNavigate();
  const { isFreePlan, isSuperAdmin } = useOrgContext();
  const isReadOnly = isFreePlan && !isSuperAdmin;
  const [url, setUrl] = useState('');
  const [connectors, setConnectors] = useState<ConnectorResponse[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadingConnectors, setLoadingConnectors] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [showUpgradeModal, setShowUpgradeModal] = useState(false);
  const [upgradeReason, setUpgradeReason] = useState<'DAILY_LIMIT' | 'NOT_ORG_CREATOR'>('DAILY_LIMIT');
  const [limitInfo, setLimitInfo] = useState<{ used: number; limit: number }>({ used: 3, limit: 3 });
  const [quotaStatus, setQuotaStatus] = useState<QuotaStatusResponse | null>(null);

  // Load connectors when component mounts
  useEffect(() => {
    const fetchConnectors = async () => {
      try {
        const data = await getConnectors();
        setConnectors(data);
        setLoadingConnectors(false);
      } catch (err) {
        console.error('Error fetching connectors:', err);
        setError('Failed to load git connectors. Please try again later.');
        setLoadingConnectors(false);
      }
    };

    fetchConnectors();
  }, []);

  useEffect(() => {
    apiClient
      .get<QuotaStatusResponse>('/quota/status')
      .then((data) => setQuotaStatus(data))
      .catch(() => setQuotaStatus(null));
  }, []);

  // Extract base URL from input URL
  const extractBaseUrl = (url: string): string => {
    try {
      const parsedUrl = new URL(url);
      return `${parsedUrl.protocol}//${parsedUrl.hostname}`;
    } catch (err) {
      return '';
    }
  };

  // Check if base URL is in connectors
  const isUrlSupported = (url: string): boolean => {
    if (!url) return false;

    const baseUrl = extractBaseUrl(url);
    if (!baseUrl) return false;

    return connectors.some(connector =>
      connector.provider_url && connector.provider_url.includes(baseUrl)
    );
  };

  // Handle form submission
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setSuccess(null);

    // Validate URL
    if (!url) {
      setError('Please enter a URL');
      return;
    }

    try {
      // Validate URL format
      new URL(url);
    } catch (err) {
      setError('Please enter a valid URL');
      return;
    }

    // Check if URL is from a supported provider
    if (!isUrlSupported(url)) {
      setError(
        'This URL is not from a connected Git provider. Please connect the appropriate Git provider first.'
      );
      return;
    }

    if (quotaStatus?.envelope?.trial_readonly) {
      setError('Trial is read-only for this organization. Upgrade your plan to resume review creation.');
      return;
    }
    if (quotaStatus?.envelope?.blocked || quotaStatus?.can_trigger_reviews === false) {
      setError('Review creation is currently blocked by plan/quota limits. Upgrade or wait for usage reset.');
      return;
    }

    setLoading(true);

    try {
      const request: TriggerReviewRequest = { url };
      const response = await triggerReview(request);

      setSuccess(`Review successfully triggered! Review ID: ${response.reviewId}`);
      setUrl('');

      // Navigate directly to the new review's detail page
      setTimeout(() => {
        navigate(`/reviews/${response.reviewId}`);
      }, 1500);

    } catch (err: any) {
      console.error('Error triggering review:', err);

      // Check for subscription limit errors (HTTP 402)
      if (err?.status === 402) {
        const errorData = err?.data || err;
        const errorCode = errorData.code;

        if (errorCode === 'DAILY_LIMIT_EXCEEDED') {
          setLimitInfo({
            used: errorData.used || 3,
            limit: errorData.limit || 3,
          });
          setUpgradeReason('DAILY_LIMIT');
          setShowUpgradeModal(true);
        } else if (errorCode === 'NOT_ORG_CREATOR') {
          setUpgradeReason('NOT_ORG_CREATOR');
          setShowUpgradeModal(true);
        } else {
          setError(errorData.message || err.message || 'Failed to trigger review. Please try again later.');
        }
      } else if (err?.status === 403) {
        // LOC quota exceeded (from new backend enforcement)
        const errorData = err?.data || err;
        setError(errorData.error || 'Monthly LOC quota exceeded. Upgrade your plan or wait for reset.');
      } else if (err?.status === 429) {
        const trialReadOnly = Boolean(err?.data?.envelope?.trial_readonly);
        if (trialReadOnly) {
          setError('Trial is read-only for this organization. Upgrade your plan to resume review creation.');
        } else {
          setError(err?.data?.error || 'LOC quota exhausted for this billing period. Upgrade your plan or wait for reset.');
        }
      } else {
        setError(err.message || 'Failed to trigger review. Please try again later.');
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <ErrorBoundary>
      <div className="container mx-auto px-4 py-8">
        <PageHeader
          title="New Review"
          description="Generate review comments safely — no comments posted automatically"
          actions={
            <Button
              variant="outline"
              onClick={() => navigate(-1)}
              icon={<svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" />
              </svg>}
            >
              Back
            </Button>
          }
        />

        <Card className="max-w-3xl mx-auto mt-8">
          {loadingConnectors ? (
            <div className="flex justify-center items-center py-12">
              <Spinner size="lg" />
            </div>
          ) : connectors.length === 0 ? (
            <EmptyState
              icon={<Icons.EmptyState />}
              title="No Git Providers Connected"
              description="Please connect a Git provider before triggering a review"
              action={
                <Button
                  variant="primary"
                  onClick={() => navigate('/git')}
                  icon={<Icons.Git />}
                >
                  Connect Git Provider
                </Button>
              }
            />
          ) : (
            <form onSubmit={handleSubmit}>
              {/* Safety Banner */}
              <SafetyBanner variant="detailed" className="mb-6" />

              <h3 className="text-lg font-medium text-white mb-4">Enter Merge/Pull Request URL</h3>

              {/* LOC 90% Warning Banner */}
              {/* LOC 100% Exhausted Banner */}
              {!quotaStatus?.envelope?.blocked && !quotaStatus?.envelope?.trial_readonly && (quotaStatus?.envelope?.usage_percent ?? 0) >= 100 && (
                <QuotaExhaustedBanner
                  locUsed={quotaStatus.envelope?.loc_used_month ?? 0}
                  locLimit={quotaStatus.envelope?.loc_limit_month ?? 0}
                  usagePct={quotaStatus.envelope?.usage_percent ?? 0}
                  onUpgrade={() => navigate(quotaStatus.envelope?.upgrade_url || '/settings-subscriptions-overview')}
                />
              )}
              {!quotaStatus?.envelope?.blocked && !quotaStatus?.envelope?.trial_readonly && quotaStatus?.envelope?.threshold_state === '90' && (quotaStatus?.envelope?.usage_percent ?? 0) < 100 && (
                <QuotaWarningBanner
                  locUsed={quotaStatus.envelope?.loc_used_month ?? 0}
                  locLimit={quotaStatus.envelope?.loc_limit_month ?? 0}
                  usagePct={quotaStatus.envelope?.usage_percent ?? 0}
                  onUpgrade={() => navigate(quotaStatus.envelope?.upgrade_url || '/settings-subscriptions-overview')}
                />
              )}

              {/* Blocked / Trial Read-Only / Free Plan Read-Only Banner */}
              {(isReadOnly || quotaStatus?.envelope?.trial_readonly || quotaStatus?.envelope?.blocked || quotaStatus?.can_trigger_reviews === false) && (
                <Alert
                  variant="error"
                  className="mb-4"
                  icon={<Icons.Error />}
                >
                  <div>
                    <div className="font-medium text-red-100">
                      {isReadOnly ? 'Free Plan Read-Only Mode' : (quotaStatus?.envelope?.trial_readonly ? 'Trial Read-Only Active' : 'Monthly LOC Quota Exceeded')}
                    </div>
                    <div className="text-sm mt-1 text-red-200/90">
                      {isReadOnly 
                        ? 'Review creation is not available on the Free plan. Upgrade to a paid plan to start using AI code reviews.'
                        : (quotaStatus?.envelope?.trial_readonly
                          ? 'This organization is in trial read-only mode. Upgrade from Subscription Management to continue triggering reviews.'
                          : `You've used ${(quotaStatus?.envelope?.loc_used_month ?? 0).toLocaleString()} of ${(quotaStatus?.envelope?.loc_limit_month ?? 0).toLocaleString()} LOC this month. Reviews are blocked until your quota resets${quotaStatus?.envelope?.billing_period_end ? ` on ${new Date(quotaStatus.envelope.billing_period_end).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })}` : ''} or you upgrade your plan.`)}
                    </div>
                    <button
                      type="button"
                      onClick={() => navigate(quotaStatus?.envelope?.upgrade_url || '/settings-subscriptions-overview')}
                      className="mt-2 px-3 py-1 bg-red-600 hover:bg-red-500 text-white text-xs font-semibold rounded transition-colors"
                    >
                      Upgrade Now
                    </button>
                  </div>
                </Alert>
              )}

              {error && (
                <Alert
                  variant="error"
                  className="mb-4"
                  icon={<Icons.Error />}
                >
                  {error}
                </Alert>
              )}

              {success && (
                <Alert
                  variant="success"
                  className="mb-4"
                  icon={<Icons.Success />}
                >
                  <div>
                    <div>{success}</div>
                    <div className="text-sm mt-1">Redirecting to reviews list...</div>
                  </div>
                </Alert>
              )}

              <div className="mb-4">
                <Input
                  label="URL"
                  placeholder="https://gitlab.com/your-group/your-project/-/merge_requests/123"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  disabled={loading}
                  icon={<svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
                  </svg>}
                  helperText="Enter the URL of the merge/pull request to start a review"
                />
              </div>

              {/* URL Examples */}
              <div className="mb-6 p-4 bg-slate-700 rounded-lg">
                <h4 className="text-sm font-medium text-white mb-2">Supported URL Examples:</h4>
                <div className="space-y-1 text-xs text-slate-300">
                  <div>• GitLab: https://gitlab.com/group/project/-/merge_requests/123</div>
                  <div>• GitHub: https://github.com/owner/repo/pull/123</div>
                  <div>• Bitbucket: https://bitbucket.org/workspace/repo/pull-requests/123</div>
                </div>
                <div className="mt-3 pt-3 border-t border-slate-600">
                  <p className="text-xs text-green-300 font-medium">
                    🔒 Safe run - No comments will be posted to your PR/MR
                  </p>
                </div>
              </div>

              <div className="flex justify-end space-x-3">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => navigate('/reviews')}
                  disabled={loading}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  variant="primary"
                  disabled={
                    loading ||
                    !url.trim() ||
                    isReadOnly ||
                    Boolean(quotaStatus?.envelope?.blocked) ||
                    Boolean(quotaStatus?.envelope?.trial_readonly) ||
                    (quotaStatus?.envelope?.usage_percent ?? 0) >= 100 ||
                    quotaStatus?.can_trigger_reviews === false
                  }
                  isLoading={loading}
                >
                  Start Review
                </Button>
              </div>
            </form>
          )}
        </Card>

        {/* Upgrade Modal */}
        <UpgradePromptModal
          isOpen={showUpgradeModal}
          onClose={() => setShowUpgradeModal(false)}
          reason={upgradeReason}
          currentCount={limitInfo.used}
          limit={limitInfo.limit}
        />
      </div>
    </ErrorBoundary>
  );
};

export default NewReview;
