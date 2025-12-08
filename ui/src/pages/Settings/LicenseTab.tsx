import React, { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import toast from 'react-hot-toast';
import { useAppSelector, useAppDispatch } from '../../store/configureStore';
import { triggerLicenseRefresh, triggerLicenseRevalidation, triggerLicenseDelete, openModal as openLicenseModal, openDeleteConfirm, closeDeleteConfirm } from '../../store/License/slice';
import { logout } from '../../store/Auth/reducer';
import { isCloudMode } from '../../utils/deploymentMode';

const roleCanView = (role?: string) => role === 'super_admin' || role === 'owner';

const LicenseTab: React.FC = () => {
  const dispatch = useAppDispatch();
  const navigate = useNavigate();
  const license = useAppSelector(s => s.License);
  const auth = useAppSelector(s => s.Auth);
  const activeOrg = auth.organizations[0];
  const canView = roleCanView(activeOrg?.role);

  // Redirect to subscription tab if in cloud mode
  useEffect(() => {
    if (isCloudMode()) {
      navigate('/settings#subscriptions', { replace: true });
    }
  }, [navigate]);

  // Don't render anything if in cloud mode (will redirect)
  if (isCloudMode()) {
    return null;
  }

  if (!canView) {
    return (
      <div className="p-6 text-sm text-slate-300" data-testid="license-tab-deny">
        You don't have permission to view license details.
      </div>
    );
  }

  const handleRefresh = () => dispatch(triggerLicenseRefresh());
  const handleReplace = () => {
    dispatch(openLicenseModal());
  };

  const handleRevalidate = async () => {
    try {
      const result = await dispatch(triggerLicenseRevalidation()).unwrap();
      const statusMessages: Record<string, string> = {
        active: 'License validated successfully! Your license is active.',
        warning: 'License validated with warnings. Please check the status.',
        grace: 'License is in grace period. Validation issues detected.',
        expired: 'License has expired. Please renew your license.',
        invalid: 'License validation failed. The license is invalid.',
        missing: 'No license found. Please enter a valid license.'
      };
      const message = statusMessages[result.status] || 'License validation completed.';
      
      if (result.status === 'active') {
        toast.success(message);
      } else if (result.status === 'warning' || result.status === 'grace') {
        toast(message, { icon: '⚠️' });
      } else {
        toast.error(message);
      }
    } catch (error: any) {
      toast.error(`Validation failed: ${error.message || 'Unknown error occurred'}`);
    }
  };

  const handleDeleteClick = () => {
    dispatch(openDeleteConfirm());
  };

  const handleDeleteConfirm = async () => {
    try {
      await dispatch(triggerLicenseDelete()).unwrap();
      toast.success('License deleted successfully. Logging out...');
      // Wait a moment for the user to see the message, then logout and reload
      setTimeout(async () => {
        await dispatch(logout());
        // Force a full page reload to reset the app state and show login
        window.location.href = '/';
      }, 1500);
    } catch (error: any) {
      toast.error(`Failed to delete license: ${error.message || 'Unknown error occurred'}`);
    }
  };

  const handleDeleteCancel = () => {
    dispatch(closeDeleteConfirm());
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
          Replace Licence
        </button>
      </div>
      
      {/* Delete License Section */}
      <div className="pt-6 border-t border-slate-700">
        <h3 className="text-sm font-semibold text-slate-300 mb-2">Danger Zone</h3>
        <p className="text-xs text-slate-400 mb-3">
          Deleting the license will remove it from the system and log you out of LiveReview. Use this only when absolutely necessary.
        </p>
        <button
          onClick={handleDeleteClick}
          disabled={license.status === 'missing'}
          className="px-4 py-2 text-sm rounded bg-red-600 hover:bg-red-500 text-white disabled:opacity-50 disabled:cursor-not-allowed"
        >
          Delete License
        </button>
      </div>

      {/* Delete Confirmation Modal */}
      {license.deleteConfirmOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60" onClick={handleDeleteCancel}>
          <div 
            className="bg-slate-800 border border-slate-700 rounded-lg shadow-xl max-w-md w-full mx-4 p-6"
            onClick={(e) => e.stopPropagation()}
          >
            {license.deleting ? (
              <div className="text-center py-8">
                <div className="inline-block w-12 h-12 border-4 border-slate-600 border-t-red-500 rounded-full animate-spin mb-4"></div>
                <p className="text-slate-200 text-sm">Deleting license...</p>
              </div>
            ) : (
              <>
                <div className="flex items-start mb-4">
                  <div className="flex-shrink-0 w-12 h-12 rounded-full bg-red-500/10 flex items-center justify-center mr-4">
                    <svg className="w-6 h-6 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                    </svg>
                  </div>
                  <div className="flex-1">
                    <h3 className="text-lg font-semibold text-white mb-2">Delete License</h3>
                    <p className="text-sm text-slate-300 mb-4">
                      Are you sure you want to delete this license? This action will:
                    </p>
                    <ul className="text-sm text-slate-400 space-y-1 mb-4 list-disc list-inside">
                      <li>Permanently remove the license from the system</li>
                      <li>Log you out immediately</li>
                      <li>Require a new license to continue using the application</li>
                    </ul>
                    <p className="text-sm text-red-400 font-medium">
                      ⚠️ This action cannot be undone. Use only when absolutely necessary.
                    </p>
                  </div>
                </div>
                <div className="flex gap-3 justify-end mt-6">
                  <button
                    onClick={handleDeleteCancel}
                    className="px-4 py-2 text-sm rounded bg-slate-700 hover:bg-slate-600 text-slate-200"
                  >
                    Cancel
                  </button>
                  <button
                    onClick={handleDeleteConfirm}
                    className="px-4 py-2 text-sm rounded bg-red-600 hover:bg-red-500 text-white font-medium"
                  >
                    Yes, Delete License
                  </button>
                </div>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  );
};

export default LicenseTab;
