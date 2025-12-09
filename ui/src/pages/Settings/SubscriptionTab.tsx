import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { isCloudMode } from '../../utils/deploymentMode';
import { useAppSelector } from '../../store/configureStore';
import { useOrgContext } from '../../hooks/useOrgContext';
import LicenseManagement from '../Licenses/LicenseManagement';

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
  const { currentOrg } = useOrgContext();
  
  // Read plan from current org (org-scoped), not from Auth.user
  const planType = currentOrg?.plan_type || 'free';
  const licenseExpiresAt = currentOrg?.license_expires_at;
  const isTeamPlan = planType === 'team';
  const isFree = planType === 'free';

  const formatDate = (dateString: string | null | undefined) => {
    if (!dateString) return null;
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
    });
  };

  const getPlanDisplayName = (plan: string) => {
    if (plan === 'team') return 'Team Plan';
    return 'Free Plan';
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

      {/* Current Plan Section */}
      <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h3 className="text-md font-semibold text-white">Current Plan</h3>
            <p className="text-sm text-slate-400 mt-1">{getPlanDisplayName(planType)}</p>
          </div>
          <div className={`px-4 py-2 rounded-lg text-sm font-medium ${
            isTeamPlan 
              ? 'bg-blue-900/40 text-blue-300' 
              : 'bg-emerald-900/40 text-emerald-300'
          }`}>
            Active
          </div>
        </div>

        {isTeamPlan && licenseExpiresAt && (
          <div className="mb-4 p-3 bg-slate-900/60 border border-slate-700 rounded-lg">
            <div className="text-slate-400 text-xs mb-1">License Expires</div>
            <div className="text-white font-medium">{formatDate(licenseExpiresAt)}</div>
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
          <h3 className="text-md font-semibold text-white mb-2">Upgrade to Team Plan</h3>
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

      {/* Team Plan Benefits - show for team users */}
      {isTeamPlan && (
        <div className="bg-gradient-to-r from-blue-900/40 to-purple-900/40 border border-blue-700/50 rounded-lg p-6">
          <h3 className="text-md font-semibold text-white mb-2">Team Plan Benefits</h3>
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
