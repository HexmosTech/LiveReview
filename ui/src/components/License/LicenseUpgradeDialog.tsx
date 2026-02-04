import React from 'react';
import { createPortal } from 'react-dom';
import { TIER_DISPLAY_NAMES, LicenseTier } from '../../constants/licenseTiers';

// Re-export for backward compatibility
export { LicenseTier } from '../../constants/licenseTiers';
export { useLicenseTier, useHasLicenseFor, hasTierAccess as hasLicenseFor } from '../../hooks/useLicenseTier';

interface LicenseUpgradeDialogProps {
  open: boolean;
  onClose: () => void;
  requiredTier: LicenseTier;
  featureName: string;
  featureDescription?: string;
}

interface FeatureRow {
  feature: string;
  community: boolean;
  team: boolean;
  enterprise: boolean;
}

const FEATURE_COMPARISON: FeatureRow[] = [
  { feature: 'Basic Code Reviews', community: true, team: true, enterprise: true },
  { feature: 'Git Provider Integration', community: true, team: true, enterprise: true },
  { feature: 'AI Provider Configuration', community: true, team: true, enterprise: true },
  { feature: 'Dashboard & Analytics', community: true, team: true, enterprise: true },
  { feature: 'Prompt Customization', community: false, team: true, enterprise: true },
  { feature: 'Learnings Management', community: false, team: true, enterprise: true },
  { feature: 'Multiple API Keys', community: false, team: true, enterprise: true },
  { feature: 'Team Management (>3 users)', community: false, team: true, enterprise: true },
  { feature: 'Priority Support', community: false, team: true, enterprise: true },
  { feature: 'SSO / SAML', community: false, team: false, enterprise: true },
  { feature: 'Audit Logs', community: false, team: false, enterprise: true },
  { feature: 'Compliance Reports', community: false, team: false, enterprise: true },
  { feature: 'Custom Integrations', community: false, team: false, enterprise: true },
];

const CheckIcon: React.FC<{ className?: string }> = ({ className }) => (
  <svg className={className || "w-5 h-5 text-green-400"} fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M5 13l4 4L19 7" />
  </svg>
);

const XIcon: React.FC<{ className?: string }> = ({ className }) => (
  <svg className={className || "w-5 h-5 text-slate-500"} fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
  </svg>
);

const LicenseUpgradeDialog: React.FC<LicenseUpgradeDialogProps> = ({
  open,
  onClose,
  requiredTier,
  featureName,
  featureDescription,
}) => {
  const [mounted, setMounted] = React.useState(false);

  // Trigger mount animation after dialog opens
  React.useEffect(() => {
    if (open) {
      // Small delay to trigger CSS transition
      const timer = setTimeout(() => setMounted(true), 10);
      return () => clearTimeout(timer);
    } else {
      setMounted(false);
    }
  }, [open]);

  React.useEffect(() => {
    const handleEscKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && open) {
        onClose();
      }
    };
    document.addEventListener('keydown', handleEscKey);
    return () => document.removeEventListener('keydown', handleEscKey);
  }, [open, onClose]);

  if (!open) return null;

  return createPortal(
    <div className={`fixed inset-0 z-50 flex items-center justify-center p-4 transition-opacity duration-200 ${mounted ? 'opacity-100' : 'opacity-0'}`}>
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={onClose} />
      
      {/* Dialog */}
      <div className={`relative w-full max-w-4xl bg-gradient-to-b from-slate-800 to-slate-900 rounded-2xl border border-slate-700 shadow-2xl overflow-hidden transform transition-all duration-200 ${mounted ? 'opacity-100 scale-100' : 'opacity-0 scale-95'}`}>
        {/* Header with gradient */}
        <div className="relative px-6 py-6 bg-gradient-to-r from-blue-600/20 via-purple-600/20 to-pink-600/20 border-b border-slate-700">
          <button
            onClick={onClose}
            className="absolute top-4 right-4 text-slate-400 hover:text-white p-2 rounded-lg hover:bg-slate-700/50 transition-colors"
            aria-label="Close"
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
          
          <div className="flex items-center gap-4">
            <div className="w-14 h-14 rounded-xl bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center shadow-lg">
              <svg className="w-7 h-7 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
              </svg>
            </div>
            <div>
              <h2 className="text-2xl font-bold text-white">Upgrade to Unlock</h2>
              <p className="text-slate-300 mt-1">
                <span className="font-semibold text-blue-400">{featureName}</span> requires a{' '}
                <span className="font-semibold text-purple-400">{TIER_DISPLAY_NAMES[requiredTier]}</span> license or above
              </p>
            </div>
          </div>
          
          {featureDescription && (
            <p className="mt-4 text-slate-400 text-sm max-w-2xl">{featureDescription}</p>
          )}
        </div>

        {/* Comparison Table */}
        <div className="p-6 max-h-[50vh] sm:max-h-[55vh] md:max-h-[60vh] overflow-y-auto">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-slate-700">
                  <th className="text-left py-4 px-4 text-slate-300 font-medium">Feature</th>
                  <th className="text-center py-4 px-4 min-w-[100px]">
                    <div className="text-slate-300 font-medium">Community</div>
                    <div className="text-sm text-slate-500">Free</div>
                  </th>
                  <th className="text-center py-4 px-4 min-w-[100px]">
                    <div className="relative">
                      <div className="text-blue-400 font-bold">Team</div>
                      <div className="text-sm text-blue-300/70">Recommended</div>
                      {requiredTier === 'team' && (
                        <div className="absolute -top-2 -right-2 w-4 h-4 bg-blue-500 rounded-full animate-pulse" />
                      )}
                    </div>
                  </th>
                  <th className="text-center py-4 px-4 min-w-[100px]">
                    <div className="text-purple-400 font-bold">Enterprise</div>
                    <div className="text-sm text-purple-300/70">Custom</div>
                  </th>
                </tr>
              </thead>
              <tbody>
                {FEATURE_COMPARISON.map((row) => (
                  <tr 
                    key={row.feature} 
                    className={`border-b border-slate-800 transition-colors ${
                      row.feature === featureName ? 'bg-blue-500/10' : 'hover:bg-slate-800/50'
                    }`}
                  >
                    <td className="py-3 px-4 text-slate-200 text-sm">
                      {row.feature}
                      {row.feature === featureName && (
                        <span className="ml-2 text-xs bg-blue-500/30 text-blue-300 px-2 py-0.5 rounded-full">
                          Requested
                        </span>
                      )}
                    </td>
                    <td className="py-3 px-4 text-center">
                      {row.community ? <CheckIcon /> : <XIcon />}
                    </td>
                    <td className="py-3 px-4 text-center">
                      {row.team ? <CheckIcon className="w-5 h-5 text-blue-400" /> : <XIcon />}
                    </td>
                    <td className="py-3 px-4 text-center">
                      {row.enterprise ? <CheckIcon className="w-5 h-5 text-purple-400" /> : <XIcon />}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        {/* Footer / CTA */}
        <div className="px-6 py-5 bg-slate-800/50 border-t border-slate-700">
          <div className="flex flex-col sm:flex-row items-center justify-between gap-4">
            <p className="text-slate-400 text-sm">
              Unlock the full potential of LiveReview for your team
            </p>
            <div className="flex items-center gap-3">
              <button
                onClick={onClose}
                className="px-4 py-2 text-sm rounded-lg bg-slate-700 hover:bg-slate-600 text-slate-200 transition-colors"
              >
                Maybe Later
              </button>
              <a
                href="https://hexmos.com/livereview/selfhosted-access/"
                target="_blank"
                rel="noopener noreferrer"
                className="px-5 py-2.5 text-sm font-medium rounded-lg bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-500 hover:to-purple-500 text-white transition-all shadow-lg hover:shadow-xl hover:shadow-purple-500/25 flex items-center gap-2"
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                </svg>
                Get License
              </a>
            </div>
          </div>
        </div>
      </div>
    </div>,
    document.body
  );
};

export default LicenseUpgradeDialog;
