import React, { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { isCloudMode } from '../../utils/deploymentMode';

const SubscriptionTab: React.FC = () => {
  const navigate = useNavigate();

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
            <p className="text-sm text-slate-400 mt-1">Free Plan</p>
          </div>
          <div className="px-4 py-2 bg-emerald-900/40 text-emerald-300 rounded-lg text-sm font-medium">
            Active
          </div>
        </div>

        <div className="space-y-3 text-sm">
          <div className="flex justify-between items-center py-2 border-b border-slate-700">
            <span className="text-slate-400">Daily Review Limit</span>
            <span className="text-white font-medium">3 reviews per day</span>
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
            <span className="text-slate-500">Not included</span>
          </div>
        </div>
      </div>

      {/* Upgrade Section */}
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

      {/* Billing History Placeholder */}
      <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-6">
        <h3 className="text-md font-semibold text-white mb-2">Billing History</h3>
        <p className="text-sm text-slate-400">
          No billing history available for free plan
        </p>
      </div>
    </div>
  );
};

export default SubscriptionTab;
