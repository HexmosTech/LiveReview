import React, { useState } from 'react';
import { createPortal } from 'react-dom';
import toast from 'react-hot-toast';

export interface CancelSubscriptionModalProps {
    isOpen: boolean;
    onClose: () => void;
    onSuccess?: () => void;
    subscriptionId: string;
    expiryDate: string | null | undefined;
}

export const CancelSubscriptionModal: React.FC<CancelSubscriptionModalProps> = ({
    isOpen,
    onClose,
    onSuccess,
    subscriptionId,
    expiryDate,
}) => {
    const [isSubmitting, setIsSubmitting] = useState(false);
    const [showSuccess, setShowSuccess] = useState(false);

    const formatDate = (dateString: string | null | undefined) => {
        if (!dateString) return 'the end of the current billing period';
        return new Date(dateString).toLocaleDateString('en-US', {
            year: 'numeric',
            month: 'long',
            day: 'numeric',
        });
    };

    const handleCancel = async () => {
        setIsSubmitting(true);

        try {
            const response = await fetch(`/api/v1/subscriptions/${subscriptionId}/cancel`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    immediate: false, // Cancel at end of billing cycle
                }),
            });

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error || 'Failed to cancel subscription');
            }

            // Show success message
            setShowSuccess(true);

            // Wait a moment to show success, then close
            setTimeout(() => {
                setShowSuccess(false);
                
                // Call success callback
                if (onSuccess) {
                    onSuccess();
                }
                
                // Close modal
                handleClose();
            }, 2000);

        } catch (error: any) {
            console.error('Failed to cancel subscription:', error);
            toast.error(error.message || 'Failed to cancel subscription');
        } finally {
            setIsSubmitting(false);
        }
    };

    const handleClose = () => {
        if (!isSubmitting && !showSuccess) {
            onClose();
        }
    };

    if (!isOpen) {
        return null;
    }

    return createPortal(
        <div className="fixed inset-0 z-[9999] flex items-center justify-center p-4">
            {/* Backdrop */}
            <div 
                className="absolute inset-0 bg-black/70 backdrop-blur-sm" 
                onClick={handleClose}
            />
            
            {/* Modal */}
            <div className="relative bg-slate-800 rounded-lg border border-slate-600 shadow-2xl max-w-md w-full">
                {/* Success Overlay */}
                {showSuccess && (
                    <div className="absolute inset-0 bg-slate-800/95 rounded-lg flex items-center justify-center z-10">
                        <div className="text-center space-y-4">
                            <div className="w-16 h-16 mx-auto bg-green-500/20 rounded-full flex items-center justify-center">
                                <svg className="w-8 h-8 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                                </svg>
                            </div>
                            <div>
                                <h3 className="text-lg font-semibold text-white mb-1">Subscription Cancelled</h3>
                                <p className="text-sm text-slate-400">Your subscription will not renew</p>
                            </div>
                        </div>
                    </div>
                )}

                {/* Header */}
                <div className="flex items-center justify-between p-6 border-b border-slate-700">
                    <div className="flex items-center space-x-3">
                        <div className="w-10 h-10 rounded-full bg-red-500/20 flex items-center justify-center">
                            <svg className="w-5 h-5 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                            </svg>
                        </div>
                        <h2 className="text-xl font-semibold text-white">Cancel Subscription</h2>
                    </div>
                    <button
                        onClick={handleClose}
                        disabled={isSubmitting || showSuccess}
                        className="text-slate-400 hover:text-white transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                        <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                        </svg>
                    </button>
                </div>

                {/* Content */}
                <div className="p-6 space-y-4">
                    <div className="space-y-3">
                        <p className="text-slate-300">
                            Are you sure you want to cancel your subscription?
                        </p>

                        {/* Important Information */}
                        <div className="bg-amber-900/20 border border-amber-500/30 rounded-lg p-4 space-y-2">
                            <div className="flex items-start space-x-2">
                                <svg className="w-5 h-5 text-amber-400 mt-0.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                                </svg>
                                <div className="text-sm text-amber-200">
                                    <strong className="font-semibold">What happens next:</strong>
                                </div>
                            </div>
                            <ul className="ml-7 space-y-2 text-sm text-amber-100">
                                <li className="flex items-start">
                                    <span className="mr-2">•</span>
                                    <span>Your subscription will remain active until <strong className="font-semibold">{formatDate(expiryDate)}</strong></span>
                                </li>
                                <li className="flex items-start">
                                    <span className="mr-2">•</span>
                                    <span>You'll continue to have full access until the end of your current billing period</span>
                                </li>
                                <li className="flex items-start">
                                    <span className="mr-2">•</span>
                                    <span>After expiry, your plan will automatically revert to the Free plan (3 reviews per day)</span>
                                </li>
                                <li className="flex items-start">
                                    <span className="mr-2">•</span>
                                    <span>No further charges will be made</span>
                                </li>
                            </ul>
                        </div>

                        {/* Reconsider Notice */}
                        <div className="bg-blue-900/20 border border-blue-500/30 rounded-lg p-4">
                            <div className="flex items-start space-x-2">
                                <svg className="w-5 h-5 text-blue-400 mt-0.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z" />
                                </svg>
                                <div className="text-sm text-blue-200">
                                    <p className="mb-1"><strong className="font-semibold">Keep your team productive!</strong></p>
                                    <p>With unlimited reviews and priority support, your team can maintain velocity. You can always resubscribe later if you change your mind.</p>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>

                {/* Actions */}
                <div className="flex items-center justify-end space-x-3 p-6 bg-slate-900/50 border-t border-slate-700 rounded-b-lg">
                    <button
                        type="button"
                        onClick={handleClose}
                        disabled={isSubmitting || showSuccess}
                        className="px-5 py-2.5 text-sm font-medium text-slate-300 bg-slate-700 hover:bg-slate-600 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                        Keep Subscription
                    </button>
                    <button
                        type="button"
                        onClick={handleCancel}
                        disabled={isSubmitting || showSuccess}
                        className="px-5 py-2.5 text-sm font-medium text-white bg-red-600 hover:bg-red-700 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center space-x-2 min-w-[160px] justify-center"
                    >
                        {isSubmitting ? (
                            <>
                                <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin"></div>
                                <span>Cancelling...</span>
                            </>
                        ) : (
                            <>
                                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                                </svg>
                                <span>Cancel Subscription</span>
                            </>
                        )}
                    </button>
                </div>
            </div>
        </div>,
        document.body
    );
};
