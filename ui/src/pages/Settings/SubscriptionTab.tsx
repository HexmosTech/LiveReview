import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import moment from 'moment-timezone';
import { isCloudMode } from '../../utils/deploymentMode';
import { useAppSelector } from '../../store/configureStore';
import { useOrgContext } from '../../hooks/useOrgContext';
import LicenseManagement from '../Licenses/LicenseManagement';
import { CancelSubscriptionModal } from '../../components/Subscriptions';
import apiClient from '../../api/apiClient';

declare global {
  interface Window {
    Razorpay: any;
  }
}

const ensureRazorpay = (): Promise<void> => {
  if (typeof window !== 'undefined' && window.Razorpay) {
    return Promise.resolve();
  }
  return new Promise((resolve, reject) => {
    const script = document.createElement('script');
    script.src = 'https://checkout.razorpay.com/v1/checkout.js';
    script.async = true;
    script.onload = () => resolve();
    script.onerror = () => reject(new Error('Failed to load Razorpay'));
    document.head.appendChild(script);
  });
};

const ensureRazorpayStyles = () => {
  if (document.getElementById('razorpay-custom-style')) return;
  const style = document.createElement('style');
  style.id = 'razorpay-custom-style';
  style.textContent = `
    body.razorpay-active {
      overflow: hidden !important;
    }

    body.razorpay-active .razorpay-backdrop {
      background: rgba(2, 6, 23, 0.86) !important;
      backdrop-filter: blur(1px);
    }

    body.razorpay-active .razorpay-container {
      background: rgba(2, 6, 23, 0.86) !important;
    }
  `;
  document.head.appendChild(style);
};

const cleanupRazorpayOverlay = () => {
  document.body.classList.remove('razorpay-active');
};

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

type UpgradeActionResponse = {
  message?: string;
  upgrade_request_id?: string;
  status?: string;
  resolved?: boolean;
  plan_code?: string;
  transition_mode?: string;
  scheduled_plan_code?: string;
  scheduled_plan_effective_at?: string;
  proration?: {
    from_plan_code?: string;
    to_plan_code?: string;
    cycle_start?: string;
    cycle_end?: string;
    remaining_cycle_fraction?: number;
    charge_amount_cents?: number;
    charge_currency?: string;
    charge_status?: string;
    order_id?: string;
    payment_id?: string;
    immediate_loc_grant?: number;
    next_cycle_price_cents?: number;
    next_cycle_loc_limit?: number;
  };
};

type UpgradePreviewResponse = {
  upgrade_request_id: string;
  preview: {
    from_plan_code: string;
    to_plan_code: string;
    cycle_start: string;
    cycle_end: string;
    remaining_cycle_fraction: number;
    immediate_charge_cents: number;
    immediate_charge_currency: string;
    immediate_loc_grant: number;
    next_cycle_price_cents: number;
    next_cycle_loc_limit: number;
    charge_timing: string;
    plan_switch_timing: string;
    final_payable_cents: number;
  };
  preview_token: string;
  preview_expires_at: string;
  checkout_required?: boolean;
  checkout_path?: string;
};

type PrepareUpgradePaymentResponse = {
  payment_required: boolean;
  upgrade_request_id?: string;
  razorpay_key_id?: string;
  order_id?: string;
  amount_cents?: number;
  currency?: string;
  preview_token: string;
};

type UpgradeRequestStatusResponse = {
  request: {
    upgrade_request_id: string;
    status: string;
    from_plan_code: string;
    to_plan_code: string;
    expected_amount_cents: number;
    currency: string;
    payment_capture_confirmed: boolean;
    subscription_change_confirmed: boolean;
    plan_grant_applied: boolean;
    created_at: string;
    updated_at: string;
    razorpay_order_id?: string | null;
    razorpay_payment_id?: string | null;
    razorpay_subscription_id?: string | null;
    payment_capture_confirmed_at?: string | null;
    subscription_change_confirmed_at?: string | null;
    plan_grant_applied_at?: string | null;
    resolved_at?: string | null;
  } | null;
  events: Array<{
    event_source: string;
    event_type: string;
    event_time: string;
    from_status?: string;
    to_status?: string;
    payload?: Record<string, any>;
  }>;
};

const buildUpgradeCheckoutFailureMessage = (response: any, prepared: PrepareUpgradePaymentResponse): string => {
  const error = response?.error || {};
  const description = error.description || 'Payment failed while processing upgrade';
  const details = [
    error.code ? `code=${error.code}` : '',
    error.reason ? `reason=${error.reason}` : '',
    error.source ? `source=${error.source}` : '',
    error.step ? `step=${error.step}` : '',
  ].filter(Boolean);

  const key = String(prepared?.razorpay_key_id || '').trim();
  const isTestMode = key.startsWith('rzp_test_');
  const currency = String(prepared?.currency || 'USD').trim().toUpperCase();

  if (!isTestMode) {
    if (details.length === 0) {
      return description;
    }
    return `${description} (${details.join(', ')})`;
  }

  const testHint =
    currency === 'INR'
      ? 'Test mode hint: use an INR success test card like 4100 2800 0000 1007 and complete OTP with 4-10 digits.'
      : 'Test mode hint: use an international success test card like 4012 8888 8888 1881 (Visa) or 5555 5555 5555 4444 (Mastercard).';

  if (details.length === 0) {
    return `${description}. ${testHint}`;
  }

  return `${description} (${details.join(', ')}). ${testHint}`;
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
  const [lastUpgradeResult, setLastUpgradeResult] = useState<UpgradeActionResponse | null>(null);
  const [upgradePreview, setUpgradePreview] = useState<UpgradePreviewResponse | null>(null);
  const [showUpgradeModal, setShowUpgradeModal] = useState(false);
  const [upgradeCheckoutLoading, setUpgradeCheckoutLoading] = useState(false);
  const [upgradeScriptReady, setUpgradeScriptReady] = useState(false);
  const [activeUpgradeRequestID, setActiveUpgradeRequestID] = useState<string>('');
  const [upgradeRequestStatus, setUpgradeRequestStatus] = useState<UpgradeRequestStatusResponse | null>(null);
  const [upgradeRequestLoading, setUpgradeRequestLoading] = useState(false);
  
  // Use billing status as source-of-truth; fall back to org-scoped plan only until billing loads.
  const rawPlanType = (currentOrg?.plan_type || 'free').toLowerCase();
  const fallbackPlanCode = rawPlanType === 'free' ? 'free_30k' : rawPlanType === 'team' ? 'team_32usd' : rawPlanType;
  const currentPlanCode = billingStatus?.billing?.current_plan_code || fallbackPlanCode;
  const currentPlan = billingStatus?.available_plans?.find((p) => p.plan_code === currentPlanCode) || null;
  const licenseExpiresAt = currentOrg?.license_expires_at;
  const isFree = currentPlanCode === 'free_30k' || currentPlanCode === 'free';
  const isTeamPlan = !isFree;
  const canManageBilling = isSuperAdmin || currentOrg?.role === 'owner' || currentOrg?.role === 'admin';

  // Fetch current subscription (org-scoped)
  useEffect(() => {
    if (!currentOrg?.id) {
      setStatusLoading(false);
      return;
    }

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
      .catch(() => {
        setSubscriptionId(null);
        setPendingCancel(false);
        setStatus('');
      })
      .finally(() => setStatusLoading(false));
  }, [currentOrg?.id, licenseExpiresAt]);

  useEffect(() => {
    if (!currentOrg?.id) return;
    refreshBilling().catch((err) => {
      setBillingError(err?.message || 'Failed to load billing status');
    });
  }, [currentOrg?.id]);

  useEffect(() => {
    const requestID = String(activeUpgradeRequestID || '').trim();
    if (!requestID || !currentOrg?.id) {
      return;
    }

    let cancelled = false;
    let intervalId: any;

    const tick = async () => {
      if (cancelled) return;
      await refreshUpgradeRequestStatus(requestID);
    };

    tick();
    intervalId = setInterval(async () => {
      if (cancelled) return;
      await refreshUpgradeRequestStatus(requestID);
    }, 5000);

    return () => {
      cancelled = true;
      if (intervalId) {
        clearInterval(intervalId);
      }
    };
  }, [activeUpgradeRequestID, currentOrg?.id]);

  useEffect(() => {
    let mounted = true;
    ensureRazorpay()
      .then(() => {
        if (mounted) {
          ensureRazorpayStyles();
          setUpgradeScriptReady(true);
        }
      })
      .catch(() => {
        if (mounted) {
          setUpgradeScriptReady(false);
        }
      });
    return () => {
      mounted = false;
      cleanupRazorpayOverlay();
    };
  }, []);

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

  const orgScopedRequestOptions = currentOrg?.id
    ? { headers: { 'X-Org-Context': currentOrg.id.toString() } }
    : {};

  const refreshBilling = async () => {
    const emptyOpsResponse: BillingUsageOperationsResponse = { operations: [], limit: 10, offset: 0, count: 0 };
    const [billing, quota, summary, operations] = await Promise.all([
      apiClient.get<BillingStatusResponse>('/billing/status', orgScopedRequestOptions),
      apiClient.get<QuotaStatusResponse>('/quota/status', orgScopedRequestOptions).catch((): null => null),
      apiClient.get<BillingUsageSummaryResponse>('/billing/usage/summary', orgScopedRequestOptions).catch((): null => null),
      apiClient.get<BillingUsageOperationsResponse>('/billing/usage/operations?limit=10&offset=0', orgScopedRequestOptions).catch((): BillingUsageOperationsResponse => emptyOpsResponse),
    ]);
    setBillingStatus(billing);
    setQuotaStatus(quota);
    setUsageSummary(summary);
    setUsageOps(operations.operations || []);
  };

  const refreshUpgradeRequestStatus = async (requestID?: string) => {
    const effectiveRequestID = String(requestID || activeUpgradeRequestID || '').trim();
    if (!effectiveRequestID) {
      setUpgradeRequestStatus(null);
      return;
    }

    setUpgradeRequestLoading(true);
    try {
      const status = await apiClient.get<UpgradeRequestStatusResponse>(
        `/billing/upgrade/request-status?upgrade_request_id=${encodeURIComponent(effectiveRequestID)}`,
        orgScopedRequestOptions
      );
      setUpgradeRequestStatus(status || null);
      setBillingError(null);
    } catch (err: any) {
      setBillingError(err?.message || 'Failed to refresh upgrade request status');
    } finally {
      setUpgradeRequestLoading(false);
    }
  };

  const runBillingAction = async (mode: 'schedule_downgrade' | 'cancel_downgrade') => {
    setActionLoading(true);
    setBillingError(null);
    try {
      if (mode === 'schedule_downgrade') {
        await apiClient.post('/billing/downgrade/schedule', { target_plan_code: selectedDowngradePlan }, orgScopedRequestOptions);
      } else {
        await apiClient.post('/billing/downgrade/cancel', {}, orgScopedRequestOptions);
      }
      await refreshBilling();
    } catch (err: any) {
      setBillingError(err?.message || 'Billing action failed');
    } finally {
      setActionLoading(false);
    }
  };

  const openUpgradePreview = async () => {
    if (!selectedUpgradePlan) return;

    setActionLoading(true);
    setBillingError(null);
    setLastUpgradeResult(null);
    try {
      if (isFree && selectedUpgradePlan === 'team_32usd') {
        navigate('/checkout/team?period=monthly');
        return;
      }

      const preview = await apiClient.post<UpgradePreviewResponse>(
        '/billing/upgrade/preview',
        {
          target_plan_code: selectedUpgradePlan,
        },
        orgScopedRequestOptions
      );

      if (preview?.checkout_required) {
        navigate(preview.checkout_path || '/checkout/team?period=monthly');
        return;
      }

      setUpgradePreview(preview || null);
      setActiveUpgradeRequestID(String(preview?.upgrade_request_id || '').trim());
      if (preview?.upgrade_request_id) {
        void refreshUpgradeRequestStatus(preview.upgrade_request_id);
      }
      setShowUpgradeModal(Boolean(preview?.preview_token));
    } catch (err: any) {
      if (err?.status === 404) {
        setBillingError('Upgrade preview endpoint returned 404. Ensure API is redeployed with /billing/upgrade/preview route and this org context is valid.');
      } else {
        setBillingError(err?.message || 'Failed to preview upgrade charge');
      }
    } finally {
      setActionLoading(false);
    }
  };

  const buildUpgradeExecuteIdempotencyKey = (orderId?: string, requestId?: string) => {
    const orderPart = String(orderId || 'no_order').trim();
    const requestPart = String(requestId || activeUpgradeRequestID || 'no_request').trim();
    return `upgrade_execute_${requestPart}_${orderPart}`;
  };

  const executeUpgradeWithConfirmation = async (
    payment: { orderId?: string; paymentId?: string; signature?: string },
    executeIdempotencyKey: string
  ) => {
    if (!upgradePreview?.preview_token) {
      throw new Error('Missing preview token');
    }

    const requestID = String(upgradePreview?.upgrade_request_id || activeUpgradeRequestID || '').trim();
    if (!requestID) {
      throw new Error('Missing upgrade_request_id');
    }

    const executeResult = await apiClient.post<UpgradeActionResponse>(
      '/billing/upgrade/execute',
      {
        target_plan_code: selectedUpgradePlan,
        upgrade_request_id: requestID,
        preview_token: upgradePreview.preview_token,
        razorpay_order_id: payment.orderId || '',
        razorpay_payment_id: payment.paymentId || '',
        razorpay_signature: payment.signature || '',
        execute_idempotency_key: executeIdempotencyKey,
        modal_version: 'upgrade_proration_v1',
        modal_acknowledged_at: new Date().toISOString(),
      },
      orgScopedRequestOptions
    );

    setBillingError(null);
    setLastUpgradeResult(executeResult || null);
    setActiveUpgradeRequestID(String(executeResult?.upgrade_request_id || requestID).trim());
    setShowUpgradeModal(false);
    setUpgradePreview(null);
    await refreshBilling();
    await refreshUpgradeRequestStatus(String(executeResult?.upgrade_request_id || requestID).trim());
  };

  const confirmUpgrade = async () => {
    if (!upgradePreview?.preview_token) return;

    setUpgradeCheckoutLoading(true);
    setBillingError(null);

    try {
      const prepared = await apiClient.post<PrepareUpgradePaymentResponse>(
        '/billing/upgrade/prepare-payment',
        {
          target_plan_code: selectedUpgradePlan,
          preview_token: upgradePreview.preview_token,
          upgrade_request_id: String(upgradePreview?.upgrade_request_id || activeUpgradeRequestID || '').trim(),
        },
        orgScopedRequestOptions
      );

      const requestID = String(prepared?.upgrade_request_id || upgradePreview?.upgrade_request_id || activeUpgradeRequestID || '').trim();
      if (requestID) {
        setActiveUpgradeRequestID(requestID);
      }

      if (!prepared?.payment_required) {
        await executeUpgradeWithConfirmation({}, buildUpgradeExecuteIdempotencyKey(undefined, requestID));
        setUpgradeCheckoutLoading(false);
        return;
      }

      if (!prepared.order_id || !prepared.razorpay_key_id || typeof prepared.amount_cents !== 'number') {
        throw new Error('Payment preparation response is incomplete');
      }

      if (!upgradeScriptReady) {
        await ensureRazorpay();
        ensureRazorpayStyles();
        setUpgradeScriptReady(true);
      }

      const options = {
        key: prepared.razorpay_key_id,
        order_id: prepared.order_id,
        amount: prepared.amount_cents,
        currency: prepared.currency || 'USD',
        name: 'LiveReview Upgrade',
        description: `Upgrade to ${getPlanDisplayName(selectedUpgradePlan)}`,
        image: '/assets/logo-with-text.svg',
        handler: async (razorpayResponse: any) => {
          try {
            const orderId = razorpayResponse?.razorpay_order_id || prepared.order_id;
            await executeUpgradeWithConfirmation({
              orderId,
              paymentId: razorpayResponse?.razorpay_payment_id,
              signature: razorpayResponse?.razorpay_signature,
            }, buildUpgradeExecuteIdempotencyKey(orderId, requestID));
          } catch (executeErr: any) {
            setBillingError(executeErr?.message || 'Upgrade execution failed after payment');
          } finally {
            setUpgradeCheckoutLoading(false);
            cleanupRazorpayOverlay();
          }
        },
        modal: {
          ondismiss: () => {
            setUpgradeCheckoutLoading(false);
            cleanupRazorpayOverlay();
          },
        },
        theme: {
          color: '#131C2F',
        },
      };

      const rzp = new window.Razorpay(options);
      document.body.classList.add('razorpay-active');
      rzp.on('payment.failed', (response: any) => {
        setBillingError(buildUpgradeCheckoutFailureMessage(response, prepared));
        setUpgradeCheckoutLoading(false);
        cleanupRazorpayOverlay();
      });
      rzp.open();
    } catch (err: any) {
      setBillingError(err?.message || 'Failed to start payment for upgrade');
      setUpgradeCheckoutLoading(false);
      cleanupRazorpayOverlay();
    }
  };

  const formatDate = (dateString: string | null | undefined) => {
    if (!dateString) return null;
    const userTimezone = moment.tz.guess();
    return moment.tz(dateString, userTimezone).format('MMM D, YYYY, h:mm A z');
  };

  const getPlanDisplayName = (plan: string) => {
    const normalized = plan.toLowerCase();
    if (normalized === 'team_32usd') return 'Team 32 USD (100k LOC)';
    if (normalized === 'loc_200k') return 'Team 64 USD (200k LOC)';
    if (normalized === 'loc_400k') return 'Team 128 USD (400k LOC)';
    if (normalized === 'loc_800k') return 'Team 256 USD (800k LOC)';
    if (normalized === 'loc_1600k') return 'Team 512 USD (1.6M LOC)';
    if (normalized === 'loc_3200k') return 'Team 1024 USD (3.2M LOC)';
    if (normalized === 'team' || normalized.includes('team')) return 'Team Plan';
    if (normalized === 'free_30k' || normalized === 'free') return 'Free 30k BYOK';
    return plan;
  };

  const dailyLimit = isFree
    ? 'Quota-based (30,000 LOC/month)'
    : `Quota-based (${(currentPlan?.monthly_loc_limit || 100000).toLocaleString()} LOC/month)`;

  const scheduledPlanCode = billingStatus?.billing?.scheduled_plan_code || '';
  const scheduledPlan = billingStatus?.available_plans?.find((p) => p.plan_code === scheduledPlanCode);
  const currentBillingPlan = billingStatus?.available_plans?.find((p) => p.plan_code === billingStatus?.billing?.current_plan_code);
  const isScheduledUpgrade = Boolean(
    scheduledPlan && currentBillingPlan && scheduledPlan.monthly_loc_limit > currentBillingPlan.monthly_loc_limit
  );
  const cancelScheduledLabel = isScheduledUpgrade ? 'Cancel Scheduled Upgrade' : 'Cancel Scheduled Downgrade';
  const scheduledChangeLabel = isScheduledUpgrade ? 'Scheduled upgrade' : 'Scheduled downgrade';

  const normalizedStatus = status.trim().toLowerCase();
  const statusIsTerminal = ['cancelled', 'expired', 'halted', 'past_due', 'incomplete'].includes(normalizedStatus);
  const statusBadgeLabel = statusLoading
    ? 'LOADING'
    : pendingCancel
    ? 'PENDING EXPIRY'
    : statusIsTerminal
    ? normalizedStatus.replace('_', ' ').toUpperCase()
    : isTeamPlan
    ? 'TEAM ACTIVE'
    : 'FREE PLAN';

  const formatChargeUSD = (cents?: number) => {
    if (typeof cents !== 'number') return null;
    return `$${(cents / 100).toFixed(2)}`;
  };

  const requestStatus = upgradeRequestStatus?.request || null;
  const timelineRows = requestStatus
    ? [
        {
          id: 'request_created',
          label: 'Upgrade request created',
          done: Boolean(requestStatus.created_at),
          at: requestStatus.created_at,
        },
        {
          id: 'order_created',
          label: 'One-time payment order created',
          done: Boolean(requestStatus.razorpay_order_id),
          at: requestStatus.updated_at,
        },
        {
          id: 'payment_confirmed',
          label: 'Payment capture confirmed',
          done: Boolean(requestStatus.payment_capture_confirmed),
          at: requestStatus.payment_capture_confirmed_at,
        },
        {
          id: 'subscription_confirmed',
          label: 'Subscription change confirmed',
          done: Boolean(requestStatus.subscription_change_confirmed),
          at: requestStatus.subscription_change_confirmed_at,
        },
        {
          id: 'resolved',
          label: 'Upgrade resolved and granted',
          done: Boolean(requestStatus.plan_grant_applied),
          at: requestStatus.plan_grant_applied_at || requestStatus.resolved_at,
        },
      ]
    : [];

  const effectiveChargeStatus = (() => {
    const localStatus = String(lastUpgradeResult?.proration?.charge_status || '').trim().toLowerCase();
    if (requestStatus?.payment_capture_confirmed) {
      return 'capture_confirmed';
    }
    if (requestStatus?.plan_grant_applied || String(requestStatus?.status || '').trim().toLowerCase() === 'resolved') {
      return 'resolved';
    }
    return localStatus || 'unknown';
  })();

  const chargeStatusLabel = (() => {
    switch (effectiveChargeStatus) {
      case 'capture_confirmed':
        return 'CAPTURE_CONFIRMED';
      case 'resolved':
        return 'RESOLVED';
      case 'verification_pending':
        return 'VERIFICATION_PENDING';
      default:
        return effectiveChargeStatus.toUpperCase();
    }
  })();

  const chargeSummaryHint = (() => {
    if (requestStatus?.plan_grant_applied || String(requestStatus?.status || '').trim().toLowerCase() === 'resolved') {
      return 'Upgrade completed. Payment and subscription confirmations are done, and LOC grant has been applied.';
    }
    if (requestStatus?.payment_capture_confirmed && !requestStatus?.subscription_change_confirmed) {
      return 'Payment is confirmed. Waiting for subscription change confirmation to finalize grant.';
    }
    if (!requestStatus?.payment_capture_confirmed) {
      return 'Upgrade execution has started. Waiting for payment capture and subscription change confirmations.';
    }
    return 'Upgrade is in progress. Final grant happens after both confirmations complete.';
  })();

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
            <p className="text-sm text-slate-400 mt-1">{getPlanDisplayName(currentPlanCode)}</p>
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
              ) : statusBadgeLabel}
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

          {requestStatus && (
            <div className="bg-slate-900/50 border border-slate-700 rounded-lg p-4 space-y-3">
              <div className="flex items-center justify-between gap-3">
                <p className="text-sm text-slate-200 font-medium">Upgrade request timeline</p>
                <span className="text-xs text-slate-400 font-mono">
                  {requestStatus.upgrade_request_id}
                </span>
              </div>
              <div className="space-y-2">
                {timelineRows.map((row) => (
                  <div key={row.id} className="flex items-center justify-between text-xs border border-slate-700 rounded px-3 py-2 bg-slate-950/50">
                    <div className="flex items-center gap-2">
                      <span className={`inline-flex h-2.5 w-2.5 rounded-full ${row.done ? 'bg-emerald-400' : 'bg-amber-400'}`} />
                      <span className={row.done ? 'text-slate-100' : 'text-slate-300'}>{row.label}</span>
                    </div>
                    <span className="text-slate-400">{row.at ? formatDate(row.at) : 'Pending'}</span>
                  </div>
                ))}
              </div>
              <div className="text-xs text-slate-300 flex items-center justify-between gap-3">
                <span>Current status: <span className="text-white font-semibold uppercase">{requestStatus.status.replace(/_/g, ' ')}</span></span>
                <button
                  type="button"
                  onClick={() => refreshUpgradeRequestStatus(requestStatus.upgrade_request_id)}
                  disabled={upgradeRequestLoading}
                  className="px-2 py-1 rounded border border-slate-600 text-slate-200 hover:bg-slate-800 disabled:opacity-50"
                >
                  {upgradeRequestLoading ? 'Refreshing...' : 'Refresh'}
                </button>
              </div>
            </div>
          )}

          {lastUpgradeResult?.proration && (
            <div className="bg-blue-900/20 border border-blue-500/40 rounded-lg p-4 space-y-2">
              <p className="text-sm text-blue-200 font-medium">Latest Upgrade Charge Summary</p>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-xs text-slate-200">
                <p>From: <span className="text-white">{lastUpgradeResult.proration.from_plan_code || 'n/a'}</span></p>
                <p>To: <span className="text-white">{lastUpgradeResult.proration.to_plan_code || lastUpgradeResult.plan_code || 'n/a'}</span></p>
                <p>Charged now: <span className="text-white">{formatChargeUSD(lastUpgradeResult.proration.charge_amount_cents) || '$0.00'}</span></p>
                <p>Status: <span className="text-white">{chargeStatusLabel}</span></p>
                {typeof lastUpgradeResult.proration.remaining_cycle_fraction === 'number' && (
                  <p>Remaining cycle fraction: <span className="text-white">{(lastUpgradeResult.proration.remaining_cycle_fraction * 100).toFixed(2)}%</span></p>
                )}
                {typeof lastUpgradeResult.proration.immediate_loc_grant === 'number' && (
                  <p>Immediate LOC grant: <span className="text-white">{lastUpgradeResult.proration.immediate_loc_grant.toLocaleString()}</span></p>
                )}
                {lastUpgradeResult.proration.order_id && (
                  <p>Order ID: <span className="text-white font-mono">{lastUpgradeResult.proration.order_id}</span></p>
                )}
                {lastUpgradeResult.proration.payment_id && (
                  <p>Payment ID: <span className="text-white font-mono">{lastUpgradeResult.proration.payment_id}</span></p>
                )}
                {lastUpgradeResult.proration.cycle_end && (
                  <p>Cycle end: <span className="text-white">{formatDate(lastUpgradeResult.proration.cycle_end)}</span></p>
                )}
              </div>
              <p className="text-xs text-slate-300">{chargeSummaryHint}</p>
            </div>
          )}

          {billingError && (
            <div className="bg-red-500/10 border border-red-500/40 rounded-lg p-3 text-sm text-red-200">
              {billingError}
            </div>
          )}

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="p-4 bg-slate-900/70 border border-slate-700 rounded-lg space-y-3">
              <p className="text-sm text-slate-300 font-medium">Upgrade With Proration Preview</p>
              <select
                value={selectedUpgradePlan}
                onChange={(e) => setSelectedUpgradePlan(e.target.value)}
                className="w-full bg-slate-800 border border-slate-600 rounded px-3 py-2 text-sm text-white"
                disabled={actionLoading || upgradeCheckoutLoading}
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
                disabled={actionLoading || upgradeCheckoutLoading || !selectedUpgradePlan}
                onClick={openUpgradePreview}
              >
                Preview Upgrade Charge
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
                  {cancelScheduledLabel}
                </button>
              )}
            </div>
          </div>

          {billingStatus.billing.scheduled_plan_code && (
            <div className="text-sm text-slate-300 bg-slate-900/40 border border-slate-700 rounded-lg p-3">
              {scheduledChangeLabel}: <span className="text-white font-medium">{billingStatus.billing.scheduled_plan_code}</span>
              {billingStatus.billing.scheduled_plan_effective_at && (
                <span> effective {formatDate(billingStatus.billing.scheduled_plan_effective_at)}</span>
              )}
            </div>
          )}
        </div>
      )}

      {showUpgradeModal && upgradePreview?.preview && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/80 p-4">
          <div className="w-full max-w-2xl rounded-xl border border-slate-700 bg-slate-900 shadow-2xl">
            <div className="border-b border-slate-700 px-6 py-4">
              <h3 className="text-lg font-semibold text-white">Confirm Upgrade and Prorated Charge</h3>
              <p className="mt-1 text-sm text-slate-300">
				This charge applies now for the remaining current cycle. Upgrade grant is finalized after deterministic payment and subscription confirmations.
              </p>
            </div>

            <div className="space-y-4 px-6 py-5 text-sm">
              <div className="rounded-lg border border-slate-700 bg-slate-800/70 p-4 space-y-2 text-slate-200">
                <p>Current plan: <span className="text-white">{getPlanDisplayName(upgradePreview.preview.from_plan_code)}</span></p>
                <p>Target plan: <span className="text-white">{getPlanDisplayName(upgradePreview.preview.to_plan_code)}</span></p>
                <p>Current cycle ends: <span className="text-white">{formatDate(upgradePreview.preview.cycle_end)}</span></p>
              </div>

              <div className="rounded-lg border border-blue-500/40 bg-blue-900/20 p-4 space-y-2 text-slate-100">
                <p className="font-medium text-blue-200">Immediate one-time payment</p>
                <p>
                  Charge now: <span className="text-white font-semibold">{formatChargeUSD(upgradePreview.preview.immediate_charge_cents) || '$0.00'}</span>
                  {' '}(remaining cycle {(upgradePreview.preview.remaining_cycle_fraction * 100).toFixed(2)}%)
                </p>
                <p>
                  Formula: <span className="text-white">{formatChargeUSD(upgradePreview.preview.next_cycle_price_cents) || '$0.00'} × {(upgradePreview.preview.remaining_cycle_fraction * 100).toFixed(2)}%</span>
                </p>
                <p>
                  Immediate LOC grant: <span className="text-white font-semibold">{upgradePreview.preview.immediate_loc_grant.toLocaleString()} LOC</span>
                </p>
              </div>

              <div className="rounded-lg border border-emerald-500/40 bg-emerald-900/20 p-4 space-y-2 text-slate-100">
                <p className="font-medium text-emerald-200">Next billing cycle</p>
                <p>
                  Recurring amount: <span className="text-white font-semibold">{formatChargeUSD(upgradePreview.preview.next_cycle_price_cents) || '$0.00'}/month</span>
                </p>
                <p>
                  Monthly LOC limit: <span className="text-white font-semibold">{upgradePreview.preview.next_cycle_loc_limit.toLocaleString()} LOC</span>
                </p>
              </div>

              <div className="rounded-lg border border-amber-500/40 bg-amber-900/20 p-3 text-xs text-amber-100">
				By confirming, you authorize a one-time prorated payment now and start a tracked upgrade process that resolves after all confirmations.
              </div>
            </div>

            <div className="flex items-center justify-end gap-3 border-t border-slate-700 px-6 py-4">
              <button
                type="button"
                disabled={upgradeCheckoutLoading}
                onClick={() => {
                  setShowUpgradeModal(false);
                  setUpgradePreview(null);
                }}
                className="rounded border border-slate-600 px-4 py-2 text-sm text-slate-200 hover:bg-slate-800 disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                type="button"
                disabled={upgradeCheckoutLoading}
                onClick={confirmUpgrade}
                className="rounded bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-500 disabled:opacity-50"
              >
                {upgradeCheckoutLoading ? 'Processing...' : 'Confirm and Pay Now'}
              </button>
            </div>
          </div>
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
