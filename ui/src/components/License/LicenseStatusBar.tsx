import React, { useMemo } from 'react';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { triggerLicenseRefresh } from '../../store/License/slice';

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
  const dispatch = useAppDispatch();
  const license = useAppSelector(s => s.License);
  const style = STATUS_STYLES[license.status] || STATUS_STYLES['missing'];

  const daysLeft = useMemo(() => formatDaysLeft(license.expiresAt), [license.expiresAt]);
  const needsAction = ['missing','invalid','expired'].includes(license.status);

  const handleRefresh = () => {
    if (!license.refreshing) dispatch(triggerLicenseRefresh());
  };

  return (
    <div className={`w-full text-xs px-4 py-1 flex items-center gap-4 border-b border-slate-800 ${style.bg}`} data-testid="license-status-bar">
      <span className={`inline-flex items-center gap-1 font-medium ${style.fg}`}>
        <span className={`h-2 w-2 rounded-full ${style.accent} animate-pulse`}></span>
        {style.label}
      </span>
      {style.description && <span className="text-slate-400 hidden sm:inline" data-testid="license-desc">{style.description}</span>}
      {daysLeft && license.status === 'active' && (
        <span className="text-slate-400" data-testid="license-days-left">{daysLeft}</span>
      )}
      <div className="ml-auto flex items-center gap-3">
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
  );
};

export default LicenseStatusBar;
