import React, { useEffect, useMemo, useState } from 'react';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { triggerLicenseRefresh } from '../../store/License/slice';
import { getSystemInfo } from '../../api/system';
import { Tooltip } from '../UIPrimitives';
import { isCloudMode } from '../../utils/deploymentMode';

export interface LicenseStatusBarProps {
  onOpenModal?: () => void;
}

// Map license status to styles & human labels
const STATUS_STYLES: Record<string, { label: string; bg: string; fg: string; accent: string; description?: string; } > = {
  active: { label: 'Active', bg: 'bg-emerald-900/40', fg: 'text-emerald-300', accent: 'bg-emerald-500', description: 'License valid' },
  missing: { label: 'Missing', bg: 'bg-amber-900/40', fg: 'text-amber-300', accent: 'bg-amber-500', description: 'A license is required' },
  warning: { label: 'Network Warning', bg: 'bg-yellow-900/40', fg: 'text-yellow-300', accent: 'bg-yellow-500', description: 'Recent validation failures' },
  grace: { label: 'Grace Period', bg: 'bg-orange-900/40', fg: 'text-orange-300', accent: 'bg-orange-500', description: 'Connectivity issues persist; days remaining limited' },
  expired: { label: 'Expired', bg: 'bg-red-900/40', fg: 'text-red-300', accent: 'bg-red-500', description: 'License expired — action required' },
  invalid: { label: 'Invalid', bg: 'bg-red-900/40', fg: 'text-red-300', accent: 'bg-red-500', description: 'Provided token invalid' },
};

const formatDaysLeft = (expiresAt?: string) => {
  if (!expiresAt) return undefined;
  const now = new Date();
  const exp = new Date(expiresAt);
  const diff = Math.ceil((exp.getTime() - now.getTime()) / (1000 * 60 * 60 * 24));
  if (diff < 0) return 'expired';
  if (diff === 0) return 'expires today';
  return `${diff} day${diff === 1 ? '' : 's'} left`;
};

const LicenseStatusBar: React.FC<LicenseStatusBarProps> = ({ onOpenModal }) => {
  // Don't render in cloud mode - subscription management replaces license UI
  if (isCloudMode()) {
    return null;
  }

  const dispatch = useAppDispatch();
  const license = useAppSelector(s => s.License);
  const isLoading = !license.loadedOnce || license.loading;
  const style = STATUS_STYLES[license.status] || STATUS_STYLES['missing'];
  const [deploymentMode, setDeploymentMode] = useState<'demo' | 'production' | 'unknown'>('unknown');
  const [showDemoInfo, setShowDemoInfo] = useState(false);
  const [firstLoadBannerShown, setFirstLoadBannerShown] = useState<boolean>(() => {
    try { return localStorage.getItem('lr_demo_firstload_shown') === '1'; } catch { return true; }
  });

  const daysLeft = useMemo(() => formatDaysLeft(license.expiresAt), [license.expiresAt]);
  const needsAction = !isLoading && ['missing','invalid','expired'].includes(license.status);

  const handleRefresh = () => {
    if (!license.refreshing) dispatch(triggerLicenseRefresh());
  };

  // Load deployment mode once
  useEffect(() => {
    let mounted = true;
    getSystemInfo().then(info => {
      if (!mounted) return;
      setDeploymentMode(info.deployment_mode);
      // Only auto-show the info the very first time in this browser
      if (info.deployment_mode === 'demo' && !firstLoadBannerShown) {
        setShowDemoInfo(true);
        try { localStorage.setItem('lr_demo_firstload_shown', '1'); } catch {}
      }
    }).catch(() => setDeploymentMode('unknown'));
    return () => { mounted = false; };
  }, []);

  // Loading state: avoid showing 'Missing' before the first real status arrives
  if (isLoading) {
    return (
      <div className="w-full border-b border-slate-800 bg-slate-800/40" data-testid="license-status-bar">
        <div className="container mx-auto px-4 text-xs py-1 flex items-center justify-between">
          <div className="flex items-center gap-2 text-slate-300">
            <span className="h-2 w-2 rounded-full bg-slate-400 animate-pulse" />
            Loading license…
          </div>
          <div />
        </div>
      </div>
    );
  }

  return (
    <div className={`w-full border-b border-slate-800 ${style.bg}`} data-testid="license-status-bar">
      <div className="container mx-auto px-4 text-xs py-1 flex items-center justify-between">
        {/* Left: All license-related info and actions */}
        <div className="flex items-center gap-4 flex-wrap">
          <span className={`inline-flex items-center gap-1 font-medium ${style.fg}`}>
            <span className={`h-2 w-2 rounded-full ${style.accent} animate-pulse`}></span>
            {style.label}
          </span>
          {style.description && (
            <span className="text-slate-400 hidden sm:inline" data-testid="license-desc">{style.description}</span>
          )}
          {daysLeft && license.status === 'active' && (
            <span className="text-slate-400" data-testid="license-days-left">{daysLeft}</span>
          )}
          <div className="flex items-center gap-3">
            {license.status === 'active' && (
              <button
                onClick={handleRefresh}
                disabled={license.refreshing}
                className="text-slate-400 hover:text-slate-200 disabled:opacity-40"
                aria-label="Refresh license"
              >
                {license.refreshing ? 'Refreshing…' : 'Refresh'}
              </button>
            )}
            <button
              onClick={onOpenModal}
              className={`underline ${needsAction ? 'text-amber-300 hover:text-amber-200' : 'text-slate-300 hover:text-slate-200'}`}
            >
              {needsAction ? 'Enter License' : 'Update License'}
            </button>
          </div>
        </div>

        {/* Right: Deployment badge with hover details and click for modal */}
        <div className="flex items-center">
          <Tooltip
            content={deploymentMode === 'demo'
              ? 'Demo Mode: Webhooks disabled; manual triggers only.'
              : (deploymentMode === 'production' ? 'Production Mode: Full capabilities enabled.' : 'Mode unknown')
            }
            position="left"
          >
            <button
              type="button"
              onClick={() => setShowDemoInfo(true)}
              className={
                `inline-flex items-center gap-1 px-2 py-0.5 rounded-full font-medium border ` +
                (deploymentMode === 'demo' ? 'bg-amber-900/50 text-amber-200 border-amber-700' : 'bg-emerald-900/50 text-emerald-200 border-emerald-700')
              }
              title={deploymentMode === 'demo' ? 'Demo Mode' : (deploymentMode === 'production' ? 'Production Mode' : 'Mode unknown')}
            >
              <span className={`h-1.5 w-1.5 rounded-full ${deploymentMode === 'demo' ? 'bg-amber-400' : 'bg-emerald-400'}`}></span>
              {deploymentMode === 'demo' ? 'Demo Mode' : (deploymentMode === 'production' ? 'Production' : 'Mode')}
            </button>
          </Tooltip>
        </div>
      </div>

      {/* Modal/popover for demo info */}
      {showDemoInfo && (
        <div className="fixed inset-0 z-50 flex items-start sm:items-center justify-center bg-black/40 p-4" role="dialog" aria-modal="true">
          <div className="w-full max-w-2xl rounded-lg border border-slate-700 bg-slate-900 shadow-xl">
            <div className={`flex items-center justify-between px-4 py-3 rounded-t-lg ${deploymentMode === 'demo' ? 'bg-amber-900/40' : 'bg-emerald-900/40'}`}>
              <div className="flex items-center gap-2">
                <span className={`h-2 w-2 rounded-full ${deploymentMode === 'demo' ? 'bg-amber-400' : 'bg-emerald-400'}`}></span>
                <h3 className="text-sm font-semibold text-white">{deploymentMode === 'demo' ? 'Demo Mode' : 'Production Mode'}</h3>
              </div>
              <button onClick={() => setShowDemoInfo(false)} className="text-slate-300 hover:text-white">✕</button>
            </div>
            <div className="px-4 py-4 text-sm text-slate-200">
              {deploymentMode === 'demo' ? (
                <>
                  <p className="mb-2">Webhooks are disabled. Only manual review triggers are available. This mode is for local testing and demonstration purposes only.</p>
                  <div className="flex items-center gap-2 mt-3">
                    <a
                      href="/docs/deployment"
                      target="_blank"
                      rel="noreferrer"
                      className="inline-flex items-center gap-2 text-amber-200 hover:text-amber-100 underline"
                    >
                      Upgrade to Production
                    </a>
                  </div>
                </>
              ) : (
                <p>System is running in production mode with full capabilities enabled.</p>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default LicenseStatusBar;
