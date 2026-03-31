import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import moment from 'moment-timezone';
import { isCloudMode } from '../../utils/deploymentMode';
import { useAppSelector } from '../../store/configureStore';
import { useOrgContext } from '../../hooks/useOrgContext';
import LicenseManagement from '../Licenses/LicenseManagement';
import { CancelSubscriptionModal } from '../../components/Subscriptions';
import apiClient from '../../api/apiClient';

type BillingPlan = {
  plan_code: string;
  monthly_loc_limit: number;
  monthly_price_usd: number;
  trial_days: number;
};

type BillingStatusResponse = {
  billing: {
    current_plan_code: string;
    billing_period_start: string;
    billing_period_end: string;
    loc_used_month: number;
    trial_readonly: boolean;
    scheduled_plan_code?: string | null;
    scheduled_plan_effective_at?: string | null;
  };
  available_plans: BillingPlan[];
};

type QuotaStatusResponse = {
  can_trigger_reviews: boolean;
  envelope?: {
    blocked?: boolean;
    trial_readonly?: boolean;
    usage_pct?: number;
  };
};

type BillingUsageSummaryResponse = {
  period_start: string;
  period_end: string;
  total_billable_loc: number;
  total_input_tokens: number;
  total_output_tokens: number;
  total_tokens: number;
  total_cost_usd: number;
  accounted_operations: number;
  token_tracked_ops: number;
  latest_accounted_at?: string;
};

type BillingUsageOperationsResponse = {
  operations: Array<{
    review_id?: number;
    operation_type: string;
    trigger_source: string;
    operation_id: string;
    billable_loc: number;
    provider?: string;
    model?: string;
    input_tokens?: number;
    output_tokens?: number;
    cost_usd?: number;
    accounted_at: string;
  }>;
  limit: number;
  offset: number;
  count: number;
};

const SubscriptionTab: React.FC = () => {
  const navigate = useNavigate();
  const { currentOrg, isSuperAdmin } = useOrgContext();
  
  // Initialize tab from URL path
  const getInitialTab = (): 'overview' | 'assignments' => {
    const hash = window.location.hash;
    if (hash.includes('settings-subscriptions-assign')) return 'assignments';
    return 'overview';
  };
  
  const [activeTab, setActiveTab] = useState<'overview' | 'assignments'>(getInitialTab);

  // Check if user can manage licenses (owner or super admin)
  const canManageLicenses = isSuperAdmin || currentOrg?.role === 'owner';

  useEffect(() => {
    // Redirect to license page if in self-hosted mode
    if (!isCloudMode()) {
      navigate('/settings/license', { replace: true });
    }
  }, [navigate]);

  // Update tab when location changes
  useEffect(() => {
    const hash = window.location.hash;
    if (hash.includes('settings-subscriptions-assign')) {
      setActiveTab('assignments');
    } else if (hash.includes('settings-subscriptions-overview')) {
      setActiveTab('overview');
    }
  }, [window.location.hash]);

  const handleTabChange = (tab: 'overview' | 'assignments') => {
    setActiveTab(tab);
    const route = tab === 'assignments' ? '/settings-subscriptions-assign' : '/settings-subscriptions-overview';
    navigate(route);
  };

  // Only render if in cloud mode
  if (!isCloudMode()) {
    return null;
  }

  return (
    <div className="p-6 space-y-6" data-testid="subscription-tab">
      {/* Tab Navigation */}
      <div className="border-b border-slate-700 -mx-6 px-6">
        <div className="flex space-x-1">
          <button
            onClick={() => handleTabChange('overview')}
            className={`px-4 py-3 text-sm font-medium transition-colors ${
              activeTab === 'overview'
                ? 'text-white border-b-2 border-blue-500'
                : 'text-slate-400 hover:text-slate-300'
            }`}
          >
            Overview
          </button>
          {canManageLicenses && (
            <button
              onClick={() => handleTabChange('assignments')}
              className={`px-4 py-3 text-sm font-medium transition-colors ${
                activeTab === 'assignments'
                  ? 'text-white border-b-2 border-blue-500'
                  : 'text-slate-400 hover:text-slate-300'
              }`}
            >
              License Assignments
            </button>
          )}
        </div>
      </div>

      {/* Tab Content */}
      {activeTab === 'overview' ? (
        <OverviewTab navigate={navigate} />
      ) : canManageLicenses ? (
        <AssignmentsTab />
      ) : (
        <OverviewTab navigate={navigate} />
      )}
    </div>
  );
};

// Overview Tab Component
const OverviewTab: React.FC<{ navigate: any }> = ({ navigate }) => {
  const { currentOrg, isSuperAdmin } = useOrgContext();
  const [showCancelModal, setShowCancelModal] = useState(false);
  const [subscriptionId, setSubscriptionId] = useState<string | null>(null);
  const [pendingCancel, setPendingCancel] = useState(false);
  const [status, setStatus] = useState<string>('');
  const [statusLoading, setStatusLoading] = useState(false);
  const [displayExpiry, setDisplayExpiry] = useState<string | null>(null);
  const [billingStatus, setBillingStatus] = useState<BillingStatusResponse | null>(null);
  const [quotaStatus, setQuotaStatus] = useState<QuotaStatusResponse | null>(null);
  const [billingError, setBillingError] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState(false);
  const [selectedUpgradePlan, setSelectedUpgradePlan] = useState('');
  const [selectedDowngradePlan, setSelectedDowngradePlan] = useState('');
  const [usageSummary, setUsageSummary] = useState<BillingUsageSummaryResponse | null>(null);
  const [usageOps, setUsageOps] = useState<BillingUsageOperationsResponse['operations']>([]);
  
  // Read plan from current org (org-scoped), not from Auth.user
  const rawPlanType = (currentOrg?.plan_type || 'free').toLowerCase();
  const planType = rawPlanType === 'free' ? 'free_30k' : rawPlanType === 'team' ? 'team_32usd' : rawPlanType;
  const licenseExpiresAt = currentOrg?.license_expires_at;
  const isTeamPlan = planType === 'team_32usd' || planType.includes('team');
  const isFree = planType === 'free_30k' || planType === 'free';
  const canManageBilling = isSuperAdmin || currentOrg?.role === 'owner' || currentOrg?.role === 'admin';

  // Fetch current subscription (org-scoped)
  useEffect(() => {
    if (isTeamPlan && currentOrg?.id) {
      setStatusLoading(true);
      setPendingCancel(false);
      setStatus('');
      apiClient
        .get('/subscriptions/current', {
          headers: {
            'X-Org-Context': currentOrg.id.toString(),
          },
        })
        .then((data: any) => {
          if (data) {
            setSubscriptionId(data.subscription_id || null);
            setPendingCancel(Boolean(data.cancel_at_period_end));
            setStatus(data.status || '');
            const expirySrc = data.cancel_at_period_end ? data.current_period_end : data.license_expires_at;
            setDisplayExpiry(expirySrc || licenseExpiresAt || null);
          }
        })
        .catch(err => console.error('Failed to fetch current subscription:', err))
        .finally(() => setStatusLoading(false));
    } else {
      setStatusLoading(false);
    }
  }, [isTeamPlan, currentOrg?.id, licenseExpiresAt]);

  useEffect(() => {
    if (!currentOrg?.id) return;
    refreshBilling().catch((err) => {
      setBillingError(err?.message || 'Failed to load billing status');
    });
  }, [currentOrg?.id]);

  useEffect(() => {
    if (!billingStatus?.available_plans || !billingStatus?.billing?.current_plan_code) return;
    const current = billingStatus.available_plans.find((p) => p.plan_code === billingStatus.billing.current_plan_code);
    if (!current) return;
    const upgrades = billingStatus.available_plans
      .filter((p) => p.monthly_loc_limit > current.monthly_loc_limit)
      .sort((a, b) => a.monthly_loc_limit - b.monthly_loc_limit);
    const downgrades = billingStatus.available_plans
      .filter((p) => p.monthly_loc_limit < current.monthly_loc_limit)
      .sort((a, b) => b.monthly_loc_limit - a.monthly_loc_limit);
    if (!selectedUpgradePlan && upgrades.length > 0) {
      setSelectedUpgradePlan(upgrades[0].plan_code);
    }
    if (!selectedDowngradePlan && downgrades.length > 0) {
      setSelectedDowngradePlan(downgrades[0].plan_code);
    }
  }, [billingStatus, selectedUpgradePlan, selectedDowngradePlan]);

  const handleCancelSuccess = () => {
    // Reload the page to reflect updated subscription status
    window.location.reload();
  };

  const refreshBilling = async () => {
    const emptyOpsResponse: BillingUsageOperationsResponse = { operations: [], limit: 10, offset: 0, count: 0 };
    const [billing, quota, summary, operations] = await Promise.all([
      apiClient.get<BillingStatusResponse>('/billing/status'),
      apiClient.get<QuotaStatusResponse>('/quota/status').catch((): null => null),
      apiClient.get<BillingUsageSummaryResponse>('/billing/usage/summary').catch((): null => null),
      apiClient.get<BillingUsageOperationsResponse>('/billing/usage/operations?limit=10&offset=0').catch((): BillingUsageOperationsResponse => emptyOpsResponse),
    ]);
    setBillingStatus(billing);
    setQuotaStatus(quota);
    setUsageSummary(summary);
    setUsageOps(operations.operations || []);
  };

  const runBillingAction = async (mode: 'upgrade' | 'schedule_downgrade' | 'cancel_downgrade') => {
    setActionLoading(true);
    setBillingError(null);
    try {
      if (mode === 'upgrade') {
        if (selectedUpgradePlan === 'team_32usd') {
          navigate('/checkout/team?period=monthly');
          return;
        }
        await apiClient.post('/billing/upgrade', { target_plan_code: selectedUpgradePlan });
      } else if (mode === 'schedule_downgrade') {
        await apiClient.post('/billing/downgrade/schedule', { target_plan_code: selectedDowngradePlan });
      } else {
        await apiClient.post('/billing/downgrade/cancel', {});
      }
      await refreshBilling();
    } catch (err: any) {
      setBillingError(err?.message || 'Billing action failed');
    } finally {
      setActionLoading(false);
    }
  };

  const formatDate = (dateString: string | null | undefined) => {
    if (!dateString) return null;
    const userTimezone = moment.tz.guess();
    return moment.tz(dateString, userTimezone).format('MMM D, YYYY, h:mm A z');
  };

  const getPlanDisplayName = (plan: string) => {
    const normalized = plan.toLowerCase();
    if (normalized === 'team_32usd' || normalized === 'team' || normalized.includes('team')) return 'Team 32 USD';
    if (normalized === 'free_30k' || normalized === 'free') return 'Free 30k BYOK';
    return plan;
  };

  const dailyLimit = isTeamPlan ? 'Unlimited' : 'Quota-based (30,000 LOC/month)';

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold text-white mb-2">Subscription Management</h2>
        <p className="text-sm text-slate-400 mb-4">
          Manage billing, quota limits, and AI execution mode
        </p>
      </div>

      <div className="bg-slate-800/50 border border-slate-700 rounded-lg p-4">
        <h3 className="text-sm font-semibold text-white mb-2">Plan execution model</h3>
        <div className="space-y-1 text-sm text-slate-300">
          <p><span className="text-slate-400">Free 30k:</span> Bring your own AI key (BYOK) is required.</p>
          <p><span className="text-slate-400">Team 32 USD:</span> Hosted Auto model is default; BYOK remains optional.</p>
        </div>
      </div>

      {(billingStatus?.billing?.trial_readonly || quotaStatus?.envelope?.trial_readonly) && (
        <div className="bg-amber-500/10 border border-amber-400/40 rounded-lg p-4">
          <p className="text-amber-200 text-sm font-medium">Trial is now read-only</p>
          <p className="text-amber-100/90 text-sm mt-1">
            Review creation is blocked until the organization is moved to a paid LOC plan.
          </p>
        </div>
      )}

      {(quotaStatus?.envelope?.blocked || quotaStatus?.can_trigger_reviews === false) && (
        <div className="bg-red-500/10 border border-red-400/40 rounded-lg p-4">
          <p className="text-red-200 text-sm font-medium">Quota blocked</p>
          <p className="text-red-100/90 text-sm mt-1">
            This organization has reached its current usage limit. Upgrade now or wait for the next billing period.
          </p>
        </div>
      )}

      {/* Current Subscription Section */}
      <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h3 className="text-md font-semibold text-white">Current Subscription</h3>
            <p className="text-sm text-slate-400 mt-1">{getPlanDisplayName(planType)}</p>
          </div>
          <div className="flex items-center gap-2">
            <div className={`px-4 py-2 rounded-lg text-sm font-medium ${
              pendingCancel
                ? 'bg-amber-500/10 text-amber-400 border border-amber-500/40'
                : statusLoading
                ? 'bg-slate-700 text-slate-300 border border-slate-600'
                : isTeamPlan
                ? 'bg-blue-900/40 text-blue-300'
                : 'bg-emerald-900/40 text-emerald-300'
            }`}>
              {statusLoading ? (
                <span className="flex items-center gap-2">
                  <span className="inline-block w-3 h-3 border-2 border-slate-500 border-t-transparent rounded-full animate-spin" aria-label="Loading status" />
                  Loading...
                </span>
              ) : pendingCancel ? 'PENDING EXPIRY' : (status || 'Active').toUpperCase()}
            </div>
            {subscriptionId && (isSuperAdmin || currentOrg?.role === 'owner') && (
              <button
                onClick={() => navigate(`/subscribe/subscriptions/${subscriptionId}/assign`)}
                className="px-3 py-1.5 text-xs font-medium bg-slate-700 hover:bg-slate-600 text-white rounded-lg border border-slate-600 transition-colors"
              >
                View Subscription Details
              </button>
            )}
          </div>
        </div>

        {pendingCancel && displayExpiry && (
          <div className="mb-4 p-3 bg-slate-700/50 border border-slate-600 rounded-lg">
            <div className="flex items-start gap-3">
              <svg className="w-5 h-5 text-slate-400 mt-0.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <div>
                <p className="text-slate-300 text-sm font-medium">Subscription Cancelled</p>
                <p className="text-slate-400 text-sm mt-1">
                  Your access will remain active until <span className="text-white">{formatDate(displayExpiry)}</span>. After this date, you will move to the free hobby plan and your team members will need a new subscription to continue access.
                </p>
              </div>
            </div>
          </div>
        )}

        {isTeamPlan && (displayExpiry || licenseExpiresAt) && (
          <div className="mb-4 p-3 bg-slate-900/60 border border-slate-700 rounded-lg">
            <div className="text-slate-400 text-xs mb-1">License Expires</div>
            <div className="text-white font-medium">{formatDate(displayExpiry || licenseExpiresAt)}</div>
          </div>
        )}

        <div className="space-y-3 text-sm">
          <div className="flex justify-between items-center py-2 border-b border-slate-700">
            <span className="text-slate-400">Daily Review Limit</span>
            <span className="text-white font-medium">{dailyLimit}</span>
          </div>
          <div className="flex justify-between items-center py-2 border-b border-slate-700">
            <span className="text-slate-400">AI-Powered Analysis</span>
            <span className="text-emerald-400">✓ Included</span>
          </div>
          <div className="flex justify-between items-center py-2 border-b border-slate-700">
            <span className="text-slate-400">AI Execution Mode</span>
            <span className="text-white font-medium">{isTeamPlan ? 'Auto default (BYOK optional)' : 'BYOK required'}</span>
          </div>
          <div className="flex justify-between items-center py-2 border-b border-slate-700">
            <span className="text-slate-400">Git Provider Integration</span>
            <span className="text-emerald-400">✓ Included</span>
          </div>
          <div className="flex justify-between items-center py-2">
            <span className="text-slate-400">Priority Support</span>
            <span className={isTeamPlan ? 'text-emerald-400' : 'text-slate-500'}>
              {isTeamPlan ? '✓ Included' : 'Not included'}
            </span>
          </div>
        </div>
      </div>

      {/* Upgrade Section - only show for free users */}
      {isFree && (
        <div className="bg-gradient-to-r from-blue-900/40 to-purple-900/40 border border-blue-700/50 rounded-lg p-6">
          <h3 className="text-md font-semibold text-white mb-2">Upgrade to Team Subscription</h3>
          <p className="text-sm text-slate-300 mb-4">
            Switch from BYOK to hosted Auto defaults with optional BYOK override and higher LOC capacity
          </p>
          <ul className="space-y-2 text-sm text-slate-300 mb-4">
            <li className="flex items-center">
              <svg className="w-4 h-4 text-emerald-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
              </svg>
              Unlimited daily reviews
            </li>
            <li className="flex items-center">
              <svg className="w-4 h-4 text-emerald-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
              </svg>
              Priority support
            </li>
            <li className="flex items-center">
              <svg className="w-4 h-4 text-emerald-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
              </svg>
              Advanced analytics
            </li>
            <li className="flex items-center">
              <svg className="w-4 h-4 text-emerald-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
              </svg>
              Team collaboration features
            </li>
          </ul>
          <button
            onClick={() => navigate('/checkout/team?period=monthly')}
            className="w-full px-4 py-2 text-sm rounded bg-blue-600 hover:bg-blue-500 text-white font-medium transition-colors"
          >
            Upgrade Now
          </button>
        </div>
      )}

      {/* Team Subscription Benefits - show for team users */}
      {isTeamPlan && (
        <div className="bg-gradient-to-r from-blue-900/40 to-purple-900/40 border border-blue-700/50 rounded-lg p-6">
          <div className="flex items-center justify-between mb-2">
            <h3 className="text-md font-semibold text-white">Team Subscription Benefits</h3>
            {subscriptionId && !pendingCancel && (
              <button
                onClick={() => setShowCancelModal(true)}
                className="px-3 py-1.5 text-xs font-medium text-red-300 bg-red-900/40 hover:bg-red-900/60 border border-red-500/30 hover:border-red-500/50 rounded-lg transition-colors"
              >
                Cancel Subscription
              </button>
            )}
          </div>
          <p className="text-sm text-slate-300 mb-4">
            You're enjoying all premium features
          </p>
          <ul className="space-y-2 text-sm text-slate-300">
            <li className="flex items-center">
              <svg className="w-4 h-4 text-emerald-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
              </svg>
              Unlimited daily reviews
            </li>
            <li className="flex items-center">
              <svg className="w-4 h-4 text-emerald-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
              </svg>
              Priority support
            </li>
            <li className="flex items-center">
              <svg className="w-4 h-4 text-emerald-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
              </svg>
              Advanced analytics
            </li>
            <li className="flex items-center">
              <svg className="w-4 h-4 text-emerald-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
              </svg>
              Team collaboration features
            </li>
          </ul>
        </div>
      )}

      {/* Cancel Subscription Modal */}
      {subscriptionId && (
        <CancelSubscriptionModal
          isOpen={showCancelModal}
          onClose={() => setShowCancelModal(false)}
          onSuccess={handleCancelSuccess}
          subscriptionId={subscriptionId}
          expiryDate={licenseExpiresAt}
        />
      )}

      {/* Billing History Placeholder */}
      <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-6">
        <h3 className="text-md font-semibold text-white mb-2">Billing History</h3>
        <p className="text-sm text-slate-400">
          {isFree ? 'No billing history available for free plan' : 'View your billing history in the License Assignments tab'}
        </p>
      </div>

      {usageSummary && (
        <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-6 space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-md font-semibold text-white">Usage and Token Consumption</h3>
            <span className="text-xs text-slate-400">
              {formatDate(usageSummary.period_start)} to {formatDate(usageSummary.period_end)}
            </span>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-3 text-sm">
            <div className="bg-slate-900/70 border border-slate-700 rounded p-3">
              <p className="text-slate-400">Total LOC</p>
              <p className="text-white font-semibold">{usageSummary.total_billable_loc.toLocaleString()}</p>
            </div>
            <div className="bg-slate-900/70 border border-slate-700 rounded p-3">
              <p className="text-slate-400">Input Tokens</p>
              <p className="text-white font-semibold">{usageSummary.total_input_tokens.toLocaleString()}</p>
            </div>
            <div className="bg-slate-900/70 border border-slate-700 rounded p-3">
              <p className="text-slate-400">Output Tokens</p>
              <p className="text-white font-semibold">{usageSummary.total_output_tokens.toLocaleString()}</p>
            </div>
            <div className="bg-slate-900/70 border border-slate-700 rounded p-3">
              <p className="text-slate-400">Total Tokens</p>
              <p className="text-white font-semibold">{usageSummary.total_tokens.toLocaleString()}</p>
            </div>
            <div className="bg-slate-900/70 border border-slate-700 rounded p-3">
              <p className="text-slate-400">Estimated Cost (USD)</p>
              <p className="text-white font-semibold">${usageSummary.total_cost_usd.toFixed(4)}</p>
            </div>
            <div className="bg-slate-900/70 border border-slate-700 rounded p-3">
              <p className="text-slate-400">Operations</p>
              <p className="text-white font-semibold">
                {usageSummary.accounted_operations.toLocaleString()} ({usageSummary.token_tracked_ops.toLocaleString()} token-tracked)
              </p>
            </div>
          </div>

          <div>
            <p className="text-sm text-slate-300 mb-2">Recent Operations</p>
            <div className="overflow-x-auto border border-slate-700 rounded-lg">
              <table className="min-w-full text-xs text-left">
                <thead className="bg-slate-900/80 text-slate-300">
                  <tr>
                    <th className="px-3 py-2">When</th>
                    <th className="px-3 py-2">Type</th>
                    <th className="px-3 py-2">LOC</th>
                    <th className="px-3 py-2">Tokens (In/Out)</th>
                    <th className="px-3 py-2">Cost</th>
                    <th className="px-3 py-2">Provider/Model</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-700 bg-slate-950/40">
                  {usageOps.length === 0 && (
                    <tr>
                      <td colSpan={6} className="px-3 py-3 text-slate-400">No operations found in current billing period.</td>
                    </tr>
                  )}
                  {usageOps.map((op) => (
                    <tr key={`${op.operation_id}-${op.accounted_at}`}>
                      <td className="px-3 py-2 text-slate-300">{formatDate(op.accounted_at)}</td>
                      <td className="px-3 py-2 text-white">{op.operation_type}</td>
                      <td className="px-3 py-2 text-white">{op.billable_loc.toLocaleString()}</td>
                      <td className="px-3 py-2 text-white">{(op.input_tokens || 0).toLocaleString()} / {(op.output_tokens || 0).toLocaleString()}</td>
                      <td className="px-3 py-2 text-white">{op.cost_usd !== undefined ? `$${op.cost_usd.toFixed(4)}` : 'N/A'}</td>
                      <td className="px-3 py-2 text-slate-300">{op.provider || 'unknown'} / {op.model || 'unknown'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {canManageBilling && billingStatus && (
        <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-6 space-y-4">
          <h3 className="text-md font-semibold text-white">Billing Actions</h3>

          {billingError && (
            <div className="bg-red-500/10 border border-red-500/40 rounded-lg p-3 text-sm text-red-200">
              {billingError}
            </div>
          )}

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="p-4 bg-slate-900/70 border border-slate-700 rounded-lg space-y-3">
              <p className="text-sm text-slate-300 font-medium">Immediate Upgrade</p>
              <select
                value={selectedUpgradePlan}
                onChange={(e) => setSelectedUpgradePlan(e.target.value)}
                className="w-full bg-slate-800 border border-slate-600 rounded px-3 py-2 text-sm text-white"
                disabled={actionLoading}
              >
                {billingStatus.available_plans
                  .filter((p) => {
                    const current = billingStatus.available_plans.find((x) => x.plan_code === billingStatus.billing.current_plan_code);
                    return current ? p.monthly_loc_limit > current.monthly_loc_limit : false;
                  })
                  .map((plan) => (
                    <option key={plan.plan_code} value={plan.plan_code}>
                      {plan.plan_code} ({plan.monthly_loc_limit.toLocaleString()} LOC / ${plan.monthly_price_usd})
                    </option>
                  ))}
              </select>
              <button
                className="w-full px-3 py-2 rounded bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-sm font-medium"
                disabled={actionLoading || !selectedUpgradePlan}
                onClick={() => runBillingAction('upgrade')}
              >
                Upgrade Plan
              </button>
            </div>

            <div className="p-4 bg-slate-900/70 border border-slate-700 rounded-lg space-y-3">
              <p className="text-sm text-slate-300 font-medium">Schedule Downgrade</p>
              <select
                value={selectedDowngradePlan}
                onChange={(e) => setSelectedDowngradePlan(e.target.value)}
                className="w-full bg-slate-800 border border-slate-600 rounded px-3 py-2 text-sm text-white"
                disabled={actionLoading}
              >
                {billingStatus.available_plans
                  .filter((p) => {
                    const current = billingStatus.available_plans.find((x) => x.plan_code === billingStatus.billing.current_plan_code);
                    return current ? p.monthly_loc_limit < current.monthly_loc_limit : false;
                  })
                  .map((plan) => (
                    <option key={plan.plan_code} value={plan.plan_code}>
                      {plan.plan_code} ({plan.monthly_loc_limit.toLocaleString()} LOC / ${plan.monthly_price_usd})
                    </option>
                  ))}
              </select>
              <button
                className="w-full px-3 py-2 rounded bg-amber-600 hover:bg-amber-500 disabled:opacity-50 text-sm font-medium"
                disabled={actionLoading || !selectedDowngradePlan}
                onClick={() => runBillingAction('schedule_downgrade')}
              >
                Schedule Downgrade
              </button>
              {billingStatus.billing.scheduled_plan_code && (
                <button
                  className="w-full px-3 py-2 rounded bg-slate-700 hover:bg-slate-600 disabled:opacity-50 text-sm font-medium"
                  disabled={actionLoading}
                  onClick={() => runBillingAction('cancel_downgrade')}
                >
                  Cancel Scheduled Downgrade
                </button>
              )}
            </div>
          </div>

          {billingStatus.billing.scheduled_plan_code && (
            <div className="text-sm text-slate-300 bg-slate-900/40 border border-slate-700 rounded-lg p-3">
              Scheduled: <span className="text-white font-medium">{billingStatus.billing.scheduled_plan_code}</span>
              {billingStatus.billing.scheduled_plan_effective_at && (
                <span> effective {formatDate(billingStatus.billing.scheduled_plan_effective_at)}</span>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
};

// Assignments Tab Component
const AssignmentsTab: React.FC = () => {
  return (
    <div className="-mx-6 -my-6">
      <LicenseManagement />
    </div>
  );
};

export default SubscriptionTab;
