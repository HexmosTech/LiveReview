import React from 'react';
import { Button, Icons } from '../UIPrimitives';

interface QuotaWarningBannerProps {
    locUsed: number;
    locLimit: number;
    usagePct: number;
    onUpgrade: () => void;
}

export const QuotaWarningBanner: React.FC<QuotaWarningBannerProps> = ({
    locUsed,
    locLimit,
    usagePct,
    onUpgrade,
}) => {
    return (
        <div className="border-l-4 border-amber-500 bg-slate-800/90 rounded-xl border border-slate-700/50 p-5 mb-6 shadow-lg shadow-black/20 backdrop-blur-sm">
            <div className="flex items-start gap-4">
                {/* Icon Section */}
                <div className="flex-shrink-0 w-9 h-9 rounded-full bg-amber-500/10 flex items-center justify-center border border-amber-500/20">
                    <div className="text-amber-500">
                        <Icons.Warning />
                    </div>
                </div>

                {/* Content Section */}
                <div className="flex-1">
                    <p className="text-sm font-bold text-slate-100 mb-1">
                        LOC Usage Nearing Limit
                    </p>
                    <p className="text-sm text-slate-400 mb-4 leading-relaxed">
                        You've used <strong className="text-slate-100">{locUsed.toLocaleString()}</strong> of <strong className="text-slate-100">{locLimit > 0 ? locLimit.toLocaleString() : 'N/A'} LOC</strong> ({usagePct}%) this month. Upgrade to avoid interruption to your workflow.
                    </p>
                    <Button 
                        variant="primary"
                        onClick={onUpgrade}
                        className="!bg-amber-600 hover:!bg-amber-500 text-white text-sm font-medium px-5 py-2 rounded-lg transition-colors border-none shadow-md shadow-amber-900/20"
                    >
                        Upgrade plan
                    </Button>
                </div>
            </div>
        </div>
    );
};
