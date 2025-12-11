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

type CheckoutSuccess = {
  subscriptionId: string;
  paymentId?: string;
  planLabel: string;
  seats: number;
  total: number;
  billingNote: string;
};

const TeamCheckout: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { currentOrgId, userOrganizations } = useOrgContext();
  const { organizations: authOrgs } = useAppSelector((state) => state.Auth);
  
  const period = searchParams.get('period') || 'annual';
  const [seats, setSeats] = useState<number>(5);
  const [isProcessing, setIsProcessing] = useState<boolean>(false);
  const [isConfirming, setIsConfirming] = useState<boolean>(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [successInfo, setSuccessInfo] = useState<CheckoutSuccess | null>(null);

  const isAnnual = period === 'annual';
  const pricePerSeat = isAnnual ? 60 : 6;
  const totalPrice = seats * pricePerSeat;
  const savingsPerSeat = isAnnual ? 12 : 0;
  const totalSavings = seats * savingsPerSeat;

  // Get org ID - try currentOrgId first, then fall back to first auth org
  const orgId = currentOrgId || (authOrgs && authOrgs.length > 0 ? authOrgs[0].id : null);

  useEffect(() => {
    // Verify user is authenticated
    const token = localStorage.getItem('accessToken');
    if (!token) {
      navigate('/signin', { state: { returnTo: `/checkout/team?period=${period}` } });
    }
  }, [navigate, period]);

  const handlePurchase = async () => {
    setIsProcessing(true);
    setErrorMessage(null);
    setSuccessInfo(null);

    try {
      const token = localStorage.getItem('accessToken');
      if (!token) {
        setErrorMessage('Please sign in to continue');
        setIsProcessing(false);
        return;
      }

      if (!currentOrgId) {
        setErrorMessage('No organization selected. Please switch to an organization and try again.');
        setIsProcessing(false);
        return;
      }

      let data: any;
      try {
        // Use apiClient which automatically adds X-Org-Context from Redux store
        data = await apiClient.post('/subscriptions', {
          plan_type: isAnnual ? 'team_annual' : 'team_monthly',
          quantity: seats,
        });
      } catch (err: any) {
        // Fall back to direct fetch with explicit headers if org context error occurs
        if (err?.message?.toLowerCase().includes('organization context')) {
          const response = await fetch('/api/v1/subscriptions', {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${token}`,
              'X-Org-Context': currentOrgId.toString(),
            },
            body: JSON.stringify({
              plan_type: isAnnual ? 'team_annual' : 'team_monthly',
              quantity: seats,
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

      // Initialize Razorpay checkout
      const options = {
        key: data.razorpay_key_id,
        subscription_id: data.razorpay_subscription_id,
        name: 'LiveReview',
        description: `Team ${isAnnual ? 'Annual' : 'Monthly'} - ${seats} ${seats === 1 ? 'seat' : 'seats'}`,
        image: '/assets/logo-with-text.svg',
        handler: async (razorpayResponse: any) => {
          // Show loader while confirming purchase
          setIsConfirming(true);
          
          // Immediately confirm the purchase to prevent race conditions with webhooks
          try {
            await apiClient.post('/subscriptions/confirm-purchase', {
              razorpay_subscription_id: data.razorpay_subscription_id,
              razorpay_payment_id: razorpayResponse?.razorpay_payment_id,
            });
          } catch (confirmError) {
            console.error('Failed to confirm purchase (non-blocking):', confirmError);
            // Don't block the success flow - webhooks will eventually process the payment
          }

          setIsProcessing(false);
          setIsConfirming(false);
          setSuccessInfo({
            subscriptionId: data.razorpay_subscription_id,
            paymentId: razorpayResponse?.razorpay_payment_id,
            planLabel: `Team ${isAnnual ? 'Annual' : 'Monthly'} Plan`,
            seats,
            total: totalPrice,
            billingNote: isAnnual ? 'Billed annually' : 'Billed monthly',
          });
        },
        modal: {
          ondismiss: () => {
            setIsProcessing(false);
            setIsConfirming(false);
          },
        },
        theme: {
          color: '#3B82F6',
        },
      };

      const rzp = new window.Razorpay(options);
      rzp.on('payment.failed', (response: any) => {
        setErrorMessage(`Payment failed: ${response.error.description}`);
        setIsProcessing(false);
        setIsConfirming(false);
      });
      rzp.open();
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : 'An error occurred');
      setIsProcessing(false);
      setIsConfirming(false);
    }
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
          <h1 className="text-3xl font-bold text-white mb-3">Purchase Successful! 🎉</h1>
          <p className="text-slate-300 mb-6">
            Your subscription has been created successfully.
          </p>

          {/* Important Notice - Seat Assignment */}
          <div className="bg-amber-900/20 border-2 border-amber-500/60 rounded-lg p-5 mb-8">
            <div className="flex items-start gap-3">
              <div className="flex-shrink-0 mt-0.5">
                <svg className="w-6 h-6 text-amber-400" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
                </svg>
              </div>
              <div className="flex-1">
                <h3 className="text-lg font-bold text-amber-300 mb-2">Important: Assign Seats to Activate</h3>
                <p className="text-amber-100 text-sm leading-relaxed mb-3">
                  Your subscription is created, but <strong>no seats are assigned yet</strong>. 
                  Team members won't have access to premium features until you explicitly assign licenses to them.
                </p>
                <p className="text-amber-100 text-sm leading-relaxed">
                  Click <strong>"Assign Team Licenses"</strong> below to manage seat assignments.
                </p>
              </div>
            </div>
          </div>

          <div className="bg-slate-900/60 border border-slate-700 rounded-xl p-6 text-left mb-8">
            <dl className="space-y-4">
              <div className="flex justify-between text-slate-300">
                <dt>Plan</dt>
                <dd className="text-white font-semibold">{successInfo.planLabel}</dd>
              </div>
              <div className="flex justify-between text-slate-300">
                <dt>Seats purchased</dt>
                <dd className="text-white font-semibold">{successInfo.seats}</dd>
              </div>
              <div className="flex justify-between text-slate-300">
                <dt>Total</dt>
                <dd className="text-white font-semibold">${successInfo.total}</dd>
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

          <div className="flex flex-col sm:flex-row gap-4 justify-center">
            <button
              type="button"
              onClick={() => navigate(`/subscribe/subscriptions/${successInfo.subscriptionId}/assign`)}
              className="flex-1 sm:flex-none px-8 py-4 bg-blue-600 hover:bg-blue-700 text-white font-bold rounded-lg transition-colors shadow-lg text-lg"
            >
              Assign Team Licenses →
            </button>
            <button
              type="button"
              onClick={() => navigate('/dashboard')}
              className="flex-1 sm:flex-none px-6 py-3 bg-slate-700 hover:bg-slate-600 text-white font-semibold rounded-lg transition-colors"
            >
              Go to dashboard
            </button>
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
            Team {isAnnual ? 'Annual' : 'Monthly'} Plan
          </p>
        </div>

        {/* Main Card */}
        <div className="bg-slate-800 rounded-lg border border-slate-700 p-8 shadow-xl">
          {/* Plan Summary */}
          <div className="mb-8 pb-8 border-b border-slate-700">
            <div className="flex items-baseline mb-2">
              <span className="text-4xl font-bold text-white">
                ${isAnnual ? '60' : '6'}
              </span>
              <span className="text-slate-400 ml-2">
                /user/{isAnnual ? 'year' : 'month'}
              </span>
            </div>
            {isAnnual && (
              <p className="text-emerald-400 text-sm font-semibold">
                Save $12/user/year (17% off)
              </p>
            )}
          </div>

          {/* Seat Selector */}
          <div className="mb-8">
            <label className="block text-lg font-semibold text-white mb-4">
              How many team members?
            </label>
            <div className="flex items-center gap-4">
              <button
                type="button"
                onClick={() => setSeats(Math.max(1, seats - 1))}
                disabled={seats <= 1}
                className="w-12 h-12 flex items-center justify-center bg-slate-700 hover:bg-slate-600 text-white rounded-lg transition-colors text-xl font-bold disabled:opacity-30 disabled:cursor-not-allowed"
              >
                −
              </button>
              <div className="flex-1 max-w-xs">
                <input
                  type="number"
                  min="1"
                  value={seats}
                  onChange={(e) => setSeats(Math.max(1, parseInt(e.target.value) || 1))}
                  className="w-full px-4 py-3 bg-slate-700 text-white text-center text-2xl font-bold rounded-lg border border-slate-600 focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
                <p className="text-center text-slate-400 text-sm mt-2">
                  {seats === 1 ? '1 seat' : `${seats} seats`}
                </p>
              </div>
              <button
                type="button"
                onClick={() => setSeats(seats + 1)}
                className="w-12 h-12 flex items-center justify-center bg-slate-700 hover:bg-slate-600 text-white rounded-lg transition-colors text-xl font-bold"
              >
                +
              </button>
            </div>
          </div>

          {/* Price Breakdown */}
          <div className="mb-8 p-6 bg-slate-900/50 rounded-lg border border-slate-700">
            <h3 className="text-lg font-semibold text-white mb-4">Order Summary</h3>
            <div className="space-y-3">
              <div className="flex justify-between text-slate-300">
                <span>{seats} × ${pricePerSeat} ({isAnnual ? 'annual' : 'monthly'})</span>
                <span className="font-semibold">${totalPrice}</span>
              </div>
              {isAnnual && totalSavings > 0 && (
                <div className="flex justify-between text-emerald-400">
                  <span>Annual savings</span>
                  <span className="font-semibold">−${totalSavings}</span>
                </div>
              )}
              <div className="pt-3 border-t border-slate-700 flex justify-between text-white text-xl font-bold">
                <span>Total</span>
                <span>${totalPrice}</span>
              </div>
              <p className="text-slate-400 text-sm">
                Billed {isAnnual ? 'annually' : 'monthly'}
              </p>
            </div>
          </div>

          {/* Error Message */}
          {errorMessage && (
            <div className="mb-6 p-4 bg-rose-500/10 border border-rose-500/40 rounded-lg">
              <p className="text-rose-300 text-sm">{errorMessage}</p>
            </div>
          )}

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
                `Purchase for $${totalPrice}`
              )}
            </button>
          </div>

          {/* Additional Info */}
          <div className="mt-6 pt-6 border-t border-slate-700">
            <p className="text-slate-400 text-sm text-center">
              You can assign licenses to team members after purchase
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
              Prioritized support
            </li>
          </ul>
        </div>
      </div>
    </div>
  );
};

export default TeamCheckout;
