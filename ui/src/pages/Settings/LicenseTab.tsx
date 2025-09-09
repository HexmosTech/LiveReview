import React from 'react';
import { useAppSelector, useAppDispatch } from '../../store/configureStore';
import { triggerLicenseRefresh, submitLicenseToken } from '../../store/License/slice';

const roleCanView = (role?: string) => role === 'super_admin' || role === 'owner';

const LicenseTab: React.FC = () => {
  const dispatch = useAppDispatch();
  const license = useAppSelector(s => s.License);
  const auth = useAppSelector(s => s.Auth);
  const activeOrg = auth.organizations[0];
  const canView = roleCanView(activeOrg?.role);

  if (!canView) {
    return (
      <div className="p-6 text-sm text-slate-300" data-testid="license-tab-deny">
        You don't have permission to view license details.
      </div>
    );
  }

  const handleRefresh = () => dispatch(triggerLicenseRefresh());
  const handleReplace = () => {
    // naive prompt for now; full UI handled by modal in app
    const token = window.prompt('Paste new license token');
    if (token) dispatch(submitLicenseToken(token));
  };

  return (
    <div className="p-6 space-y-6" data-testid="license-tab">
      <div>
        <h2 className="text-lg font-semibold text-white mb-2">License Overview</h2>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 text-sm">
          <div className="bg-slate-800/60 border border-slate-700 rounded p-4">
            <div className="text-slate-400 mb-1">Status</div>
            <div className="font-medium text-slate-200">{license.status}</div>
          </div>
          <div className="bg-slate-800/60 border border-slate-700 rounded p-4">
            <div className="text-slate-400 mb-1">Subject</div>
            <div className="font-mono text-slate-300 break-all">{license.subject || '—'}</div>
          </div>
          <div className="bg-slate-800/60 border border-slate-700 rounded p-4">
            <div className="text-slate-400 mb-1">App Name</div>
            <div className="text-slate-200">{license.appName || '—'}</div>
          </div>
          <div className="bg-slate-800/60 border border-slate-700 rounded p-4">
            <div className="text-slate-400 mb-1">Seats</div>
            <div className="text-slate-200">{license.unlimited ? 'Unlimited' : (license.seatCount ?? '—')}</div>
          </div>
          <div className="bg-slate-800/60 border border-slate-700 rounded p-4">
            <div className="text-slate-400 mb-1">Expires At</div>
            <div className="text-slate-200">{license.expiresAt || '—'}</div>
          </div>
          <div className="bg-slate-800/60 border border-slate-700 rounded p-4">
            <div className="text-slate-400 mb-1">Last Validated</div>
            <div className="text-slate-200">{license.lastValidatedAt || '—'}</div>
          </div>
          <div className="bg-slate-800/60 border border-slate-700 rounded p-4">
            <div className="text-slate-400 mb-1">Validation Code</div>
            <div className="text-slate-200">{license.lastValidationCode || '—'}</div>
          </div>
        </div>
      </div>
      <div className="flex gap-3">
        <button
          onClick={handleRefresh}
          disabled={license.refreshing}
          className="px-4 py-2 text-sm rounded bg-slate-700 hover:bg-slate-600 text-slate-200 disabled:opacity-50"
        >
          {license.refreshing ? 'Refreshing…' : 'Refresh'}
        </button>
        <button
          onClick={handleReplace}
          className="px-4 py-2 text-sm rounded bg-blue-600 hover:bg-blue-500 text-white"
        >
          Replace Token
        </button>
      </div>
      <p className="text-xs text-slate-500">Token replacement opens a prompt for now; future enhancement may reuse the main modal contextually.</p>
    </div>
  );
};

export default LicenseTab;
