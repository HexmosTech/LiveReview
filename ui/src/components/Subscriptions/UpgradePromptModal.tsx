import React from 'react';
import { createPortal } from 'react-dom';
import { useNavigate } from 'react-router-dom';

type UpgradeReason = 'DAILY_LIMIT' | 'MEMBER_ACTIVATION' | 'NOT_ORG_CREATOR';

interface UpgradePromptModalProps {
    isOpen: boolean;
    onClose: () => void;
    reason: UpgradeReason;
    currentCount?: number;
    limit?: number;
}

export const UpgradePromptModal: React.FC<UpgradePromptModalProps> = ({
    isOpen,
    onClose,
    reason,
    currentCount,
    limit,
}) => {
    const navigate = useNavigate();

    if (!isOpen) return null;

    const getContent = () => {
        switch (reason) {
            case 'DAILY_LIMIT':
                return {
                    title: 'Daily Review Limit Reached',
                    icon: (
                        <svg className="w-6 h-6 text-amber-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                    ),
                    message: (
                        <>
                            <p className="text-slate-300 text-base leading-relaxed mb-4">
                                You've used <strong className="text-white">{currentCount || 3} out of {limit || 3}</strong> reviews today on the
                                <span className="inline-flex items-center mx-2 px-2.5 py-0.5 rounded-full text-xs font-medium bg-slate-700 text-slate-300 border border-slate-600">
                                    Free Plan
                                </span>
                            </p>
                            <div className="bg-blue-900/20 border border-blue-500/30 rounded-lg p-4 mb-4">
                                <h3 className="text-blue-200 font-semibold mb-2">Upgrade to Team Plan</h3>
                                <ul className="space-y-2 text-blue-100/80 text-sm">
                                    <li className="flex items-center">
                                        <svg className="w-4 h-4 text-emerald-400 mr-2 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                                            <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
                                        </svg>
                                        <span><strong>Unlimited</strong> daily reviews</span>
                                    </li>
                                    <li className="flex items-center">
                                        <svg className="w-4 h-4 text-emerald-400 mr-2 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                                            <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
                                        </svg>
                                        <span>Team collaboration with multiple members</span>
                                    </li>
                                    <li className="flex items-center">
                                        <svg className="w-4 h-4 text-emerald-400 mr-2 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                                            <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
                                        </svg>
                                        <span>Priority support</span>
                                    </li>
                                </ul>
                            </div>
                            <p className="text-slate-400 text-sm">
                                Your review limit will reset tomorrow at midnight.
                            </p>
                        </>
                    ),
                };
            
            case 'MEMBER_ACTIVATION':
                return {
                    title: 'Member Successfully Created',
                    icon: (
                        <svg className="w-6 h-6 text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                    ),
                    message: (
                        <>
                            <p className="text-emerald-300 text-base font-medium leading-relaxed mb-4">
                                ✓ The member has been added to your organization.
                            </p>
                            <div className="bg-amber-900/20 border border-amber-500/30 rounded-lg p-4 mb-4">
                                <h3 className="text-amber-200 font-semibold mb-2 flex items-center">
                                    <svg className="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                                    </svg>
                                    Team Plan Required for Activation
                                </h3>
                                <ul className="space-y-2 text-amber-100/80 text-sm">
                                    <li>• <strong>Free Plan:</strong> You can invite members, but only you can access reviews</li>
                                    <li>• <strong>Team Plan:</strong> Activate licenses to give team members full access</li>
                                </ul>
                            </div>
                            <p className="text-slate-400 text-sm">
                                Upgrade to the Team plan to activate members and enable team collaboration.
                            </p>
                        </>
                    ),
                };

            case 'NOT_ORG_CREATOR':
                return {
                    title: 'Permission Required',
                    icon: (
                        <svg className="w-6 h-6 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
                        </svg>
                    ),
                    message: (
                        <>
                            <p className="text-slate-300 text-base leading-relaxed mb-4">
                                Only the <strong className="text-white">organization creator</strong> can trigger reviews on the
                                <span className="inline-flex items-center mx-2 px-2.5 py-0.5 rounded-full text-xs font-medium bg-slate-700 text-slate-300 border border-slate-600">
                                    Free Plan
                                </span>
                            </p>
                            <div className="bg-blue-900/20 border border-blue-500/30 rounded-lg p-4 mb-4">
                                <h3 className="text-blue-200 font-semibold mb-2">Upgrade to Team Plan</h3>
                                <ul className="space-y-2 text-blue-100/80 text-sm">
                                    <li className="flex items-center">
                                        <svg className="w-4 h-4 text-emerald-400 mr-2 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                                            <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
                                        </svg>
                                        <span>All team members can trigger reviews</span>
                                    </li>
                                    <li className="flex items-center">
                                        <svg className="w-4 h-4 text-emerald-400 mr-2 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                                            <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
                                        </svg>
                                        <span><strong>Unlimited</strong> daily reviews</span>
                                    </li>
                                    <li className="flex items-center">
                                        <svg className="w-4 h-4 text-emerald-400 mr-2 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                                            <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
                                        </svg>
                                        <span>Team collaboration features</span>
                                    </li>
                                </ul>
                            </div>
                            <p className="text-slate-400 text-sm">
                                Contact your organization creator to upgrade the plan.
                            </p>
                        </>
                    ),
                };

            default:
                return {
                    title: 'Upgrade Required',
                    icon: null,
                    message: <p>Please upgrade to access this feature.</p>,
                };
        }
    };

    const content = getContent();

    const handleUpgrade = () => {
        onClose();
        navigate('/subscribe');
    };

    return createPortal(
        <div className="fixed inset-0 z-[10000] flex items-center justify-center p-4 bg-slate-900/80 backdrop-blur-sm">
            <div className="relative bg-slate-800 rounded-lg border border-slate-700 shadow-2xl max-w-lg w-full overflow-hidden">
                {/* Header */}
                <div className="p-6 border-b border-slate-700 bg-slate-900/50 flex items-center justify-between">
                    <div className="flex items-center space-x-4">
                        <div className="w-12 h-12 rounded-full bg-blue-500/20 flex items-center justify-center flex-shrink-0">
                            {content.icon}
                        </div>
                        <div>
                            <h2 className="text-xl font-bold text-white">{content.title}</h2>
                            <p className="text-slate-400 text-sm">Upgrade to continue</p>
                        </div>
                    </div>
                    <button
                        onClick={onClose}
                        className="text-slate-400 hover:text-white transition-colors"
                        aria-label="Close"
                    >
                        <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                        </svg>
                    </button>
                </div>

                {/* Content */}
                <div className="p-6">
                    {content.message}
                </div>

                {/* Actions */}
                <div className="p-6 bg-slate-900/50 border-t border-slate-700 flex justify-end gap-3">
                    <button
                        onClick={onClose}
                        className="px-4 py-2 text-sm font-medium text-slate-300 hover:text-white bg-slate-700 hover:bg-slate-600 rounded-lg transition-colors"
                    >
                        Maybe Later
                    </button>
                    <button
                        onClick={handleUpgrade}
                        className="px-6 py-2 text-sm font-medium text-white bg-blue-600 hover:bg-blue-500 rounded-lg transition-colors flex items-center gap-2"
                    >
                        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 7h8m0 0v8m0-8l-8 8-4-4-6 6" />
                        </svg>
                        Upgrade Now
                    </button>
                </div>
            </div>
        </div>,
        document.body
    );
};
