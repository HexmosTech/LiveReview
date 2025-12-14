import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import moment from 'moment-timezone';
import { isCloudMode } from '../../utils/deploymentMode';
import { useAppSelector } from '../../store/configureStore';
import { useOrgContext } from '../../hooks/useOrgContext';
import LicenseManagement from '../Licenses/LicenseManagement';
import { CancelSubscriptionModal } from '../../components/Subscriptions';
import apiClient from '../../api/apiClient';

const SubscriptionTab: React.FC = () => {
  const navigate = useNavigate();
  const { currentOrg, isSuperAdmin } = useOrgContext();
  const [activeTab, setActiveTab] = useState<'overview' | 'assignments'>('overview');

  // Check if user can manage licenses (owner or super admin)
  const canManageLicenses = isSuperAdmin || currentOrg?.role === 'owner';

  useEffect(() => {
    // Redirect to license page if in self-hosted mode
    if (!isCloudMode()) {
      navigate('/settings/license', { replace: true });
    }
  }, [navigate]);

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
            onClick={() => setActiveTab('overview')}
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
              onClick={() => setActiveTab('assignments')}
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
  
  // Read plan from current org (org-scoped), not from Auth.user
  const planType = currentOrg?.plan_type || 'free';
  const licenseExpiresAt = currentOrg?.license_expires_at;
  const isTeamPlan = planType === 'team';
  const isFree = planType === 'free';

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

  const handleCancelSuccess = () => {
    // Reload the page to reflect updated subscription status
    window.location.reload();
  };

  const formatDate = (dateString: string | null | undefined) => {
    if (!dateString) return null;
    const userTimezone = moment.tz.guess();
    return moment.tz(dateString, userTimezone).format('MMM D, YYYY, h:mm A z');
  };

  const getPlanDisplayName = (plan: string) => {
    if (plan === 'team' || plan.includes('team')) return 'Team Subscription';
    return 'Free Subscription';
  };

  const dailyLimit = isTeamPlan ? 'Unlimited' : '3 reviews per day';

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold text-white mb-2">Subscription Management</h2>
        <p className="text-sm text-slate-400 mb-4">
          Manage your LiveReview Cloud subscription and billing
        </p>
      </div>

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
            Get unlimited reviews, priority support, and advanced features for your team
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
            onClick={() => navigate('/subscribe')}
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
