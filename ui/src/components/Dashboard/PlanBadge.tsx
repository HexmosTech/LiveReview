import React from 'react';
import { useNavigate } from 'react-router-dom';
import { useAppSelector } from '../../store/configureStore';

export const PlanBadge: React.FC = () => {
  const navigate = useNavigate();
  const user = useAppSelector(state => state.Auth.user);

  if (!user) return null;

  const planType = user.plan_type || 'free';
  const licenseExpiresAt = user.license_expires_at;
  const isTeamPlan = planType === 'team';
  const isFree = planType === 'free';

  const formatDate = (dateString: string | null | undefined) => {
    if (!dateString) return '';
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
    });
  };

  return (
    <div className="bg-slate-800 rounded-lg border border-slate-700 p-6">
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-3 mb-2">
            <h3 className="text-lg font-semibold text-white">Current Plan</h3>
            <span
              className={`px-3 py-1 text-sm font-bold rounded-full ${
                isTeamPlan
                  ? 'bg-blue-500/10 text-blue-400 border border-blue-500/40'
                  : 'bg-slate-700 text-slate-300'
              }`}
            >
              {isTeamPlan ? 'Team' : 'Free'}
            </span>
          </div>

          {isTeamPlan && licenseExpiresAt && (
            <p className="text-sm text-slate-400">
              License expires:{' '}
              <span className="text-white font-medium">
                {formatDate(licenseExpiresAt)}
              </span>
            </p>
          )}

          {isFree && (
            <p className="text-sm text-slate-400">
              Limited to 3 reviews per day
            </p>
          )}
        </div>

        {isFree && (
          <button
            onClick={() => navigate('/subscribe')}
            className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white text-sm font-semibold rounded-lg transition-colors"
          >
            Upgrade to Team
          </button>
        )}

        {isTeamPlan && (
          <button
            onClick={() => navigate('/subscribe/manage')}
            className="px-4 py-2 bg-slate-700 hover:bg-slate-600 text-white text-sm font-semibold rounded-lg transition-colors"
          >
            Manage Licenses
          </button>
        )}
      </div>
    </div>
  );
};
