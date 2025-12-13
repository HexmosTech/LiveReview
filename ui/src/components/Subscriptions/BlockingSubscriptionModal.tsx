import React from 'react';
import { createPortal } from 'react-dom';
import { OrganizationSelector } from '../OrganizationSelector';

interface BlockingSubscriptionModalProps {
    orgName: string;
}

export const BlockingSubscriptionModal: React.FC<BlockingSubscriptionModalProps> = ({
    orgName,
}) => {
    // Prevent closing by not providing onClose
    // This modal is blocking.

    return createPortal(
        <div className="fixed inset-0 z-[10000] flex items-center justify-center p-4 bg-slate-900/95 backdrop-blur-md">
            <div className="relative bg-slate-800 rounded-lg border border-slate-700 shadow-2xl max-w-lg w-full overflow-hidden">
                {/* Header */}
                <div className="p-6 border-b border-slate-700 bg-slate-900/50 flex items-center space-x-4">
                    <div className="w-12 h-12 rounded-full bg-red-500/20 flex items-center justify-center flex-shrink-0">
                        <svg className="w-6 h-6 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
                        </svg>
                    </div>
                    <div>
                        <h2 className="text-xl font-bold text-white">Access Restricted</h2>
                        <p className="text-slate-400 text-sm">Subscription Required</p>
                    </div>
                </div>

                {/* Content */}
                <div className="p-8 space-y-6">
                    <div className="space-y-4">
                        <p className="text-slate-300 text-lg leading-relaxed">
                            The organization <strong className="text-white">{orgName}</strong> is currently on the
                            <span className="inline-flex items-center mx-2 px-2.5 py-0.5 rounded-full text-xs font-medium bg-slate-700 text-slate-300 border border-slate-600">
                                Hobby Plan
                            </span>
                        </p>

                        <div className="bg-amber-900/20 border border-amber-500/30 rounded-lg p-5">
                            <h3 className="text-amber-200 font-semibold mb-2 flex items-center">
                                <svg className="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                                </svg>
                                Access Policy
                            </h3>
                            <p className="text-amber-100/80 text-sm">
                                On the Hobby plan, only the <strong>Organization Creator</strong> can access the dashboard and reviews.
                                All other members lose access until the subscription is upgraded.
                            </p>
                        </div>

                        <p className="text-slate-400 text-sm">
                            Please contact the organization creator to upgrade to the Team plan to restore access for all members.
                        </p>
                    </div>
                </div>

                {/* Actions */}
                <div className="p-6 bg-slate-900/50 border-t border-slate-700 flex justify-between items-center">
                    <span className="text-slate-400 text-sm">Switch to another organization:</span>
                    <OrganizationSelector
                        position="inline"
                        size="md"
                    />
                </div>
            </div>
        </div>,
        document.body
    );
};
