import React, { useState, useEffect } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useOrgContext } from '../../hooks/useOrgContext';
import apiClient from '../../api/apiClient';
import { useAppSelector } from '../../store/configureStore';

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

    /* Match LiveReview shell with a dark overlay while checkout is open. */
    body.razorpay-active .razorpay-backdrop {
      background: rgba(2, 6, 23, 0.86) !important;
      backdrop-filter: blur(1px);
    }

    /* Fallback dimmer in variants where backdrop node is absent. */
    body.razorpay-active .razorpay-container {
      background: rgba(2, 6, 23, 0.86) !important;
    }
  `;
  document.head.appendChild(style);
};

const cleanupRazorpayOverlay = () => {
  document.body.classList.remove('razorpay-active');
};

type CheckoutSuccess = {
  subscriptionId: string;
  paymentId?: string;
  planCode: string;
  planLabel: string;
  locLimit: number;
  total: number;
  billingNote: string;
};

type PaymentFailure = {
  errorCode?: string;
  errorDescription: string;
  errorStep?: string;
  errorReason?: string;
  subscriptionId?: string;
  planCode: string;
  planLabel: string;
  locLimit: number;
  total: number;
  currency?: PurchaseCurrency;
};

type PurchaseCurrency = 'USD' | 'INR';

type LOCSlab = {
  code: string;
  label: string;
  locLimit: number;
  monthlyPriceUSD: number;
};

type APIVersionResponse = {
  subscriptionContractVersion?: string;
};

type ActivationProgress = {
  state: 'idle' | 'checking' | 'applied' | 'delayed';
  message: string;
  lastCheckedAt?: string;
};

const RAZORPAY_THEME = {
  color: '#131C2F',
};

const LOC_SLABS: LOCSlab[] = [
  { code: 'team_32usd', label: '100k LOC', locLimit: 100000, monthlyPriceUSD: 32 },
  { code: 'loc_200k', label: '200k LOC', locLimit: 200000, monthlyPriceUSD: 64 },
  { code: 'loc_400k', label: '400k LOC', locLimit: 400000, monthlyPriceUSD: 128 },
  { code: 'loc_800k', label: '800k LOC', locLimit: 800000, monthlyPriceUSD: 256 },
  { code: 'loc_1600k', label: '1.6M LOC', locLimit: 1600000, monthlyPriceUSD: 512 },
  { code: 'loc_3200k', label: '3.2M LOC', locLimit: 3200000, monthlyPriceUSD: 1024 },
];

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

const resolvePlanCodeFromQuery = (planCode: string | null): string => {
  const normalized = String(planCode || '').trim().toLowerCase();
  if (!normalized) {
    return 'team_32usd';
  }
  const found = LOC_SLABS.find((slab) => slab.code.toLowerCase() === normalized);
  return found?.code || 'team_32usd';
};

const TeamCheckout: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { currentOrgId, userOrganizations } = useOrgContext();
  const { organizations: authOrgs } = useAppSelector((state) => state.Auth);
  
  const period = searchParams.get('period') || 'monthly';
  const [selectedPlanCode, setSelectedPlanCode] = useState<string>(() => resolvePlanCodeFromQuery(searchParams.get('plan')));
  const [selectedCurrency, setSelectedCurrency] = useState<PurchaseCurrency>(() => {
    const queryCurrency = normalizePurchaseCurrency(searchParams.get('currency'));
    return queryCurrency || fallbackPurchaseCurrencyFromLocale();
  });
  const [isProcessing, setIsProcessing] = useState<boolean>(false);
  const [isConfirming, setIsConfirming] = useState<boolean>(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [successInfo, setSuccessInfo] = useState<CheckoutSuccess | null>(null);
  const [failureInfo, setFailureInfo] = useState<PaymentFailure | null>(null);
  const [currentSubscriptionData, setCurrentSubscriptionData] = useState<any>(null);
  const [razorpayReady, setRazorpayReady] = useState(false);
  const [contractReady, setContractReady] = useState(false);
  const [activationProgress, setActivationProgress] = useState<ActivationProgress>({
    state: 'idle',
    message: 'Payment submitted. Waiting for plan activation confirmation.',
  });

  const selectedPlan = LOC_SLABS.find((slab) => slab.code === selectedPlanCode) || LOC_SLABS[0];
  const totalPrice = selectedPlan.monthlyPriceUSD;

  // Get org ID - try currentOrgId first, then fall back to first auth org
  const orgId = currentOrgId || (authOrgs && authOrgs.length > 0 ? authOrgs[0].id : null);

  useEffect(() => {
    const requestedPlanCode = resolvePlanCodeFromQuery(searchParams.get('plan'));
    setSelectedPlanCode((prev) => (prev === requestedPlanCode ? prev : requestedPlanCode));
    const requestedCurrency = normalizePurchaseCurrency(searchParams.get('currency'));
    if (requestedCurrency) {
      setSelectedCurrency((prev) => (prev === requestedCurrency ? prev : requestedCurrency));
    }
  }, [searchParams]);

  useEffect(() => {
    // Verify user is authenticated
    const token = localStorage.getItem('accessToken');
    if (!token) {
      navigate('/signin', { state: { returnTo: `/checkout/team?period=${period}` } });
    }
  }, [navigate, period]);

  useEffect(() => {
    let active = true;
    const verifyContract = async () => {
      try {
        const response = await fetch('/api/version');
        if (!response.ok) {
          throw new Error('version endpoint unavailable');
        }
        const version: APIVersionResponse = await response.json();
        const contractVersion = version?.subscriptionContractVersion || '';
        if (contractVersion !== 'slab_plan_code_v1') {
          if (active) {
            setContractReady(false);
            setErrorMessage('Backend version mismatch detected. Subscription API is not on slab plan_code contract yet. Please restart/update the API service.');
          }
          return;
        }
        if (active) {
          setContractReady(true);
        }
      } catch (err) {
        if (active) {
          setContractReady(false);
          setErrorMessage('Unable to verify API version. Please ensure the backend is running and reachable.');
        }
      }
    };

    verifyContract();
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    let mounted = true;
    ensureRazorpay()
      .then(() => {
        if (mounted) setRazorpayReady(true);
        if (mounted) ensureRazorpayStyles();
      })
      .catch((err) => {
        console.error('Failed to load Razorpay script:', err);
        if (mounted) setErrorMessage('Unable to load payment SDK. Please retry.');
      });
    return () => {
      mounted = false;
      cleanupRazorpayOverlay();
    };
  }, []);

  useEffect(() => {
    if (!successInfo) {
      setActivationProgress({
        state: 'idle',
        message: 'Payment submitted. Waiting for plan activation confirmation.',
      });
      return;
    }

    let cancelled = false;
    let attempts = 0;
    const maxAttempts = 24;

    const checkActivation = async () => {
      attempts += 1;
      setActivationProgress({
        state: 'checking',
        message: 'Checking activation status...',
        lastCheckedAt: new Date().toISOString(),
      });

      try {
        const billing = await apiClient.get<any>('/billing/status');
        const currentPlanCode = billing?.billing?.current_plan_code;
        if (currentPlanCode === successInfo.planCode) {
          if (!cancelled) {
            setActivationProgress({
              state: 'applied',
              message: `Plan activation confirmed (${successInfo.planCode}).`,
              lastCheckedAt: new Date().toISOString(),
            });
          }
          return true;
        }
      } catch {
        // Keep polling; transient failures are expected during post-payment transitions.
      }

      return false;
    };

    const intervalId = window.setInterval(async () => {
      if (cancelled) return;
      const applied = await checkActivation();
      if (applied || attempts >= maxAttempts) {
        window.clearInterval(intervalId);
        if (!applied && !cancelled) {
          setActivationProgress({
            state: 'delayed',
            message: 'Payment is captured, but plan activation is still syncing. You can monitor status in Subscription Settings.',
            lastCheckedAt: new Date().toISOString(),
          });
        }
      }
    }, 5000);

    checkActivation();

    return () => {
      cancelled = true;
      window.clearInterval(intervalId);
    };
  }, [successInfo]);

  const handlePurchase = async () => {
    setIsProcessing(true);
    setErrorMessage(null);
    setSuccessInfo(null);
    setFailureInfo(null);

    try {
      const token = localStorage.getItem('accessToken');
      if (!token) {
        setFailureInfo({
          errorDescription: 'Please sign in to continue with your purchase.',
          planCode: selectedPlan.code,
          planLabel: selectedPlan.label,
          locLimit: selectedPlan.locLimit,
          total: totalPrice,
        });
        setIsProcessing(false);
        return;
      }

      if (!orgId) {
        setFailureInfo({
          errorDescription: 'No organization selected. Please switch to an organization and try again.',
          planCode: selectedPlan.code,
          planLabel: selectedPlan.label,
          locLimit: selectedPlan.locLimit,
          total: totalPrice,
        });
        setIsProcessing(false);
        return;
      }

      let data: any;
      try {
        // Use apiClient which automatically adds X-Org-Context from Redux store
        data = await apiClient.post('/subscriptions', {
          plan_code: selectedPlan.code,
          currency: selectedCurrency,
        });
      } catch (err: any) {
        // Fall back to direct fetch with explicit headers if org context error occurs
        if (err?.message?.toLowerCase().includes('organization context')) {
          const response = await fetch('/api/v1/subscriptions', {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${token}`,
              'X-Org-Context': orgId.toString(),
            },
            body: JSON.stringify({
              plan_code: selectedPlan.code,
              currency: selectedCurrency,
            }),
          });

          if (!response.ok) {
            const errorData = await response.json().catch(() => ({}));
            throw new Error(errorData?.message || errorData?.error || 'Failed to create subscription');
          }

          data = await response.json();
        } else {
          throw err;
        }
      }

      // Store subscription data for retry
      setCurrentSubscriptionData(data);

      // Ensure Razorpay SDK is present before opening checkout
      if (!razorpayReady) {
        await ensureRazorpay();
        ensureRazorpayStyles();
        setRazorpayReady(true);
      }

      // Initialize Razorpay checkout
      const options = {
        key: data.razorpay_key_id,
        subscription_id: data.razorpay_subscription_id,
        name: 'LiveReview LOC Plan',
        description: `${selectedPlan.label} (${selectedPlan.locLimit.toLocaleString()} LOC/month)`,
        image: '/assets/logo-with-text.svg',
        handler: async (razorpayResponse: any) => {
          // Show loader while confirming purchase
          setIsConfirming(true);

          const paymentID = razorpayResponse?.razorpay_payment_id;
          const signature = razorpayResponse?.razorpay_signature;

          if (!paymentID || !signature) {
            cleanupRazorpayOverlay();
            setFailureInfo({
              errorDescription: 'Payment confirmation payload is incomplete. Please retry payment.',
              planCode: selectedPlan.code,
              planLabel: selectedPlan.label,
              locLimit: selectedPlan.locLimit,
              total: totalPrice,
              subscriptionId: data.razorpay_subscription_id,
            });
            setIsProcessing(false);
            setIsConfirming(false);
            return;
          }
          
          // Immediately confirm the purchase to prevent race conditions with webhooks
          try {
            await apiClient.post('/subscriptions/confirm-purchase', {
              razorpay_subscription_id: data.razorpay_subscription_id,
              razorpay_payment_id: paymentID,
              razorpay_signature: signature,
            });
          } catch (confirmError: any) {
            const confirmErrorMessage = confirmError?.message || 'Payment was completed but confirmation failed. Please retry.';
            cleanupRazorpayOverlay();
            setFailureInfo({
              errorDescription: confirmErrorMessage,
              planCode: selectedPlan.code,
              planLabel: selectedPlan.label,
              locLimit: selectedPlan.locLimit,
              total: totalPrice,
              subscriptionId: data.razorpay_subscription_id,
            });
            setIsProcessing(false);
            setIsConfirming(false);
            return;
          }

          cleanupRazorpayOverlay();
          setIsProcessing(false);
          setIsConfirming(false);
          setSuccessInfo({
            subscriptionId: data.razorpay_subscription_id,
            paymentId: razorpayResponse?.razorpay_payment_id,
            planCode: selectedPlan.code,
            planLabel: selectedPlan.label,
            locLimit: selectedPlan.locLimit,
            total: totalPrice,
            billingNote: 'Billed monthly',
          });
        },
        modal: {
          ondismiss: () => {
            setIsProcessing(false);
            setIsConfirming(false);
            cleanupRazorpayOverlay();
          },
        },
        theme: RAZORPAY_THEME,
      };

      const rzp = new window.Razorpay(options);
      document.body.classList.add('razorpay-active');
      rzp.on('payment.failed', (response: any) => {
        const error = response.error || {};
        setFailureInfo({
          errorCode: error.code,
          errorDescription: error.description || 'Payment failed. Please try again.',
          errorStep: error.step,
          errorReason: error.reason,
          subscriptionId: data.razorpay_subscription_id,
          planCode: selectedPlan.code,
          planLabel: selectedPlan.label,
          locLimit: selectedPlan.locLimit,
          total: totalPrice,
        });
        setIsProcessing(false);
        setIsConfirming(false);
        cleanupRazorpayOverlay();
      });
      rzp.open();
    } catch (error) {
      // Show failure page for any errors during subscription creation
      const errorMessage = error instanceof Error ? error.message : 'An error occurred during checkout';
      cleanupRazorpayOverlay();
      setFailureInfo({
        errorDescription: errorMessage,
        planCode: selectedPlan.code,
        planLabel: selectedPlan.label,
        locLimit: selectedPlan.locLimit,
        total: totalPrice,
      });
      setIsProcessing(false);
      setIsConfirming(false);
    }
  };

  const handleRetryPayment = () => {
    if (!currentSubscriptionData) {
      // If we don't have the subscription data, restart the flow
      setFailureInfo(null);
      return;
    }

    // Reopen Razorpay with the existing subscription
    setFailureInfo(null);
    setIsProcessing(true);

    const options = {
      key: currentSubscriptionData.razorpay_key_id,
      subscription_id: currentSubscriptionData.razorpay_subscription_id,
      name: 'LiveReview LOC Plan',
      description: `${selectedPlan.label} (${selectedPlan.locLimit.toLocaleString()} LOC/month)`,
      image: '/assets/logo-with-text.svg',
      handler: async (razorpayResponse: any) => {
        setIsConfirming(true);

        const paymentID = razorpayResponse?.razorpay_payment_id;
        const signature = razorpayResponse?.razorpay_signature;

        if (!paymentID || !signature) {
          cleanupRazorpayOverlay();
          setFailureInfo({
            errorDescription: 'Payment confirmation payload is incomplete. Please retry payment.',
            planCode: selectedPlan.code,
            planLabel: selectedPlan.label,
            locLimit: selectedPlan.locLimit,
            total: totalPrice,
            subscriptionId: currentSubscriptionData.razorpay_subscription_id,
          });
          setIsProcessing(false);
          setIsConfirming(false);
          return;
        }
        
        try {
          await apiClient.post('/subscriptions/confirm-purchase', {
            razorpay_subscription_id: currentSubscriptionData.razorpay_subscription_id,
            razorpay_payment_id: paymentID,
            razorpay_signature: signature,
          });
        } catch (confirmError: any) {
          const confirmErrorMessage = confirmError?.message || 'Payment was completed but confirmation failed. Please retry.';
          cleanupRazorpayOverlay();
          setFailureInfo({
            errorDescription: confirmErrorMessage,
            planCode: selectedPlan.code,
            planLabel: selectedPlan.label,
            locLimit: selectedPlan.locLimit,
            total: totalPrice,
            subscriptionId: currentSubscriptionData.razorpay_subscription_id,
          });
          setIsProcessing(false);
          setIsConfirming(false);
          return;
        }

        cleanupRazorpayOverlay();
        setIsProcessing(false);
        setIsConfirming(false);
        setSuccessInfo({
          subscriptionId: currentSubscriptionData.razorpay_subscription_id,
          paymentId: razorpayResponse?.razorpay_payment_id,
          planCode: selectedPlan.code,
          planLabel: selectedPlan.label,
          locLimit: selectedPlan.locLimit,
          total: totalPrice,
          billingNote: 'Billed monthly',
        });
      },
      modal: {
        ondismiss: () => {
          setIsProcessing(false);
          setIsConfirming(false);
          cleanupRazorpayOverlay();
        },
      },
      theme: RAZORPAY_THEME,
    };

    const rzp = new window.Razorpay(options);
    document.body.classList.add('razorpay-active');
    rzp.on('payment.failed', (response: any) => {
      const error = response.error || {};
      setFailureInfo({
        errorCode: error.code,
        errorDescription: error.description || 'Payment failed. Please try again.',
        errorStep: error.step,
        errorReason: error.reason,
        subscriptionId: currentSubscriptionData.razorpay_subscription_id,
        planCode: selectedPlan.code,
        planLabel: selectedPlan.label,
        locLimit: selectedPlan.locLimit,
        total: totalPrice,
      });
      setIsProcessing(false);
      setIsConfirming(false);
      cleanupRazorpayOverlay();
    });
    rzp.open();
  };

  // Show loader overlay while confirming payment
  if (isConfirming) {
    return (
      <div className="min-h-screen bg-gradient-to-b from-slate-900 to-slate-800 flex items-center justify-center">
        <div className="text-center">
          <div className="inline-flex h-16 w-16 animate-spin rounded-full border-4 border-white/30 border-t-white mb-4" />
          <h2 className="text-2xl font-bold text-white mb-2">Confirming your purchase...</h2>
          <p className="text-slate-300">Please wait while we process your payment</p>
        </div>
      </div>
    );
  }

  if (successInfo) {
    return (
      <div className="min-h-screen bg-gradient-to-b from-slate-900 to-slate-800 py-12 px-4">
        <div className="max-w-xl mx-auto bg-slate-800 rounded-2xl border border-slate-700 p-10 text-center shadow-xl">
          <div className="flex items-center justify-center w-16 h-16 mx-auto rounded-full bg-emerald-500/10 border border-emerald-500/40 mb-6">
            <svg className="w-9 h-9 text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
            </svg>
          </div>
          <h1 className="text-3xl font-bold text-white mb-3">Payment Initiated Successfully! 🎉</h1>
          <p className="text-slate-300 mb-6">
            Your subscription and payment have been created successfully.
          </p>

          <div className="bg-slate-900/60 border border-slate-700 rounded-xl p-5 mb-6 text-left">
            <div className="flex items-start gap-3">
              <div className="flex-shrink-0 mt-0.5">
                <span className={`inline-block h-3 w-3 rounded-full ${
                  activationProgress.state === 'applied'
                    ? 'bg-emerald-400'
                    : activationProgress.state === 'delayed'
                    ? 'bg-amber-400'
                    : 'bg-blue-400'
                }`} />
              </div>
              <div className="flex-1">
                <h3 className="text-sm font-semibold text-white mb-1">Activation Status</h3>
                <p className="text-sm text-slate-300">{activationProgress.message}</p>
                {activationProgress.lastCheckedAt && (
                  <p className="text-xs text-slate-400 mt-2">Last checked: {new Date(activationProgress.lastCheckedAt).toLocaleString()}</p>
                )}
              </div>
            </div>
          </div>

          <div className="bg-slate-900/60 border border-slate-700 rounded-xl p-6 text-left mb-8">
            <dl className="space-y-4">
              <div className="flex justify-between text-slate-300">
                <dt>Plan</dt>
                <dd className="text-white font-semibold">{successInfo.planCode}</dd>
              </div>
              <div className="flex justify-between text-slate-300">
                <dt>Monthly LOC</dt>
                <dd className="text-white font-semibold">{successInfo.locLimit.toLocaleString()}</dd>
              </div>
              <div className="flex justify-between text-slate-300">
                <dt>Reference price (USD)</dt>
                <dd className="text-white font-semibold">${successInfo.total}</dd>
              </div>
              <div className="flex justify-between text-slate-300">
                <dt>Checkout currency</dt>
                <dd className="text-white font-semibold">{selectedCurrency}</dd>
              </div>
              <div className="flex justify-between text-slate-300">
                <dt>Billing</dt>
                <dd className="text-white font-semibold">{successInfo.billingNote}</dd>
              </div>
              <div className="flex justify-between text-slate-300">
                <dt>Subscription ID</dt>
                <dd className="text-white font-mono text-sm break-all">{successInfo.subscriptionId}</dd>
              </div>
              {successInfo.paymentId && (
                <div className="flex justify-between text-slate-300">
                  <dt>Payment ID</dt>
                  <dd className="text-white font-mono text-sm break-all">{successInfo.paymentId}</dd>
                </div>
              )}
            </dl>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <button
              type="button"
              onClick={() => navigate('/settings#subscriptions')}
              className="w-full inline-flex items-center justify-center px-5 py-3 bg-blue-600 hover:bg-blue-700 text-white font-semibold rounded-lg transition-colors shadow-lg"
            >
              View Activation in Subscription Settings
            </button>
            <button
              type="button"
              onClick={() => navigate('/dashboard')}
              className="w-full inline-flex items-center justify-center px-5 py-3 bg-slate-700 hover:bg-slate-600 text-white font-semibold rounded-lg transition-colors"
            >
              Go to dashboard
            </button>
          </div>
        </div>
      </div>
    );
  }

  // Payment failure page
  if (failureInfo) {
    const getErrorHelpText = (errorCode?: string) => {
      const helpMessages: Record<string, string> = {
        'BAD_REQUEST_ERROR': 'There was an issue with the payment request. Please check your payment details and try again.',
        'GATEWAY_ERROR': 'The payment gateway encountered an error. This is usually temporary—please try again in a few moments.',
        'SERVER_ERROR': 'Our server encountered an error. Please try again in a few moments.',
        'INVALID_CARD': 'The card details entered are invalid. Please verify your card number, expiry date, and CVV.',
        'INSUFFICIENT_FUNDS': 'Your card has insufficient funds. Please try a different payment method.',
        'CARD_DECLINED': 'Your card was declined by the bank. Please contact your bank or try a different card.',
        'AUTHENTICATION_ERROR': '3D Secure authentication failed. Please try again or use a different card.',
        'TRANSACTION_DECLINED': 'The transaction was declined by your bank. Please contact your bank for more details.',
      };
      return helpMessages[errorCode || ''] || 'Please check your payment details and try again.';
    };

    return (
      <div className="min-h-screen bg-gradient-to-b from-slate-900 to-slate-800 py-12 px-4">
        <div className="max-w-xl mx-auto bg-slate-800 rounded-2xl border border-slate-700 p-10 text-center shadow-xl">
          <div className="flex items-center justify-center w-16 h-16 mx-auto rounded-full bg-rose-500/10 border border-rose-500/40 mb-6">
            <svg className="w-9 h-9 text-rose-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </div>
          <h1 className="text-3xl font-bold text-white mb-3">Payment Failed</h1>
          <p className="text-slate-300 mb-6">
            We couldn't process your payment. Don't worry—you can try again with the same or a different payment method.
          </p>

          {/* Error Details */}
          <div className="bg-rose-900/20 border-2 border-rose-500/40 rounded-lg p-5 mb-6 text-left">
            <h3 className="text-lg font-bold text-rose-300 mb-3">Error Details</h3>
            <div className="space-y-2 text-sm">
              <p className="text-rose-100">
                <strong>Description:</strong> {failureInfo.errorDescription}
              </p>
              {failureInfo.errorCode && (
                <p className="text-rose-100">
                  <strong>Error Code:</strong> {failureInfo.errorCode}
                </p>
              )}
              {failureInfo.errorStep && (
                <p className="text-rose-100">
                  <strong>Failed at:</strong> {failureInfo.errorStep}
                </p>
              )}
              {failureInfo.errorReason && (
                <p className="text-rose-100">
                  <strong>Reason:</strong> {failureInfo.errorReason}
                </p>
              )}
            </div>
          </div>

          {/* Helpful Tips */}
          <div className="bg-blue-900/20 border border-blue-500/40 rounded-lg p-5 mb-8 text-left">
            <h3 className="text-lg font-bold text-blue-300 mb-3">💡 What you can do</h3>
            <div className="space-y-2 text-sm text-blue-100">
              <p><strong>• {getErrorHelpText(failureInfo.errorCode)}</strong></p>
              <p>• Ensure your card has sufficient balance and is activated for online transactions</p>
              <p>• Check if your card supports international payments (if applicable)</p>
              <p>• Try using a different card or payment method</p>
              <p>• Contact your bank if the issue persists—they may have blocked the transaction</p>
              <p>• Make sure your billing address matches your card details</p>
            </div>
          </div>

          {/* Order Summary */}
          <div className="bg-slate-900/60 border border-slate-700 rounded-xl p-6 text-left mb-8">
            <h3 className="text-lg font-semibold text-white mb-4">Order Summary</h3>
            <dl className="space-y-3">
              <div className="flex justify-between text-slate-300">
                <dt>Plan</dt>
                <dd className="text-white font-semibold">{failureInfo.planCode}</dd>
              </div>
              <div className="flex justify-between text-slate-300">
                <dt>Monthly LOC</dt>
                <dd className="text-white font-semibold">{failureInfo.locLimit.toLocaleString()}</dd>
              </div>
              <div className="flex justify-between text-slate-300">
                <dt>Amount</dt>
                <dd className="text-white font-semibold">${failureInfo.total}</dd>
              </div>
              {failureInfo.subscriptionId && (
                <div className="flex justify-between text-slate-300">
                  <dt>Subscription ID</dt>
                  <dd className="text-white font-mono text-xs break-all">{failureInfo.subscriptionId}</dd>
                </div>
              )}
            </dl>
          </div>

          {/* Action Buttons */}
          <div className="flex flex-col sm:flex-row gap-4 justify-center">
            <button
              type="button"
              onClick={handleRetryPayment}
              disabled={isProcessing}
              className="flex-1 sm:flex-none px-8 py-4 bg-blue-600 hover:bg-blue-700 text-white font-bold rounded-lg transition-colors shadow-lg text-lg disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isProcessing ? (
                <span className="inline-flex items-center gap-2">
                  <span className="inline-flex h-4 w-4 animate-spin rounded-full border-2 border-white/60 border-t-transparent" />
                  Processing...
                </span>
              ) : (
                'Retry Payment'
              )}
            </button>
            <button
              type="button"
              onClick={() => navigate('/subscribe')}
              className="flex-1 sm:flex-none px-6 py-3 bg-slate-700 hover:bg-slate-600 text-white font-semibold rounded-lg transition-colors"
            >
              Back to Pricing
            </button>
          </div>

          {/* Support Info */}
          <div className="mt-6 pt-6 border-t border-slate-700">
            <p className="text-slate-400 text-sm">
              Need help? Contact our support team at{' '}
              <a href="mailto:support@livereview.io" className="text-blue-400 hover:text-blue-300 underline">
                support@livereview.io
              </a>
            </p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gradient-to-b from-slate-900 to-slate-800 py-12 px-4">
      <div className="max-w-2xl mx-auto">
        {/* Header */}
        <div className="text-center mb-8">
          <button
            onClick={() => navigate('/subscribe')}
            className="inline-flex items-center text-slate-400 hover:text-white mb-4 transition-colors"
          >
            <svg className="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
            Back to pricing
          </button>
          <h1 className="text-3xl font-bold text-white mb-2">
            Complete Your Purchase
          </h1>
          <p className="text-slate-300">
            Monthly LOC Slab
          </p>
        </div>

        {/* Main Card */}
        <div className="bg-slate-800 rounded-lg border border-slate-700 p-8 shadow-xl">
          {!contractReady && errorMessage && (
            <div className="mb-6 rounded-lg border border-amber-500/50 bg-amber-900/20 p-4 text-sm text-amber-100">
              <p className="font-semibold text-amber-200">Backend version warning</p>
              <p className="mt-1">{errorMessage}</p>
              <p className="mt-1">You can still continue and attempt payment.</p>
            </div>
          )}

          {/* Plan Summary */}
          <div className="mb-8 pb-8 border-b border-slate-700">
            <div className="flex items-baseline mb-2">
              <span className="text-4xl font-bold text-white">
                ${selectedPlan.monthlyPriceUSD}
              </span>
              <span className="text-slate-400 ml-2">
                /month
              </span>
            </div>
            <p className="text-emerald-400 text-sm font-semibold">
              {selectedPlan.locLimit.toLocaleString()} LOC included monthly
            </p>
          </div>

          {/* Slab Selector */}
          <div className="mb-8">
            <label className="block text-lg font-semibold text-white mb-4">
              Choose your LOC slab
            </label>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              {LOC_SLABS.map((slab) => {
                const isSelected = slab.code === selectedPlanCode;
                return (
                  <button
                    key={slab.code}
                    type="button"
                    onClick={() => setSelectedPlanCode(slab.code)}
                    className={`text-left rounded-lg border px-4 py-3 transition-colors ${
                      isSelected
                        ? 'border-blue-500 bg-blue-900/30'
                        : 'border-slate-600 bg-slate-700/40 hover:border-slate-500'
                    }`}
                  >
                    <p className="text-white font-semibold">{slab.label}</p>
                    <p className="text-slate-300 text-sm">${slab.monthlyPriceUSD}/month</p>
                  </button>
                );
              })}
            </div>
          </div>

          {/* Price Breakdown */}
          <div className="mb-8 p-6 bg-slate-900/50 rounded-lg border border-slate-700">
            <h3 className="text-lg font-semibold text-white mb-4">Order Summary</h3>
            <div className="space-y-3">
              <div className="flex justify-between text-slate-300">
                <span>{selectedPlan.label} monthly slab</span>
                <span className="font-semibold">${totalPrice}</span>
              </div>
              <div className="flex justify-between text-slate-300">
                <span>Included LOC / month</span>
                <span className="font-semibold">{selectedPlan.locLimit.toLocaleString()}</span>
              </div>
              <div className="pt-3 border-t border-slate-700 flex justify-between text-white text-xl font-bold">
                <span>Total</span>
                <span>${totalPrice}</span>
              </div>
              <p className="text-slate-400 text-sm">
                Billed monthly
              </p>
            </div>
          </div>

          {/* Action Buttons */}
          <div className="flex gap-4">
            <button
              type="button"
              onClick={() => navigate('/subscribe')}
              className="flex-1 px-6 py-3 bg-slate-700 hover:bg-slate-600 text-white font-semibold rounded-lg transition-colors"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={handlePurchase}
              disabled={isProcessing}
              className="flex-1 px-6 py-3 bg-blue-600 hover:bg-blue-700 text-white font-semibold rounded-lg transition-colors shadow-lg disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isProcessing ? (
                <span className="inline-flex items-center gap-2">
                  <span className="inline-flex h-4 w-4 animate-spin rounded-full border-2 border-white/60 border-t-transparent" />
                  Processing...
                </span>
              ) : (
                `Purchase ${selectedPlan.label} for $${totalPrice}/month`
              )}
            </button>
          </div>

          {/* Additional Info */}
          <div className="mt-6 pt-6 border-t border-slate-700">
            <p className="text-slate-400 text-sm text-center">
              LOC quota updates after payment capture confirmation
            </p>
          </div>
        </div>

        {/* Features Reminder */}
        <div className="mt-8 bg-slate-800/50 rounded-lg border border-slate-700 p-6">
          <h3 className="text-lg font-semibold text-white mb-4">What's included:</h3>
          <ul className="grid grid-cols-1 md:grid-cols-2 gap-3 text-slate-300 text-sm">
            <li className="flex items-center gap-2">
              <svg className="w-5 h-5 text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
              </svg>
              Unlimited team members
            </li>
            <li className="flex items-center gap-2">
              <svg className="w-5 h-5 text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
              </svg>
              Unlimited reviews
            </li>
            <li className="flex items-center gap-2">
              <svg className="w-5 h-5 text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
              </svg>
              Multiple organizations
            </li>
            <li className="flex items-center gap-2">
              <svg className="w-5 h-5 text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
              </svg>
              Hosted auto model with optional BYOK
            </li>
          </ul>
        </div>
      </div>
    </div>
  );
};

export default TeamCheckout;
