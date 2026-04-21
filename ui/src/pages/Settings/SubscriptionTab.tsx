import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import moment from 'moment-timezone';
import { isCloudMode } from '../../utils/deploymentMode';
import { useOrgContext } from '../../hooks/useOrgContext';
import BillingPortfolio from '../Admin/BillingPortfolio';
import { CancelSubscriptionModal } from '../../components/Subscriptions';
import apiClient from '../../api/apiClient';
import { getSubscriptionStatusLabel, isTerminalSubscriptionStatus } from '../../utils/subscriptionStatus';

type PurchaseCurrency = 'USD' | 'INR';

const SUPPORTED_PURCHASE_CURRENCIES: PurchaseCurrency[] = ['USD', 'INR'];

const normalizePurchaseCurrency = (raw?: string | null): PurchaseCurrency | null => {
  const normalized = String(raw || '').trim().toUpperCase();
  if (normalized === 'USD' || normalized === 'INR') {
    return normalized;
  }
  return null;
};

const fallbackPurchaseCurrencyFromLocale = (): PurchaseCurrency => {
  const locale = String((typeof navigator !== 'undefined' ? navigator.language : '') || '').trim().toUpperCase().replace('_', '-');
  if (locale.includes('-IN')) {
    return 'INR';
  }
  return 'USD';
};

const formatMinorAmount = (amountMinor: number, currency: PurchaseCurrency): string => (
  new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency,
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(amountMinor / 100)
);

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

type TrialEligibilitySummary = {
  status?: 'eligible' | 'already_used' | 'reserved' | 'unknown';
  eligible?: boolean;
  reason?: string;
  consumed_at?: string | null;
  reservation_expires_at?: string | null;
  first_plan_code?: string | null;
  first_org_id?: number | null;
};

type BillingStatusResponse = {
  billing: {
    current_plan_code: string;
    default_purchase_currency?: string;
    supported_purchase_currencies?: string[];
    billing_period_start: string;
    billing_period_end: string;
    loc_used_month: number;
    trial_active?: boolean;
    trial_started_at?: string | null;
    trial_ends_at?: string | null;
    trial_readonly: boolean;
    trial_can_cancel?: boolean;
    trial_eligibility?: TrialEligibilitySummary;
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
    trial_ends_at?: string;
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
    user_id?: number;
    actor_email?: string;
    actor_kind?: string;
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

type BillingUsageMemberSummary = {
  user_id?: number | null;
  actor_email?: string | null;
  actor_kind: string;
  total_billable_loc: number;
  operation_count: number;
  last_accounted_at?: string | null;
  org_total_billable_loc: number;
  usage_share_percent: number;
};

type BillingUsageMembersResponse = {
  members: BillingUsageMemberSummary[];
  limit: number;
  offset: number;
  count: number;
};

type ManagedSubscription = {
  razorpay_subscription_id: string;
  status: string;
  cancel_at_period_end?: boolean;
  current_period_end?: string;
  license_expires_at?: string;
  short_url?: string;
};

const SUBSCRIPTION_TAB_LABELS = {
  overview: 'Plan & Upgrade',
  breakdown: 'Usage',
  assignments: 'Control',
} as const;

type SubscriptionTabKey = 'overview' | 'breakdown' | 'assignments';

type BillingMyUsageResponse = {
  member: BillingUsageMemberSummary;
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

type CreateSubscriptionCheckoutResponse = {
  razorpay_subscription_id: string;
  razorpay_key_id: string;
  trial_applied?: boolean;
  trial_days?: number;
  trial_started_at?: string;
  trial_ends_at?: string;
  plan_unit_amount_minor?: number;
  expected_recurring_amount_minor?: number;
  expected_recurring_currency?: string;
  checkout_authorization_may_apply?: boolean;
  checkout_authorization_note?: string;
};

type KeepPlanActionResult = {
  ok: boolean;
  statusCode?: number;
  errorMessage?: string;
};

type ActionPreflightResult = {
  proceed: boolean;
  routeToBuyNew: boolean;
};

type UpgradeRequestStatusResponse = {
  request: {
    upgrade_request_id: string;
    status: string;
    customer_state?: string;
    action_needed_at?: string;
    action_required?: {
      type?: string;
      endpoint?: string;
      retry_endpoint?: string;
      sla_hours?: number;
      support_sla_business_days?: number;
      delay_minutes?: number;
    };
    latest_payment_error?: {
      code?: string | null;
      reason?: string | null;
      description?: string | null;
    };
    support_reference?: string;
    support_context?: {
      upgrade_request_id?: string;
      razorpay_order_id?: string | null;
      razorpay_payment_id?: string | null;
      razorpay_subscription_id?: string | null;
      dispute_sla_business_days?: number;
    };
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

const buildCheckoutPathWithPlan = (checkoutPath: string | undefined, planCode: string, currency?: PurchaseCurrency): string => {
  const basePath = String(checkoutPath || '/checkout/team?period=monthly').trim();
  const [pathOnly, query = ''] = basePath.split('?');
  const params = new URLSearchParams(query);
  if (!params.get('period')) {
    params.set('period', 'monthly');
  }
  if (!params.get('plan')) {
    params.set('plan', planCode);
  }
  const normalizedCurrency = normalizePurchaseCurrency(currency || '');
  if (normalizedCurrency) {
    params.set('currency', normalizedCurrency);
  }
  return `${pathOnly}?${params.toString()}`;
};

const SubscriptionTab: React.FC = () => {
  const navigate = useNavigate();
  const { currentOrg } = useOrgContext();

  // Initialize tab from URL path
  const getInitialTab = (): SubscriptionTabKey => {
    const hash = window.location.hash;
    if (hash.includes('settings-subscriptions-portfolio')) return 'breakdown';
    if (hash.includes('settings-subscriptions-breakdown')) return 'breakdown';
    if (hash.includes('settings-subscriptions-assign')) return 'assignments';
    return 'overview';
  };

  const [activeTab, setActiveTab] = useState<SubscriptionTabKey>(getInitialTab);

  useEffect(() => {
    // Redirect to license page if in self-hosted mode
    if (!isCloudMode()) {
      navigate('/settings/license', { replace: true });
    }
  }, [navigate]);

  // Update tab when location changes
  useEffect(() => {
    const hash = window.location.hash;
    if (hash.includes('settings-subscriptions-portfolio')) {
      setActiveTab('breakdown');
    } else if (hash.includes('settings-subscriptions-breakdown')) {
      setActiveTab('breakdown');
    } else if (hash.includes('settings-subscriptions-assign')) {
      setActiveTab('assignments');
    } else if (hash.includes('settings-subscriptions-overview')) {
      setActiveTab('overview');
    }
  }, [window.location.hash]);

  const handleTabChange = (tab: SubscriptionTabKey) => {
    setActiveTab(tab);
    const routeMap: Record<SubscriptionTabKey, string> = {
      overview: '/settings-subscriptions-overview',
      breakdown: '/settings-subscriptions-breakdown',
      assignments: '/settings-subscriptions-assign',
    };
    const route = routeMap[tab];
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
            className={`px-4 py-3 text-sm font-medium transition-colors ${activeTab === 'overview'
                ? 'text-white border-b-2 border-blue-500'
                : 'text-slate-400 hover:text-slate-300'
              }`}
          >
            {SUBSCRIPTION_TAB_LABELS.overview}
          </button>
          <button
            onClick={() => handleTabChange('breakdown')}
            className={`px-4 py-3 text-sm font-medium transition-colors ${activeTab === 'breakdown'
                ? 'text-white border-b-2 border-blue-500'
                : 'text-slate-400 hover:text-slate-300'
              }`}
          >
            {SUBSCRIPTION_TAB_LABELS.breakdown}
          </button>
          <button
            onClick={() => handleTabChange('assignments')}
            className={`px-4 py-3 text-sm font-medium transition-colors ${activeTab === 'assignments'
                ? 'text-white border-b-2 border-blue-500'
                : 'text-slate-400 hover:text-slate-300'
              }`}
          >
            {SUBSCRIPTION_TAB_LABELS.assignments}
          </button>
        </div>
      </div>

      {/* Tab Content */}
      {activeTab === 'overview' ? (
        <OverviewTab navigate={navigate} mode="full" />
      ) : activeTab === 'breakdown' ? (
        <OverviewTab navigate={navigate} mode="breakdown" />
      ) : (
        <OverviewTab navigate={navigate} mode="controls" />
      )}
    </div>
  );
};

// Overview Tab Component
const OverviewTab: React.FC<{ navigate: any; mode?: 'full' | 'breakdown' | 'controls' }> = ({ navigate, mode = 'full' }) => {
  const { currentOrg, isSuperAdmin } = useOrgContext();
  const isBreakdownMode = mode === 'breakdown';
  const isControlsMode = mode === 'controls';
  const isPlanUpgradeMode = mode === 'full';
  const [showCancelModal, setShowCancelModal] = useState(false);
  const [cancelImmediate, setCancelImmediate] = useState(false);
  const [subscriptionId, setSubscriptionId] = useState<string | null>(null);
  const [managedSubscription, setManagedSubscription] = useState<ManagedSubscription | null>(null);
  const [pendingCancel, setPendingCancel] = useState(false);
  const [subscriptionManageURL, setSubscriptionManageURL] = useState('');
  const [status, setStatus] = useState<string>('');
  const [statusLoading, setStatusLoading] = useState(false);
  const [subscriptionLoading, setSubscriptionLoading] = useState(false);
  const [billingLoading, setBillingLoading] = useState(true);
  const [displayExpiry, setDisplayExpiry] = useState<string | null>(null);
  const [billingStatus, setBillingStatus] = useState<BillingStatusResponse | null>(null);
  const [quotaStatus, setQuotaStatus] = useState<QuotaStatusResponse | null>(null);
  const [billingError, setBillingError] = useState<string | null>(null);
  const [keepPlanError, setKeepPlanError] = useState<string | null>(null);
  const [keepPlanSuccess, setKeepPlanSuccess] = useState<string | null>(null);
  const [keepPlanLoading, setKeepPlanLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState(false);
  const [actionProgressMessage, setActionProgressMessage] = useState<string | null>(null);
  const [selectedUpgradePlan, setSelectedUpgradePlan] = useState('');
  const [selectedDowngradePlan, setSelectedDowngradePlan] = useState('');
  const [purchaseCurrency, setPurchaseCurrency] = useState<PurchaseCurrency>(fallbackPurchaseCurrencyFromLocale());
  const [purchaseCurrencyInitialized, setPurchaseCurrencyInitialized] = useState(false);
  const [usageSummary, setUsageSummary] = useState<BillingUsageSummaryResponse | null>(null);
  const [usageOps, setUsageOps] = useState<BillingUsageOperationsResponse['operations']>([]);
  const [myUsage, setMyUsage] = useState<BillingUsageMemberSummary | null>(null);
  const [usageMembers, setUsageMembers] = useState<BillingUsageMemberSummary[]>([]);
  const [lastUpgradeResult, setLastUpgradeResult] = useState<UpgradeActionResponse | null>(null);
  const [upgradePreview, setUpgradePreview] = useState<UpgradePreviewResponse | null>(null);
  const [showUpgradeModal, setShowUpgradeModal] = useState(false);
  const [showFreeCheckoutModal, setShowFreeCheckoutModal] = useState(false);
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

  const refreshSubscriptionState = useCallback(async () => {
    if (!currentOrg?.id) {
      setStatusLoading(false);
      setSubscriptionLoading(false);
      setSubscriptionId(null);
      setManagedSubscription(null);
      setPendingCancel(false);
      setStatus('');
      return;
    }

    setStatusLoading(true);
    setSubscriptionLoading(true);
    setPendingCancel(false);
    setStatus('');

    let currentSubscriptionData: any = null;
    const currentSubscriptionPromise = apiClient
      .get('/subscriptions/current', {
        headers: {
          'X-Org-Context': currentOrg.id.toString(),
        },
      })
      .then((data: any) => {
        currentSubscriptionData = data || null;
      })
      .catch(() => {
        currentSubscriptionData = null;
      });

    let managedSubscriptions: ManagedSubscription[] = [];
    const managedSubscriptionsPromise = apiClient
      .get<{ subscriptions: ManagedSubscription[] }>('/subscriptions', {
        headers: {
          'X-Org-Context': currentOrg.id.toString(),
        },
      })
      .then((response) => {
        managedSubscriptions = Array.isArray(response?.subscriptions) ? response.subscriptions : [];
      })
      .catch(() => {
        managedSubscriptions = [];
      });

    try {
      await Promise.all([currentSubscriptionPromise, managedSubscriptionsPromise]);

      if (currentSubscriptionData) {
        setSubscriptionId(currentSubscriptionData.subscription_id || null);
        setPendingCancel(Boolean(currentSubscriptionData.cancel_at_period_end));
        setSubscriptionManageURL(String(currentSubscriptionData.short_url || ''));
        setStatus(currentSubscriptionData.status || '');
        const expirySrc = currentSubscriptionData.cancel_at_period_end
          ? currentSubscriptionData.current_period_end
          : currentSubscriptionData.license_expires_at;
        setDisplayExpiry(expirySrc || licenseExpiresAt || null);
      } else {
        setSubscriptionId(null);
        setPendingCancel(false);
        setSubscriptionManageURL('');
        setStatus('');
      }

      const score = (item: ManagedSubscription) => {
        const normalized = String(item.status || '').toLowerCase();
        let base = 3;
        if (normalized === 'active') base = 0;
        else if (normalized === 'authenticated' || normalized === 'created') base = 1;
        else if (normalized === 'halted') base = 2;

        // Strongly prefer non-cancelled subscriptions in control mode fallback selection.
        const cancelPenalty = item.cancel_at_period_end ? 10 : 0;
        return base + cancelPenalty;
      };
      const prioritized = [...managedSubscriptions].sort((a, b) => score(a) - score(b));

      let selectedManaged: ManagedSubscription | null = null;
      const currentSubscriptionID = String(currentSubscriptionData?.subscription_id || '').trim();
      if (currentSubscriptionID !== '') {
        selectedManaged = prioritized.find(
          (item) => String(item.razorpay_subscription_id || '').trim() === currentSubscriptionID,
        ) || null;
      }
      if (!selectedManaged) {
        selectedManaged = prioritized[0] || null;
      }
      setManagedSubscription(selectedManaged);
    } finally {
      setStatusLoading(false);
      setSubscriptionLoading(false);
    }
  }, [currentOrg?.id, licenseExpiresAt]);

  // Fetch current subscription (org-scoped)
  useEffect(() => {
    void refreshSubscriptionState();
  }, [refreshSubscriptionState]);

  useEffect(() => {
    if (!currentOrg?.id) return;
    setBillingStatus(null);
    setQuotaStatus(null);
    setBillingLoading(true);
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

  useEffect(() => {
    setPurchaseCurrency(fallbackPurchaseCurrencyFromLocale());
    setPurchaseCurrencyInitialized(false);
  }, [currentOrg?.id]);

  useEffect(() => {
    if (purchaseCurrencyInitialized || !billingStatus?.billing) return;
    const backendDefault = normalizePurchaseCurrency(billingStatus.billing.default_purchase_currency);
    if (backendDefault) {
      setPurchaseCurrency(backendDefault);
    }
    setPurchaseCurrencyInitialized(true);
  }, [billingStatus?.billing, purchaseCurrencyInitialized]);

  const handleCancelSuccess = () => {
    // Reload the page to reflect updated subscription status
    window.location.reload();
  };

  const orgScopedRequestOptions = useMemo(
    () => (currentOrg?.id ? { headers: { 'X-Org-Context': currentOrg.id.toString() } } : {}),
    [currentOrg?.id]
  );

  const refreshBilling = useCallback(async () => {
    setBillingLoading(true);
    const emptyOpsResponse: BillingUsageOperationsResponse = { operations: [], limit: 10, offset: 0, count: 0 };
    const emptyMembersResponse: BillingUsageMembersResponse = { members: [], limit: 10, offset: 0, count: 0 };
    try {
      const [billing, quota, summary, operations, meUsage, members] = await Promise.all([
        apiClient.get<BillingStatusResponse>('/billing/status', orgScopedRequestOptions),
        apiClient.get<QuotaStatusResponse>('/quota/status', orgScopedRequestOptions).catch((): null => null),
        apiClient.get<BillingUsageSummaryResponse>('/billing/usage/summary', orgScopedRequestOptions).catch((): null => null),
        apiClient.get<BillingUsageOperationsResponse>('/billing/usage/operations?limit=10&offset=0', orgScopedRequestOptions).catch((): BillingUsageOperationsResponse => emptyOpsResponse),
        apiClient.get<BillingMyUsageResponse>('/billing/usage/me', orgScopedRequestOptions).catch((): null => null),
        canManageBilling
          ? apiClient.get<BillingUsageMembersResponse>('/billing/usage/members?limit=10&offset=0', orgScopedRequestOptions).catch((): BillingUsageMembersResponse => emptyMembersResponse)
          : Promise.resolve(emptyMembersResponse),
      ]);
      setBillingStatus(billing);
      setQuotaStatus(quota);
      setUsageSummary(summary);
      setUsageOps(operations.operations || []);
      setMyUsage(meUsage?.member || null);
      setUsageMembers(members.members || []);
    } finally {
      setBillingLoading(false);
    }
  }, [canManageBilling, orgScopedRequestOptions]);

  useEffect(() => {
    if (!currentOrg?.id) return;

    const intervalId = window.setInterval(() => {
      void refreshSubscriptionState();
      void refreshBilling();
    }, 60_000);

    return () => {
      window.clearInterval(intervalId);
    };
  }, [currentOrg?.id, refreshBilling, refreshSubscriptionState]);

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

  const performKeepPlanAction = async (opts?: { suppressSuccessMessage?: boolean }): Promise<KeepPlanActionResult> => {
    if (!effectiveSubscriptionId) {
      return {
        ok: false,
        errorMessage: 'No active subscription available to keep.',
      };
    }

    setKeepPlanLoading(true);
    setKeepPlanError(null);
    if (!opts?.suppressSuccessMessage) {
      setKeepPlanSuccess(null);
    }

    try {
      await apiClient.post(
        `/subscriptions/${effectiveSubscriptionId}/keep-plan`,
        {},
        orgScopedRequestOptions
      );

      let refreshFailed = false;
      try {
        await refreshSubscriptionState();
      } catch {
        refreshFailed = true;
      }
      try {
        await refreshBilling();
      } catch {
        refreshFailed = true;
      }

      if (refreshFailed) {
        return {
          ok: false,
          errorMessage: 'Keep plan was applied, but billing data refresh failed. Please reload this page.',
        };
      }

      if (!opts?.suppressSuccessMessage) {
        setKeepPlanSuccess('Scheduled cancellation removed. Your current plan remains active.');
      }

      return { ok: true };
    } catch (err: any) {
      return {
        ok: false,
        statusCode: typeof err?.status === 'number' ? err.status : undefined,
        errorMessage: err?.message || 'Failed to keep current plan',
      };
    } finally {
      setKeepPlanLoading(false);
    }
  };

  const runActionPreflight = async (action: 'upgrade' | 'schedule_downgrade'): Promise<ActionPreflightResult> => {
    setKeepPlanError(null);
    setKeepPlanSuccess(null);

    if (effectiveTerminalCancellation) {
      if (action === 'upgrade') {
        setActionProgressMessage('Current subscription is already ended. Starting a new purchase flow.');
        return { proceed: false, routeToBuyNew: true };
      }

      setActionProgressMessage(null);
      setBillingError('Subscription is already cancelled or expired, so downgrade scheduling is unavailable.');
      return { proceed: false, routeToBuyNew: false };
    }

    if (!effectivePendingCancel) {
      return { proceed: true, routeToBuyNew: false };
    }

    if (action === 'upgrade') {
      return { proceed: true, routeToBuyNew: false };
    }

    if (!effectiveSubscriptionId) {
      setActionProgressMessage(null);
      setBillingError('No active subscription found for downgrade scheduling.');
      return { proceed: false, routeToBuyNew: false };
    }

    setActionProgressMessage('Reactivating current plan before downgrade...');

    const keepPlanResult = await performKeepPlanAction({ suppressSuccessMessage: true });
    if (keepPlanResult.ok) {
      setActionProgressMessage(null);
      return { proceed: true, routeToBuyNew: false };
    }

    if (keepPlanResult.statusCode === 409) {
      setActionProgressMessage('Keep-plan confirmation is still propagating. Continuing with downgrade flow.');
      return { proceed: true, routeToBuyNew: false };
    }

    setActionProgressMessage(null);
    setBillingError(keepPlanResult.errorMessage || 'Failed to reactivate current plan before continuing.');
    return { proceed: false, routeToBuyNew: false };
  };

  const runBillingAction = async (mode: 'schedule_downgrade' | 'cancel_downgrade', targetPlanCode?: string) => {
    setActionLoading(true);
    setBillingError(null);
    setActionProgressMessage(null);
    try {
      if (mode === 'schedule_downgrade') {
        const preflight = await runActionPreflight('schedule_downgrade');
        if (!preflight.proceed) {
          return;
        }

        const effectivePlanCode = String(targetPlanCode || selectedDowngradePlan || '').trim();
        if (!effectivePlanCode) {
          throw new Error('Select a lower plan to continue.');
        }
        await apiClient.post('/billing/downgrade/schedule', { target_plan_code: effectivePlanCode }, orgScopedRequestOptions);
      } else {
        await apiClient.post('/billing/downgrade/cancel', {}, orgScopedRequestOptions);
      }
      await refreshBilling();
    } catch (err: any) {
      setBillingError(err?.message || 'Billing action failed');
    } finally {
      setActionLoading(false);
      setActionProgressMessage(null);
    }
  };

  const handleKeepPlan = async () => {
    setActionProgressMessage(null);
    setKeepPlanError(null);
    setKeepPlanSuccess(null);

    const keepPlanResult = await performKeepPlanAction();
    if (!keepPlanResult.ok) {
      setKeepPlanError(keepPlanResult.errorMessage || 'Failed to keep current plan');
    }
  };

  const openUpgradePreview = async (targetPlanCode?: string, currencyOverride?: PurchaseCurrency) => {
    const effectivePlanCode = String(targetPlanCode || selectedUpgradePlan || '').trim();
    if (!effectivePlanCode) return;
    const effectiveCurrency = currencyOverride || purchaseCurrency;

    setSelectedUpgradePlan(effectivePlanCode);

    setActionLoading(true);
    setBillingError(null);
    setActionProgressMessage(null);
    setLastUpgradeResult(null);
    try {
      const preflight = await runActionPreflight('upgrade');
      if (preflight.routeToBuyNew) {
        setShowUpgradeModal(false);
        setUpgradePreview(null);
        setShowFreeCheckoutModal(true);
        return;
      }
      if (!preflight.proceed) {
        return;
      }

      if (isFree) {
        setShowUpgradeModal(false);
        setUpgradePreview(null);
        setShowFreeCheckoutModal(true);
        return;
      }

      const preview = await apiClient.post<UpgradePreviewResponse>(
        '/billing/upgrade/preview',
        {
          target_plan_code: effectivePlanCode,
          currency: effectiveCurrency,
        },
        orgScopedRequestOptions
      );

      if (preview?.checkout_required) {
        navigate(buildCheckoutPathWithPlan(preview.checkout_path, effectivePlanCode, effectiveCurrency));
        return;
      }

      setUpgradePreview(preview || null);
      setActiveUpgradeRequestID(String(preview?.upgrade_request_id || '').trim());
      if (preview?.upgrade_request_id) {
        void refreshUpgradeRequestStatus(preview.upgrade_request_id);
      }
      setShowUpgradeModal(Boolean(preview?.preview_token));
    } catch (err: any) {
      if (err?.status === 409 && err?.data?.checkout_required) {
        navigate(buildCheckoutPathWithPlan(err?.data?.checkout_path, effectivePlanCode, effectiveCurrency));
        return;
      }
      if (err?.status === 404) {
        setBillingError('Upgrade endpoint returned 404. Ensure API is redeployed with /billing/upgrade/preview route and this org context is valid.');
      } else {
        setBillingError(err?.message || 'Failed to prepare upgrade charge');
      }
    } finally {
      setActionLoading(false);
      setActionProgressMessage(null);
    }
  };

  const startFreeCheckoutFromSettings = async () => {
    const effectivePlanCode = String(selectedUpgradePlan || '').trim();
    if (!effectivePlanCode) {
      setBillingError('Select an upgrade plan to continue.');
      return;
    }

    setUpgradeCheckoutLoading(true);
    setBillingError(null);

    try {
      if (!upgradeScriptReady) {
        await ensureRazorpay();
        ensureRazorpayStyles();
        setUpgradeScriptReady(true);
      }

      const created = await apiClient.post<CreateSubscriptionCheckoutResponse>(
        '/subscriptions',
        { plan_code: effectivePlanCode, currency: purchaseCurrency },
        orgScopedRequestOptions
      );

      if (!created?.razorpay_subscription_id || !created?.razorpay_key_id) {
        throw new Error('Checkout initialization is incomplete. Please retry.');
      }

      const selectedPlanDetails = billingStatus?.available_plans?.find((plan) => plan.plan_code === effectivePlanCode);
      const locLabel = selectedPlanDetails?.monthly_loc_limit
        ? `${selectedPlanDetails.monthly_loc_limit.toLocaleString()} LOC/month`
        : 'LOC/month';
      const expectedRecurringCurrency = normalizePurchaseCurrency(created?.expected_recurring_currency || '') || purchaseCurrency;
      const expectedRecurringAmountText = typeof created?.expected_recurring_amount_minor === 'number'
        ? `${formatMinorAmount(created.expected_recurring_amount_minor, expectedRecurringCurrency)}/month`
        : null;
      const checkoutDescription = created?.trial_applied && Number(created?.trial_days || 0) > 0
        ? `${getPlanDisplayName(effectivePlanCode)} (${locLabel}) - ${created.trial_days}-day trial included`
        : `${getPlanDisplayName(effectivePlanCode)} (${locLabel})`;

      if (created?.trial_applied && created?.trial_ends_at) {
        setActionProgressMessage(
          `Trial will activate after confirmation and run until ${formatDate(created.trial_ends_at) || created.trial_ends_at}. ` +
          `${expectedRecurringAmountText ? `Recurring billing starts at ${expectedRecurringAmountText}. ` : ''}` +
          `${created?.checkout_authorization_note || 'Razorpay may show a small setup authorization amount while trial is active.'}`
        );
      } else if (created?.trial_applied && Number(created?.trial_days || 0) > 0) {
        setActionProgressMessage(
          `${created.trial_days}-day trial will activate after payment confirmation. ` +
          `${expectedRecurringAmountText ? `Recurring billing starts at ${expectedRecurringAmountText}. ` : ''}` +
          `${created?.checkout_authorization_note || 'Razorpay may show a small setup authorization amount while trial is active.'}`
        );
      } else {
        setActionProgressMessage(
          `Checkout initialized. ${expectedRecurringAmountText ? `Recurring billing is ${expectedRecurringAmountText}. ` : ''}` +
          'Billing starts after payment confirmation.'
        );
      }

      const options = {
        key: created.razorpay_key_id,
        subscription_id: created.razorpay_subscription_id,
        name: 'LiveReview LOC Plan',
        description: checkoutDescription,
        image: '/assets/logo-with-text.svg',
        handler: async (razorpayResponse: any) => {
          try {
            const paymentID = razorpayResponse?.razorpay_payment_id;
            const signature = razorpayResponse?.razorpay_signature;
            if (!paymentID || !signature) {
              throw new Error('Payment confirmation payload is incomplete.');
            }

            await apiClient.post(
              '/subscriptions/confirm-purchase',
              {
                razorpay_subscription_id: created.razorpay_subscription_id,
                razorpay_payment_id: paymentID,
                razorpay_signature: signature,
              },
              orgScopedRequestOptions
            );

            setShowFreeCheckoutModal(false);
            await refreshBilling();
          } catch (confirmErr: any) {
            setBillingError(confirmErr?.message || 'Payment completed, but confirmation failed. Please retry.');
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
        setBillingError(
          buildUpgradeCheckoutFailureMessage(response, {
            payment_required: true,
            razorpay_key_id: created.razorpay_key_id,
            preview_token: '',
            currency: purchaseCurrency,
          })
        );
        setUpgradeCheckoutLoading(false);
        cleanupRazorpayOverlay();
      });
      rzp.open();
    } catch (err: any) {
      setBillingError(err?.message || 'Failed to start checkout for selected plan');
      setUpgradeCheckoutLoading(false);
      cleanupRazorpayOverlay();
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

  const getTrialDaysRemaining = (trialEndISO: string | null | undefined): number | null => {
    const raw = String(trialEndISO || '').trim();
    if (!raw) {
      return null;
    }
    const trialEnd = moment(raw);
    if (!trialEnd.isValid()) {
      return null;
    }
    const now = moment();
    if (!trialEnd.isAfter(now)) {
      return 0;
    }
    return Math.max(1, Math.ceil((trialEnd.valueOf() - now.valueOf()) / (24 * 60 * 60 * 1000)));
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

  const selectedFreeCheckoutPlan = billingStatus?.available_plans?.find((p) => p.plan_code === selectedUpgradePlan) || null;
  const selectedFreeCheckoutPriceUSD = selectedFreeCheckoutPlan?.monthly_price_usd || 0;
  const selectedFreeCheckoutLoc = selectedFreeCheckoutPlan?.monthly_loc_limit || 0;
  const availablePurchaseCurrencies = useMemo(() => {
    const backendCurrencies = (billingStatus?.billing?.supported_purchase_currencies || [])
      .map((raw) => normalizePurchaseCurrency(raw))
      .filter((value): value is PurchaseCurrency => value !== null);
    if (backendCurrencies.length === 0) {
      return SUPPORTED_PURCHASE_CURRENCIES;
    }
    return Array.from(new Set(backendCurrencies));
  }, [billingStatus?.billing?.supported_purchase_currencies]);

  useEffect(() => {
    if (availablePurchaseCurrencies.length === 0) {
      return;
    }
    if (!availablePurchaseCurrencies.includes(purchaseCurrency)) {
      setPurchaseCurrency(availablePurchaseCurrencies[0]);
    }
  }, [availablePurchaseCurrencies, purchaseCurrency]);

  const dailyLimit = isFree
    ? 'Quota-based (30,000 LOC/month)'
    : `Quota-based (${(currentPlan?.monthly_loc_limit || 100000).toLocaleString()} LOC/month)`;

  const currentLocLimit = currentPlan?.monthly_loc_limit || (isFree ? 30000 : 100000);
  const currentLocUsed = billingStatus?.billing?.loc_used_month || 0;
  const currentLocRemaining = Math.max(0, currentLocLimit - currentLocUsed);
  const currentLocUsagePercent = currentLocLimit > 0 ? Math.min(100, Math.round((currentLocUsed / currentLocLimit) * 100)) : 0;
  const trialActive = Boolean(billingStatus?.billing?.trial_active);
  const trialStartsAt = billingStatus?.billing?.trial_started_at || null;
  const trialEndsAt = billingStatus?.billing?.trial_ends_at || quotaStatus?.envelope?.trial_ends_at || null;
  const trialDaysRemaining = getTrialDaysRemaining(trialEndsAt);
  const trialEligibility = billingStatus?.billing?.trial_eligibility;
  const trialEligibilityStatus = String(trialEligibility?.status || 'unknown').trim().toLowerCase();
  const trialEligibleForFirstPaidPurchase = Boolean(trialEligibility?.eligible);
  const sortedPlansByLoc = [...(billingStatus?.available_plans || [])].sort((a, b) => a.monthly_loc_limit - b.monthly_loc_limit);
  const maxPaidTrialDays = sortedPlansByLoc.reduce((max, plan) => {
    if (plan.monthly_price_usd <= 0) {
      return max;
    }
    return Math.max(max, Number(plan.trial_days || 0));
  }, 0);
  const firstPaidTrialDays = maxPaidTrialDays > 0 ? maxPaidTrialDays : 7;
  const selectedFreeCheckoutTrialDays = Number(selectedFreeCheckoutPlan?.trial_days || 0);
  const freeCheckoutTrialMessage = (() => {
    if (selectedFreeCheckoutTrialDays <= 0) {
      return 'This paid plan has no trial period configured. Billing starts immediately after payment confirmation.';
    }
    if (trialEligibleForFirstPaidPurchase) {
      return `Your first paid purchase is eligible for a ${selectedFreeCheckoutTrialDays}-day trial. Billing starts when that trial window ends.`;
    }
    if (trialEligibilityStatus === 'already_used') {
      return 'This email already used its one-time first-paid trial. Billing starts immediately after payment confirmation.';
    }
    if (trialEligibilityStatus === 'reserved') {
      return 'A trial reservation is currently in progress for this email. Complete checkout now to claim it.';
    }
    return `Trial eligibility is verified at checkout. This plan supports up to ${selectedFreeCheckoutTrialDays} trial days for first paid purchases.`;
  })();
  const currentPlanIndex = sortedPlansByLoc.findIndex((plan) => plan.plan_code === currentPlanCode);
  const upgradeHierarchyPlans = currentPlanIndex >= 0 ? sortedPlansByLoc.slice(currentPlanIndex) : [];
  const upgradeOptions = upgradeHierarchyPlans.filter((plan) => plan.plan_code !== currentPlanCode);
  const downgradeOptions = currentPlanIndex > 0 ? sortedPlansByLoc.slice(0, currentPlanIndex).reverse() : [];
  const hasCurrentSubscription = Boolean(subscriptionId);
  const normalizedCurrentSubscriptionID = String(subscriptionId || '').trim();
  const normalizedManagedSubscriptionID = String(managedSubscription?.razorpay_subscription_id || '').trim();
  const managedMatchesCurrentSubscription = Boolean(
    hasCurrentSubscription &&
    normalizedCurrentSubscriptionID !== '' &&
    normalizedCurrentSubscriptionID === normalizedManagedSubscriptionID,
  );
  const allowManagedFallback = Boolean(isControlsMode && (!hasCurrentSubscription || managedMatchesCurrentSubscription));
  const effectiveSubscriptionId = hasCurrentSubscription
    ? subscriptionId
    : allowManagedFallback
      ? managedSubscription?.razorpay_subscription_id || null
      : null;
  const effectivePendingCancel = hasCurrentSubscription
    ? pendingCancel
    : allowManagedFallback && Boolean(managedSubscription?.cancel_at_period_end);
  const effectiveSubscriptionManageURL = subscriptionManageURL || (allowManagedFallback ? String(managedSubscription?.short_url || '') : '');
  const normalizedCurrentStatus = String(status || '').trim().toLowerCase();
  const normalizedManagedStatus = allowManagedFallback ? String(managedSubscription?.status || '').trim().toLowerCase() : '';
  const effectiveTerminalCancellation =
    isTerminalSubscriptionStatus(normalizedCurrentStatus) ||
    (!hasCurrentSubscription && allowManagedFallback && isTerminalSubscriptionStatus(normalizedManagedStatus));
  const currentCanCancel =
    hasCurrentSubscription &&
    !pendingCancel &&
    (!normalizedCurrentStatus || normalizedCurrentStatus === 'active' || normalizedCurrentStatus === 'authenticated');
  const managedCanCancel =
    !hasCurrentSubscription &&
    allowManagedFallback &&
    Boolean(managedSubscription?.razorpay_subscription_id) &&
    !Boolean(managedSubscription?.cancel_at_period_end) &&
    (!normalizedManagedStatus || normalizedManagedStatus === 'active' || normalizedManagedStatus === 'authenticated');
  const cancelTargetSubscriptionId = currentCanCancel
    ? subscriptionId
    : managedCanCancel
      ? managedSubscription?.razorpay_subscription_id || null
      : null;
  const canCancelEffectiveSubscription = Boolean(cancelTargetSubscriptionId);
  const canKeepEffectivePlan = Boolean(isControlsMode && effectivePendingCancel && effectiveSubscriptionId);
  const trialCanCancel = Boolean(
    canManageBilling &&
    billingStatus?.billing?.trial_can_cancel &&
    cancelTargetSubscriptionId &&
    !effectivePendingCancel &&
    !effectiveTerminalCancellation
  );

  const scheduledPlanCode = billingStatus?.billing?.scheduled_plan_code || '';
  const scheduledPlan = billingStatus?.available_plans?.find((p) => p.plan_code === scheduledPlanCode);
  const currentBillingPlan = billingStatus?.available_plans?.find((p) => p.plan_code === billingStatus?.billing?.current_plan_code);
  const isScheduledUpgrade = Boolean(
    scheduledPlan && currentBillingPlan && scheduledPlan.monthly_loc_limit > currentBillingPlan.monthly_loc_limit
  );
  const cancelScheduledLabel = isScheduledUpgrade ? 'Cancel Scheduled Upgrade' : 'Cancel Scheduled Downgrade';
  const scheduledChangeLabel = isScheduledUpgrade ? 'Scheduled upgrade' : 'Scheduled downgrade';
  const hasScheduledPlanChange = Boolean(scheduledPlanCode && scheduledPlanCode !== currentPlanCode && scheduledPlan);
  const scheduledChangeTargetLabel = hasScheduledPlanChange ? getPlanDisplayName(scheduledPlanCode) : '';
  const effectivePendingExpiry =
    displayExpiry ||
    ((!hasCurrentSubscription && allowManagedFallback) ? managedSubscription?.current_period_end : null) ||
    licenseExpiresAt ||
    null;
  const pendingExpiryElapsed = Boolean(
    effectivePendingExpiry && moment(effectivePendingExpiry).isValid() && moment(effectivePendingExpiry).isBefore(moment())
  );

  const normalizedStatus = status.trim().toLowerCase();
  const autoDowngradedToFree = !isTeamPlan && !effectivePendingCancel && (
    normalizedStatus === 'expired' ||
    (!hasCurrentSubscription && allowManagedFallback && normalizedManagedStatus === 'expired') ||
    pendingExpiryElapsed
  );
  const statusBadgeLabel = getSubscriptionStatusLabel({
    status,
    pendingCancel: effectivePendingCancel,
    statusLoading,
    trialActive,
    isTeamPlan,
    autoDowngradedToFree,
  });

  const openCancelDialog = (immediate: boolean) => {
    setCancelImmediate(immediate);
    setShowCancelModal(true);
  };

  const formatChargeAmount = (amountMinor?: number, rawCurrency?: string | null) => {
    if (typeof amountMinor !== 'number') return null;
    const normalizedCurrency = normalizePurchaseCurrency(rawCurrency) || purchaseCurrency;
    return formatMinorAmount(amountMinor, normalizedCurrency);
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
        {isBreakdownMode ? (
          <>
            <h2 className="text-lg font-semibold text-white mb-2">Usage</h2>
            <p className="text-sm text-slate-400 mb-4">
              Track your usage by member and operation for the current billing period.
            </p>
          </>
        ) : isControlsMode ? (
          <>
            <h2 className="text-lg font-semibold text-white mb-2">Subscription Control</h2>
            <p className="text-sm text-slate-400 mb-4">
              Manage advanced access assignment, payment link, cancellation, and downgrade.
            </p>
          </>
        ) : (
          <>
            <h2 className="text-lg font-semibold text-white mb-2">Plan and Upgrade</h2>
            <p className="text-sm text-slate-400 mb-4">
              See your current plan, understand LOC headroom, and upgrade quickly.
            </p>
          </>
        )}
      </div>

      {isPlanUpgradeMode && (
        <div className="bg-slate-800/50 border border-slate-700 rounded-lg p-4">
          <h3 className="text-sm font-semibold text-white mb-2">AI Execution</h3>
          <div className="space-y-1 text-sm text-slate-300">
            <p><span className="text-slate-400">Free plan:</span> Bring your own AI key (BYOK) is required.</p>
            <p><span className="text-slate-400">Paid LOC plans:</span> Hosted Auto is enabled by default, and BYOK remains optional.</p>
          </div>
        </div>
      )}

      {isPlanUpgradeMode && (billingStatus?.billing?.trial_readonly || quotaStatus?.envelope?.trial_readonly) && (
        <div className="bg-amber-500/10 border border-amber-400/40 rounded-lg p-4">
          <p className="text-amber-200 text-sm font-medium">Trial is now read-only</p>
          <p className="text-amber-100/90 text-sm mt-1">
            Review creation is blocked until the organization is moved to a paid LOC plan.
          </p>
        </div>
      )}

      {isPlanUpgradeMode && (quotaStatus?.envelope?.blocked || quotaStatus?.can_trigger_reviews === false) && (
        <div className="bg-red-500/10 border border-red-400/40 rounded-lg p-4">
          <p className="text-red-200 text-sm font-medium">⛔ Monthly LOC Quota Exceeded</p>
          <p className="text-red-100/90 text-sm mt-1">
            You've used {currentLocUsed.toLocaleString()} of {currentLocLimit.toLocaleString()} LOC this month. Reviews are blocked until your quota resets{billingStatus?.billing?.billing_period_end ? ` on ${new Date(billingStatus.billing.billing_period_end).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })}` : ''} or you upgrade your plan.
          </p>
          <button
            onClick={() => navigate('/subscribe')}
            className="mt-2 px-3 py-1.5 bg-red-600 hover:bg-red-500 text-white text-xs font-semibold rounded transition-colors"
          >
            Upgrade Now
          </button>
        </div>
      )}

      {isPlanUpgradeMode && autoDowngradedToFree && (
        <div className="bg-sky-500/10 border border-sky-400/40 rounded-lg p-4">
          <p className="text-sky-200 text-sm font-medium">Your paid plan ended and was automatically downgraded to Free</p>
          <p className="text-sky-100/90 text-sm mt-1">
            You can keep using free-plan features without interruption. Renew anytime from the upgrade options below to restore paid capacity.
          </p>
        </div>
      )}

      {/* Current Plan Section */}
      {isPlanUpgradeMode && billingLoading && (
        <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-6 animate-pulse">
          <div className="flex items-center justify-between mb-4">
            <div className="space-y-2">
              <div className="h-5 w-28 bg-slate-700 rounded" />
              <div className="h-4 w-40 bg-slate-700 rounded" />
            </div>
            <div className="h-9 w-28 bg-slate-700 rounded-lg" />
          </div>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3 mb-4">
            <div className="h-16 bg-slate-900/70 border border-slate-700 rounded" />
            <div className="h-16 bg-slate-900/70 border border-slate-700 rounded" />
            <div className="h-16 bg-slate-900/70 border border-slate-700 rounded" />
          </div>
          <div className="h-2 bg-slate-900 border border-slate-700 rounded mb-4" />
          <div className="space-y-3">
            <div className="h-4 bg-slate-900/70 border border-slate-700 rounded" />
            <div className="h-4 bg-slate-900/70 border border-slate-700 rounded" />
            <div className="h-4 bg-slate-900/70 border border-slate-700 rounded" />
          </div>
        </div>
      )}

      {isPlanUpgradeMode && !billingLoading && !billingStatus && (
        <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-6">
          <h3 className="text-md font-semibold text-white">Current Plan</h3>
          <p className="text-sm text-slate-400 mt-2">Unable to load current plan details right now.</p>
        </div>
      )}

      {isPlanUpgradeMode && !billingLoading && billingStatus && (
        <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-6">
          <div className="flex items-center justify-between mb-4">
            <div>
              <h3 className="text-md font-semibold text-white">Current Plan</h3>
              <p className="text-sm text-slate-400 mt-1">{getPlanDisplayName(currentPlanCode)}</p>
            </div>
            <div className="flex items-center gap-2">
              <div className={`px-4 py-2 rounded-lg text-sm font-medium ${effectivePendingCancel
                  ? 'bg-amber-500/10 text-amber-400 border border-amber-500/40'
                  : statusLoading
                    ? 'bg-slate-700 text-slate-300 border border-slate-600'
                    : trialActive
                      ? 'bg-sky-900/40 text-sky-200 border border-sky-500/40'
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
            </div>
          </div>

          {trialActive && (
            <div className="mb-4 p-3 bg-sky-900/20 border border-sky-500/40 rounded-lg">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="text-sky-100 text-sm font-medium">
                    Trial is active{typeof trialDaysRemaining === 'number' ? ` - ${trialDaysRemaining} day${trialDaysRemaining === 1 ? '' : 's'} left` : ''}
                  </p>
                  <p className="text-sky-100/90 text-sm mt-1">
                    {trialEndsAt ? (
                      <>
                        Ends on <span className="text-white">{formatDate(trialEndsAt)}</span>
                        {trialStartsAt ? <> (started {formatDate(trialStartsAt)})</> : null}.
                      </>
                    ) : (
                      'Trial end date is being synchronized. Refresh in a moment to see precise timing.'
                    )}
                  </p>
                </div>
                {trialCanCancel && (
                  <button
                    type="button"
                    onClick={() => openCancelDialog(true)}
                    className="shrink-0 px-3 py-1.5 rounded bg-red-600 hover:bg-red-500 text-white text-xs font-semibold transition-colors"
                  >
                    Cancel Trial Now
                  </button>
                )}
              </div>
            </div>
          )}

          {effectivePendingCancel && !autoDowngradedToFree && (
            <div className="mb-4 p-3 bg-slate-700/50 border border-slate-600 rounded-lg">
              <div className="flex items-start gap-3">
                <svg className="w-5 h-5 text-slate-400 mt-0.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                <div>
                  <p className="text-slate-300 text-sm font-medium">Cancellation Scheduled</p>
                  <p className="text-slate-400 text-sm mt-1">
                    {effectivePendingExpiry ? (
                      <>
                        Your current plan stays active until <span className="text-white">{formatDate(effectivePendingExpiry)}</span>. On the next billing cycle, it will switch to the free hobby plan and team member access will be removed.
                      </>
                    ) : (
                      <>
                        Your current plan stays active until the end of this billing cycle. On the next billing cycle, it will switch to the free hobby plan and team member access will be removed.
                      </>
                    )}
                  </p>
                </div>
              </div>
            </div>
          )}

          {!effectivePendingCancel && hasScheduledPlanChange && (
            <div className={`mb-4 p-3 rounded-lg border ${isScheduledUpgrade ? 'bg-blue-900/20 border-blue-500/40' : 'bg-amber-900/20 border-amber-500/40'}`}>
              <div className="flex items-start gap-3">
                <svg className={`w-5 h-5 mt-0.5 flex-shrink-0 ${isScheduledUpgrade ? 'text-blue-300' : 'text-amber-300'}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                <div>
                  <p className={`text-sm font-medium ${isScheduledUpgrade ? 'text-blue-100' : 'text-amber-100'}`}>
                    {isScheduledUpgrade ? 'Upgrade Scheduled' : 'Downgrade Scheduled'}
                  </p>
                  <p className={`text-sm mt-1 ${isScheduledUpgrade ? 'text-blue-100/80' : 'text-amber-100/80'}`}>
                    {scheduledChangeTargetLabel ? (
                      <>
                        Your plan will change to <span className="text-white">{scheduledChangeTargetLabel}</span>
                      </>
                    ) : (
                      <>
                        A plan change is scheduled for your next billing cycle
                      </>
                    )}
                    {billingStatus?.billing?.scheduled_plan_effective_at ? (
                      <>
                        {' '}on <span className="text-white">{formatDate(billingStatus.billing.scheduled_plan_effective_at)}</span>.
                      </>
                    ) : (
                      <> at the next billing cycle.</>
                    )}
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

          <div className="grid grid-cols-1 md:grid-cols-4 gap-3 text-sm mb-4">
            <div className="bg-slate-900/70 border border-slate-700 rounded p-3">
              <p className="text-slate-400">Plan Capacity</p>
              <p className="text-white font-semibold">{currentLocLimit.toLocaleString()} LOC/month</p>
            </div>
            <div className="bg-slate-900/70 border border-slate-700 rounded p-3">
              <p className="text-slate-400">Org Usage This Period</p>
              <p className="text-white font-semibold">{currentLocUsed.toLocaleString()} LOC</p>
            </div>
            <div className="bg-slate-900/70 border border-slate-700 rounded p-3">
              <p className="text-slate-400">My Usage This Period</p>
              <p className="text-white font-semibold">{(myUsage?.total_billable_loc || 0).toLocaleString()} LOC</p>
            </div>
            <div className="bg-slate-900/70 border border-slate-700 rounded p-3">
              <p className="text-slate-400">Remaining</p>
              <p className="text-white font-semibold">{currentLocRemaining.toLocaleString()} LOC</p>
            </div>
          </div>

          <div className="space-y-2 mb-4">
            <div className="flex items-center justify-between text-xs text-slate-400">
              <span>Current billing period usage</span>
              <span className={`font-medium ${currentLocUsagePercent >= 100 ? 'text-red-400' : currentLocUsagePercent >= 90 ? 'text-amber-400' : ''}`}>
                {currentLocUsagePercent}%{currentLocUsagePercent >= 100 ? ' — BLOCKED' : ''}
              </span>
            </div>
            <div className="h-2.5 rounded-full bg-slate-900 border border-slate-700 overflow-hidden">
              <div
                className={`h-full transition-all duration-500 ${currentLocUsagePercent >= 100 ? 'bg-red-500' : currentLocUsagePercent >= 90 ? 'bg-amber-500' : 'bg-blue-500'
                  }`}
                style={{ width: `${Math.min(100, currentLocUsagePercent)}%` }}
              />
            </div>
          </div>

          <div className="space-y-3 text-sm">
            <div className="flex justify-between items-center py-2 border-b border-slate-700">
              <span className="text-slate-400">Usage Policy</span>
              <span className="text-white font-medium">{dailyLimit}</span>
            </div>
            <div className="flex justify-between items-center py-2 border-b border-slate-700">
              <span className="text-slate-400">AI Execution</span>
              <span className="text-white font-medium">{isTeamPlan ? 'Hosted Auto (BYOK optional)' : 'BYOK required'}</span>
            </div>
            <div className="flex justify-between items-center py-2">
              <span className="text-slate-400">Support</span>
              <span className={isTeamPlan ? 'text-emerald-400' : 'text-slate-500'}>
                {isTeamPlan ? 'Priority support enabled' : 'Standard support'}
              </span>
            </div>
          </div>
        </div>
      )}

      {isBreakdownMode && !usageSummary && (
        <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-6">
          <p className="text-sm text-slate-300">No usage data available yet for this billing period.</p>
        </div>
      )}

      {isBreakdownMode && usageSummary && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-md font-semibold text-white">Billing Period</h3>
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

          {myUsage && (
            <div>
              <div className="flex items-center justify-between mb-2">
                <p className="text-sm text-slate-300 font-medium">My Activity (Current Billing Period)</p>
                <span className="text-xs text-slate-400">Share: {myUsage.usage_share_percent.toFixed(2)}%</span>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-3 text-sm">
                <div className="bg-slate-950/50 border border-slate-700 rounded p-3">
                  <p className="text-slate-400">My LOC</p>
                  <p className="text-white font-semibold">{myUsage.total_billable_loc.toLocaleString()}</p>
                </div>
                <div className="bg-slate-950/50 border border-slate-700 rounded p-3">
                  <p className="text-slate-400">My Operations</p>
                  <p className="text-white font-semibold">{myUsage.operation_count.toLocaleString()}</p>
                </div>
                <div className="bg-slate-950/50 border border-slate-700 rounded p-3">
                  <p className="text-slate-400">Last Accounted</p>
                  <p className="text-white font-semibold">{formatDate(myUsage.last_accounted_at) || 'N/A'}</p>
                </div>
              </div>
            </div>
          )}

          {canManageBilling && usageMembers.length > 0 && (
            <div>
              <p className="text-sm text-slate-300 mb-2">Team Usage Breakdown</p>
              <div className="overflow-x-auto border border-slate-700 rounded-lg">
                <table className="min-w-full text-xs text-left">
                  <thead className="bg-slate-900/80 text-slate-300">
                    <tr>
                      <th className="px-3 py-2">Member</th>
                      <th className="px-3 py-2">Type</th>
                      <th className="px-3 py-2">LOC</th>
                      <th className="px-3 py-2">Share</th>
                      <th className="px-3 py-2">Operations</th>
                      <th className="px-3 py-2">Last Accounted</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-700 bg-slate-950/40">
                    {usageMembers.map((member) => (
                      <tr key={`${member.user_id || member.actor_email || member.actor_kind}-${member.last_accounted_at || member.operation_count}`}>
                        <td className="px-3 py-2 text-slate-100">{member.actor_email || 'System'}</td>
                        <td className="px-3 py-2 text-slate-300">{member.actor_kind || 'unknown'}</td>
                        <td className="px-3 py-2 text-white">{member.total_billable_loc.toLocaleString()}</td>
                        <td className="px-3 py-2 text-white">{member.usage_share_percent.toFixed(2)}%</td>
                        <td className="px-3 py-2 text-white">{member.operation_count.toLocaleString()}</td>
                        <td className="px-3 py-2 text-slate-300">{formatDate(member.last_accounted_at) || 'N/A'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          <div>
            <p className="text-sm text-slate-300 mb-2">Recent Operations</p>
            <div className="overflow-x-auto border border-slate-700 rounded-lg">
              <table className="min-w-full text-xs text-left">
                <thead className="bg-slate-900/80 text-slate-300">
                  <tr>
                    <th className="px-3 py-2">When</th>
                    <th className="px-3 py-2">Actor</th>
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
                      <td colSpan={7} className="px-3 py-3 text-slate-400">No operations found in current billing period.</td>
                    </tr>
                  )}
                  {usageOps.map((op) => (
                    <tr key={`${op.operation_id}-${op.accounted_at}`}>
                      <td className="px-3 py-2 text-slate-300">{formatDate(op.accounted_at)}</td>
                      <td className="px-3 py-2 text-slate-200">
                        {op.actor_email || (op.actor_kind === 'system' ? 'System' : 'Unknown')}
                        {op.user_id ? <span className="text-slate-400"> (#{op.user_id})</span> : null}
                      </td>
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

          {isSuperAdmin && (
            <BillingPortfolio embedded />
          )}
        </div>
      )}

      {isPlanUpgradeMode && canManageBilling && (
        <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-6 space-y-4">
          <h3 className="text-md font-semibold text-white">Upgrade Plan to Get More Usage</h3>
          <p className="text-sm text-slate-400">Choose an upgrade option below. Downgrade and cancellation are in Subscription Control.</p>

          {(billingLoading || statusLoading) && (
            <div className="space-y-3 animate-pulse">
              <div className="h-4 w-44 bg-slate-700 rounded" />
              <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-3">
                <div className="h-36 bg-slate-900/70 border border-slate-700 rounded-lg" />
                <div className="h-36 bg-slate-900/70 border border-slate-700 rounded-lg" />
                <div className="h-36 bg-slate-900/70 border border-slate-700 rounded-lg" />
              </div>
            </div>
          )}

          {!billingLoading && billingStatus && (
            <>

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
                  {requestStatus.customer_state && (
                    <div className="text-xs text-slate-300 bg-slate-950/60 border border-slate-700 rounded px-3 py-2 space-y-1">
                      <p>
                        Customer state: <span className="text-white font-semibold uppercase">{requestStatus.customer_state.replace(/_/g, ' ')}</span>
                      </p>
                      {requestStatus.action_required?.type && requestStatus.action_required.type !== 'none' && (
                        <p>
                          Action required: <span className="text-amber-300 font-medium">{requestStatus.action_required.type.replace(/_/g, ' ')}</span>
                          {requestStatus.action_needed_at ? <span className="text-slate-400"> since {formatDate(requestStatus.action_needed_at)}</span> : null}
                        </p>
                      )}
                      {requestStatus.support_context?.razorpay_order_id && (
                        <p>Order ID: <span className="text-white font-mono">{requestStatus.support_context.razorpay_order_id}</span></p>
                      )}
                      {requestStatus.support_context?.razorpay_payment_id && (
                        <p>Payment ID: <span className="text-white font-mono">{requestStatus.support_context.razorpay_payment_id}</span></p>
                      )}
                      {requestStatus.support_reference && (
                        <p>Support ref: <span className="text-white font-mono">{requestStatus.support_reference}</span></p>
                      )}
                    </div>
                  )}
                </div>
              )}

              {lastUpgradeResult?.proration && (
                <div className="bg-blue-900/20 border border-blue-500/40 rounded-lg p-4 space-y-2">
                  <p className="text-sm text-blue-200 font-medium">Latest Upgrade Charge Summary</p>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-xs text-slate-200">
                    <p>From: <span className="text-white">{lastUpgradeResult.proration.from_plan_code || 'n/a'}</span></p>
                    <p>To: <span className="text-white">{lastUpgradeResult.proration.to_plan_code || lastUpgradeResult.plan_code || 'n/a'}</span></p>
                    <p>Charged now: <span className="text-white">{formatChargeAmount(lastUpgradeResult.proration.charge_amount_cents, lastUpgradeResult.proration.charge_currency) || formatMinorAmount(0, purchaseCurrency)}</span></p>
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

              {actionProgressMessage && (
                <div className="bg-sky-500/10 border border-sky-500/40 rounded-lg p-3 text-sm text-sky-100">
                  {actionProgressMessage}
                </div>
              )}

              {(keepPlanError || keepPlanSuccess) && (
                <div className={`rounded-lg border p-3 text-sm ${keepPlanError
                    ? 'bg-red-500/10 border-red-500/40 text-red-200'
                    : 'bg-emerald-500/10 border-emerald-500/40 text-emerald-200'
                  }`}>
                  {keepPlanError || keepPlanSuccess}
                </div>
              )}

              <div className="space-y-4">
                <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-3">
                  {upgradeHierarchyPlans.map((plan) => {
                    const isCurrentCard = plan.plan_code === currentPlanCode;
                    const isSelectableUpgrade = plan.monthly_loc_limit > currentLocLimit;
                    const planTrialDays = Number(plan.trial_days || firstPaidTrialDays || 0);
                    const showsFirstPaidTrialCopy = isFree && plan.monthly_price_usd > 0 && planTrialDays > 0;
                    const firstPaidTrialCardBadgeText = trialEligibleForFirstPaidPurchase
                      ? `Free ${planTrialDays}-Day Trial Included`
                      : trialEligibilityStatus === 'already_used'
                        ? 'Trial already used'
                        : trialEligibilityStatus === 'reserved'
                          ? 'Trial reservation in progress'
                          : `Up to ${planTrialDays}-day trial`;
                    const cardBaseClass = `rounded-lg border p-4 text-left transition-all duration-200 ${isCurrentCard
                        ? 'bg-emerald-900/20 border-emerald-400/50'
                        : 'bg-slate-900/70 border-slate-700'
                      }`;

                    if (!isSelectableUpgrade) {
                      return (
                        <div
                          key={plan.plan_code}
                          className={cardBaseClass}
                        >
                          <div className="flex items-center justify-between gap-2">
                            <p className="text-sm font-semibold text-white">{getPlanDisplayName(plan.plan_code)}</p>
                            {isCurrentCard ? (
                              <span className="text-[10px] px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-300 border border-emerald-500/40">Current</span>
                            ) : null}
                          </div>
                          <p className="mt-2 text-sm text-slate-300">{plan.monthly_loc_limit.toLocaleString()} LOC / month</p>
                          <p className="text-sm text-slate-300">${plan.monthly_price_usd}/month</p>
                        </div>
                      );
                    }

                    return (
                      <button
                        key={plan.plan_code}
                        type="button"
                        disabled={actionLoading || keepPlanLoading || upgradeCheckoutLoading}
                        onClick={() => {
                          void openUpgradePreview(plan.plan_code);
                        }}
                        title={`Upgrade to ${getPlanDisplayName(plan.plan_code)}`}
                        className={`${cardBaseClass} relative overflow-hidden group hover:-translate-y-0.5 hover:bg-emerald-950/40 hover:border-emerald-300/70 hover:shadow-xl hover:shadow-emerald-950/45 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400/70 disabled:opacity-60 disabled:cursor-not-allowed`}
                      >
                        <div className="pointer-events-none absolute inset-0 opacity-0 transition-opacity duration-200 group-hover:opacity-100 bg-gradient-to-br from-emerald-500/15 via-emerald-500/5 to-transparent" />
                        <div className="relative z-10">
                          <p className="text-sm font-semibold text-white">{getPlanDisplayName(plan.plan_code)}</p>
                          <p className="mt-2 text-sm text-slate-300">{plan.monthly_loc_limit.toLocaleString()} LOC / month</p>
                          <p className="text-sm text-slate-300">${plan.monthly_price_usd}/month</p>
                          <div className="mt-3 flex flex-wrap items-center gap-2">
                            <div className="inline-flex px-3 py-1.5 rounded bg-emerald-600 text-white text-xs font-medium transition-colors group-hover:bg-emerald-500">
                              Upgrade
                            </div>
                            {showsFirstPaidTrialCopy && (
                              <span className={`inline-flex items-center rounded border px-2 py-1 text-[10px] font-semibold uppercase tracking-wide ${trialEligibleForFirstPaidPurchase
                                  ? 'border-sky-400/50 bg-sky-900/35 text-sky-100'
                                  : trialEligibilityStatus === 'already_used'
                                    ? 'border-slate-600 bg-slate-800 text-slate-300'
                                    : 'border-amber-400/50 bg-amber-900/30 text-amber-100'
                                }`}>
                                {firstPaidTrialCardBadgeText}
                              </span>
                            )}
                          </div>
                        </div>
                      </button>
                    );
                  })}
                </div>
                {upgradeOptions.length === 0 && (
                  <div className="rounded-lg border border-slate-700 bg-slate-900/50 p-3 text-sm text-slate-300">
                    This organization is already on the highest available LOC plan.
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      )}

      {isControlsMode && (
        <div className="space-y-4">

          {!canManageBilling && (
            <div className="bg-slate-900/50 border border-slate-700 rounded-lg p-3 text-sm text-slate-300">
              Only organization owners, admins, or superadmins can change subscription controls.
            </div>
          )}

          {(statusLoading || subscriptionLoading || billingLoading) && (
            <div className="space-y-3 animate-pulse">
              <div className="h-20 rounded-lg bg-slate-900/70 border border-slate-700" />
              <div className="h-20 rounded-lg bg-slate-900/70 border border-slate-700" />
              <div className="h-36 rounded-lg bg-slate-900/70 border border-slate-700" />
            </div>
          )}

          {canManageBilling && !statusLoading && !subscriptionLoading && (
            <>
              {effectiveSubscriptionId && (
                <div className="p-4 bg-slate-900/70 border border-slate-700 rounded-lg space-y-2">
                  <p className="text-sm text-slate-300 font-medium">Advanced</p>
                  <button
                    type="button"
                    onClick={() => navigate(`/subscribe/subscriptions/${effectiveSubscriptionId}/assign`)}
                    className="text-xs text-slate-300 hover:text-white underline underline-offset-2"
                  >
                    Open advanced access assignment
                  </button>
                </div>
              )}

              <div className="p-4 bg-slate-900/70 border border-slate-700 rounded-lg space-y-3">
                <p className="text-sm text-slate-300 font-medium">Subscription Link (Razorpay)</p>
                {effectiveSubscriptionManageURL ? (
                  <a
                    href={effectiveSubscriptionManageURL}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-2 text-sm text-slate-200 hover:text-white underline underline-offset-2"
                  >
                    Manage Payment
                  </a>
                ) : (
                  <p className="text-xs text-slate-400">No Razorpay management link available for this subscription.</p>
                )}
              </div>

              <div className="p-4 bg-slate-900/70 border border-slate-700 rounded-lg space-y-3">
                <p className="text-sm text-slate-300 font-medium">Cancel Subscription</p>
                {trialCanCancel ? (
                  <div className="space-y-2">
                    <p className="text-xs text-slate-400">
                      Trial ends on {trialEndsAt ? formatDate(trialEndsAt) : 'the configured trial end date'}.
                    </p>
                    <button
                      className="text-xs text-red-300 hover:text-red-200 underline underline-offset-2 disabled:opacity-60 disabled:cursor-not-allowed"
                      disabled={keepPlanLoading || actionLoading || statusLoading || subscriptionLoading}
                      onClick={() => openCancelDialog(true)}
                    >
                      Cancel Trial Now (Immediate)
                    </button>
                  </div>
                ) : canKeepEffectivePlan ? (
                  <div className="space-y-2">
                    <p className="text-xs text-slate-400">Cancellation is scheduled for period end.</p>
                    <button
                      className="text-xs text-slate-300 hover:text-white underline underline-offset-2 disabled:opacity-60 disabled:cursor-not-allowed"
                      disabled={keepPlanLoading || actionLoading || statusLoading || subscriptionLoading}
                      onClick={() => {
                        void handleKeepPlan();
                      }}
                    >
                      {keepPlanLoading ? 'Keeping Plan...' : 'Keep Plan'}
                    </button>
                  </div>
                ) : canCancelEffectiveSubscription ? (
                  <button
                    className="text-xs text-slate-300 hover:text-white underline underline-offset-2"
                    onClick={() => openCancelDialog(false)}
                  >
                    Cancel Subscription
                  </button>
                ) : (
                  <p className="text-xs text-slate-400">
                    {effectivePendingCancel ? 'Cancellation is already scheduled for period end.' : 'No active subscription cancellation action available.'}
                  </p>
                )}
                {keepPlanError && (
                  <p className="text-xs text-red-300">{keepPlanError}</p>
                )}
                {keepPlanSuccess && (
                  <p className="text-xs text-emerald-300">{keepPlanSuccess}</p>
                )}
                {billingError && (
                  <p className="text-xs text-red-300">{billingError}</p>
                )}
                {actionProgressMessage && (
                  <p className="text-xs text-sky-200">{actionProgressMessage}</p>
                )}
              </div>

              <div className="p-4 bg-slate-900/70 border border-slate-700 rounded-lg space-y-4">
                <p className="text-sm text-slate-300 font-medium">Downgrade Plan</p>
                <p className="text-xs text-slate-400">Downgrades are scheduled and take effect at the end of your current billing cycle.</p>
                {billingStatus ? (
                  <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-3">
                    {downgradeOptions.map((plan) => {
                      return (
                        <button
                          key={plan.plan_code}
                          type="button"
                          disabled={actionLoading || keepPlanLoading}
                          onClick={() => {
                            setSelectedDowngradePlan(plan.plan_code);
                            void runBillingAction('schedule_downgrade', plan.plan_code);
                          }}
                          title={`Downgrade to ${getPlanDisplayName(plan.plan_code)}`}
                          className="relative overflow-hidden group rounded-lg border p-4 text-left transition-all duration-200 bg-slate-950/50 border-slate-700 hover:-translate-y-0.5 hover:bg-amber-950/30 hover:border-amber-300/70 hover:shadow-xl hover:shadow-amber-950/35 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-400/70 disabled:opacity-60 disabled:cursor-not-allowed"
                        >
                          <div className="pointer-events-none absolute inset-0 opacity-0 transition-opacity duration-200 group-hover:opacity-100 bg-gradient-to-br from-amber-500/15 via-amber-500/5 to-transparent" />
                          <div className="relative z-10">
                            <div className="flex items-center justify-between gap-2 mb-1">
                              <p className="text-sm font-semibold text-white">{getPlanDisplayName(plan.plan_code)}</p>
                            </div>
                            <p className="text-sm text-slate-300">{plan.monthly_loc_limit.toLocaleString()} LOC / month</p>
                            <p className="text-sm text-slate-300">${plan.monthly_price_usd}/month</p>
                            <div className="mt-3 inline-flex px-3 py-1.5 rounded bg-amber-600 text-white text-xs font-medium transition-colors group-hover:bg-amber-500">
                              Downgrade
                            </div>
                          </div>
                        </button>
                      );
                    })}
                  </div>
                ) : (
                  <p className="text-xs text-slate-400">Billing details are still loading.</p>
                )}
                {downgradeOptions.length === 0 && (
                  <p className="text-xs text-slate-400">No lower plans available from the current plan.</p>
                )}
                <div className="flex flex-wrap items-center gap-3">
                  {billingStatus?.billing?.scheduled_plan_code && (
                    <button
                      className="px-3 py-2 rounded bg-slate-700 hover:bg-slate-600 disabled:opacity-50 text-sm font-medium"
                      disabled={actionLoading || keepPlanLoading}
                      onClick={() => runBillingAction('cancel_downgrade')}
                    >
                      {cancelScheduledLabel}
                    </button>
                  )}
                </div>
              </div>

              {billingStatus?.billing?.scheduled_plan_code && (
                <div className="text-sm text-slate-300 bg-slate-900/40 border border-slate-700 rounded-lg p-3">
                  {scheduledChangeLabel}: <span className="text-white font-medium">{billingStatus.billing.scheduled_plan_code}</span>
                  {billingStatus.billing.scheduled_plan_effective_at && (
                    <span> effective {formatDate(billingStatus.billing.scheduled_plan_effective_at)}</span>
                  )}
                </div>
              )}
            </>
          )}
        </div>
      )}

      {!isBreakdownMode && cancelTargetSubscriptionId && (
        <CancelSubscriptionModal
          isOpen={showCancelModal}
          onClose={() => {
            setShowCancelModal(false);
            setCancelImmediate(false);
          }}
          onSuccess={handleCancelSuccess}
          subscriptionId={cancelTargetSubscriptionId}
          expiryDate={cancelImmediate ? trialEndsAt : (displayExpiry || managedSubscription?.current_period_end || licenseExpiresAt)}
          immediate={cancelImmediate}
        />
      )}

      {!isBreakdownMode && showFreeCheckoutModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/80 p-4">
          <div className="w-full max-w-2xl rounded-xl border border-slate-700 bg-slate-900 shadow-2xl">
            <div className="border-b border-slate-700 px-6 py-4">
              <h3 className="text-lg font-semibold text-white">Confirm New Paid Plan</h3>
              <p className="mt-1 text-sm text-slate-300">
                You are moving from Free 30k BYOK to a paid LOC slab. {freeCheckoutTrialMessage}
              </p>
            </div>

            <div className="space-y-4 px-6 py-5 text-sm">
              <div className="rounded-lg border border-slate-700 bg-slate-800/70 p-4 space-y-2 text-slate-200">
                <p>Current plan: <span className="text-white">Free 30k BYOK</span></p>
                <p>Target plan: <span className="text-white">{getPlanDisplayName(selectedUpgradePlan)}</span></p>
              </div>

              <div className="rounded-lg border border-slate-700 bg-slate-800/70 p-4 space-y-2 text-slate-200">
                <label htmlFor="new-plan-currency" className="text-xs uppercase tracking-wide text-slate-300">Billing currency</label>
                <select
                  id="new-plan-currency"
                  value={purchaseCurrency}
                  disabled={upgradeCheckoutLoading}
                  onChange={(event) => {
                    const nextCurrency = normalizePurchaseCurrency(event.target.value);
                    if (!nextCurrency || nextCurrency === purchaseCurrency) {
                      return;
                    }
                    setPurchaseCurrency(nextCurrency);
                  }}
                  className="w-full rounded border border-slate-600 bg-slate-900 px-3 py-2 text-sm text-white focus:outline-none focus:ring-2 focus:ring-emerald-500"
                >
                  {availablePurchaseCurrencies.map((currency) => (
                    <option key={currency} value={currency}>{currency}</option>
                  ))}
                </select>
                <p className="text-xs text-slate-400">Default currency is selected from your region (IN defaults to INR, others default to USD).</p>
              </div>

              <div className="rounded-lg border border-blue-500/40 bg-blue-900/20 p-4 space-y-2 text-slate-100">
                <p className="font-medium text-blue-200">Checkout summary</p>
                <p>
                  Recurring amount (USD reference): <span className="text-white font-semibold">${selectedFreeCheckoutPriceUSD}/month</span>
                </p>
                <p>
                  Checkout currency: <span className="text-white font-semibold">{purchaseCurrency}</span>
                </p>
                <p>
                  Included LOC: <span className="text-white font-semibold">{selectedFreeCheckoutLoc.toLocaleString()} LOC/month</span>
                </p>
                {selectedFreeCheckoutTrialDays > 0 && (
                  <p>
                    Trial policy: <span className="text-white font-semibold">{trialEligibleForFirstPaidPurchase ? `${selectedFreeCheckoutTrialDays} days before recurring billing` : 'Not available for this email'}</span>
                  </p>
                )}
              </div>

              <div className="rounded-lg border border-amber-500/40 bg-amber-900/20 p-3 text-xs text-amber-100">
                This flow opens Razorpay checkout now and activates your selected paid plan after payment confirmation. Trial eligibility, if available, is enforced by backend policy at checkout time. During trial setup, Razorpay may show a small authorization amount to validate the payment method; recurring billing starts after trial ends.
              </div>
            </div>

            <div className="flex items-center justify-end gap-3 border-t border-slate-700 px-6 py-4">
              <button
                type="button"
                disabled={upgradeCheckoutLoading}
                onClick={() => setShowFreeCheckoutModal(false)}
                className="rounded border border-slate-600 px-4 py-2 text-sm text-slate-200 hover:bg-slate-800 disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                type="button"
                disabled={upgradeCheckoutLoading}
                onClick={startFreeCheckoutFromSettings}
                className="rounded bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-500 disabled:opacity-50"
              >
                {upgradeCheckoutLoading ? 'Processing...' : 'Confirm and Pay Now'}
              </button>
            </div>
          </div>
        </div>
      )}

      {!isBreakdownMode && showUpgradeModal && upgradePreview?.preview && (
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

              <div className="rounded-lg border border-slate-700 bg-slate-800/70 p-4 space-y-2 text-slate-200">
                <label htmlFor="upgrade-currency" className="text-xs uppercase tracking-wide text-slate-300">Billing currency</label>
                <select
                  id="upgrade-currency"
                  value={purchaseCurrency}
                  disabled={upgradeCheckoutLoading || actionLoading}
                  onChange={(event) => {
                    const nextCurrency = normalizePurchaseCurrency(event.target.value);
                    if (!nextCurrency || nextCurrency === purchaseCurrency) {
                      return;
                    }
                    setPurchaseCurrency(nextCurrency);
                    void openUpgradePreview(selectedUpgradePlan || upgradePreview.preview.to_plan_code, nextCurrency);
                  }}
                  className="w-full rounded border border-slate-600 bg-slate-900 px-3 py-2 text-sm text-white focus:outline-none focus:ring-2 focus:ring-emerald-500"
                >
                  {availablePurchaseCurrencies.map((currency) => (
                    <option key={currency} value={currency}>{currency}</option>
                  ))}
                </select>
                <p className="text-xs text-slate-400">Default currency is selected from your region (IN defaults to INR, others default to USD).</p>
              </div>

              <div className="rounded-lg border border-blue-500/40 bg-blue-900/20 p-4 space-y-2 text-slate-100">
                <p className="font-medium text-blue-200">Immediate one-time payment</p>
                <p>
                  Charge now: <span className="text-white font-semibold">{formatChargeAmount(upgradePreview.preview.immediate_charge_cents, upgradePreview.preview.immediate_charge_currency) || formatMinorAmount(0, purchaseCurrency)}</span>
                  {' '}(remaining cycle {(upgradePreview.preview.remaining_cycle_fraction * 100).toFixed(2)}%)
                </p>
                <p>
                  Formula: <span className="text-white">{formatChargeAmount(upgradePreview.preview.next_cycle_price_cents, upgradePreview.preview.immediate_charge_currency) || formatMinorAmount(0, purchaseCurrency)} × {(upgradePreview.preview.remaining_cycle_fraction * 100).toFixed(2)}%</span>
                </p>
                <p>
                  Immediate LOC grant: <span className="text-white font-semibold">{upgradePreview.preview.immediate_loc_grant.toLocaleString()} LOC</span>
                </p>
              </div>

              <div className="rounded-lg border border-emerald-500/40 bg-emerald-900/20 p-4 space-y-2 text-slate-100">
                <p className="font-medium text-emerald-200">Next billing cycle</p>
                <p>
                  Recurring amount: <span className="text-white font-semibold">{formatChargeAmount(upgradePreview.preview.next_cycle_price_cents, upgradePreview.preview.immediate_charge_currency) || formatMinorAmount(0, purchaseCurrency)}/month</span>
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

export default SubscriptionTab;
