import React, { useState } from 'react';
import { createPortal } from 'react-dom';
import { useAppDispatch } from '../../store/configureStore';
import { createOrganization, loadUserOrganizations } from '../../store/Organizations/reducer';
import { CreateOrganizationPayload } from '../../store/Organizations/types';

export interface CreateOrganizationModalProps {
    isOpen: boolean;
    onClose: () => void;
    onSuccess?: () => void;
}

export const CreateOrganizationModal: React.FC<CreateOrganizationModalProps> = ({
    isOpen,
    onClose,
    onSuccess,
}) => {
    const dispatch = useAppDispatch();
    const [formData, setFormData] = useState<CreateOrganizationPayload>({
        name: '',
        description: '',
    });
    const [errors, setErrors] = useState<{ name?: string; description?: string; submit?: string }>({});
    const [isSubmitting, setIsSubmitting] = useState(false);
    const [showSuccess, setShowSuccess] = useState(false);

    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
        const { name, value } = e.target;
        setFormData(prev => ({ ...prev, [name]: value }));
        // Clear error for this field when user starts typing
        if (errors[name as keyof typeof errors]) {
            setErrors(prev => ({ ...prev, [name]: undefined }));
        }
    };

    const validateForm = (): boolean => {
        const newErrors: typeof errors = {};
        
        if (!formData.name.trim()) {
            newErrors.name = 'Organization name is required';
        } else if (formData.name.length > 255) {
            newErrors.name = 'Name must be less than 255 characters';
        }
        
        if (formData.description && formData.description.length > 1000) {
            newErrors.description = 'Description must be less than 1000 characters';
        }
        
        setErrors(newErrors);
        return Object.keys(newErrors).length === 0;
    };

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        
        if (!validateForm()) {
            return;
        }
        
        setIsSubmitting(true);
        setErrors({});
        
        try {
            const result = await dispatch(createOrganization(formData));
            
            if (createOrganization.fulfilled.match(result)) {
                // Show success message
                setShowSuccess(true);
                
                // Reload organizations to get the updated list
                await dispatch(loadUserOrganizations());
                
                // Wait a moment to show success, then close
                setTimeout(() => {
                    setShowSuccess(false);
                    setFormData({ name: '', description: '' });
                    
                    // Call success callback
                    if (onSuccess) {
                        onSuccess();
                    }
                    
                    // Close modal
                    onClose();
                }, 1500);
            } else if (createOrganization.rejected.match(result)) {
                setErrors({ submit: result.payload as string || 'Failed to create organization' });
            }
        } catch (error: any) {
            setErrors({ submit: error.message || 'An unexpected error occurred' });
        } finally {
            setIsSubmitting(false);
        }
    };

    const handleClose = () => {
        if (!isSubmitting && !showSuccess) {
            setFormData({ name: '', description: '' });
            setErrors({});
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
                                <h3 className="text-lg font-semibold text-white mb-1">Organization Created!</h3>
                                <p className="text-sm text-slate-400">"{formData.name}" has been created successfully</p>
                            </div>
                        </div>
                    </div>
                )}

                {/* Header */}
                <div className="flex items-center justify-between p-6 border-b border-slate-700">
                    <h2 className="text-xl font-semibold text-white">Create Organization</h2>
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

                {/* Form */}
                <form onSubmit={handleSubmit} className="p-6 space-y-4">
                    {/* Organization Name */}
                    <div>
                        <label htmlFor="name" className="block text-sm font-medium text-slate-300 mb-2">
                            Organization Name <span className="text-red-400">*</span>
                        </label>
                        <input
                            type="text"
                            id="name"
                            name="name"
                            value={formData.name}
                            onChange={handleInputChange}
                            disabled={isSubmitting}
                            className={`w-full px-3 py-2 bg-slate-900 border rounded-lg text-white placeholder-slate-500 focus:outline-none focus:ring-2 transition-colors ${
                                errors.name 
                                    ? 'border-red-500 focus:ring-red-500' 
                                    : 'border-slate-600 focus:ring-blue-500'
                            } disabled:opacity-50 disabled:cursor-not-allowed`}
                            placeholder="Enter organization name"
                            maxLength={255}
                            autoFocus
                        />
                        {errors.name && (
                            <p className="mt-1 text-sm text-red-400">{errors.name}</p>
                        )}
                    </div>

                    {/* Description */}
                    <div>
                        <label htmlFor="description" className="block text-sm font-medium text-slate-300 mb-2">
                            Description <span className="text-slate-500">(optional)</span>
                        </label>
                        <textarea
                            id="description"
                            name="description"
                            value={formData.description}
                            onChange={handleInputChange}
                            disabled={isSubmitting}
                            rows={3}
                            className={`w-full px-3 py-2 bg-slate-900 border rounded-lg text-white placeholder-slate-500 focus:outline-none focus:ring-2 transition-colors resize-none ${
                                errors.description 
                                    ? 'border-red-500 focus:ring-red-500' 
                                    : 'border-slate-600 focus:ring-blue-500'
                            } disabled:opacity-50 disabled:cursor-not-allowed`}
                            placeholder="Enter a brief description"
                            maxLength={1000}
                        />
                        {errors.description && (
                            <p className="mt-1 text-sm text-red-400">{errors.description}</p>
                        )}
                        <p className="mt-1 text-xs text-slate-500">
                            {formData.description?.length || 0} / 1000 characters
                        </p>
                    </div>

                    {/* Submit Error */}
                    {errors.submit && (
                        <div className="p-3 bg-red-900/20 border border-red-500/50 rounded-lg">
                            <p className="text-sm text-red-400">{errors.submit}</p>
                        </div>
                    )}

                    {/* Actions */}
                    <div className="flex items-center justify-end space-x-3 pt-4">
                        <button
                            type="button"
                            onClick={handleClose}
                            disabled={isSubmitting || showSuccess}
                            className="px-4 py-2 text-sm font-medium text-slate-300 bg-slate-700 hover:bg-slate-600 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                            Cancel
                        </button>
                        <button
                            type="submit"
                            disabled={isSubmitting || showSuccess}
                            className="px-4 py-2 text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center space-x-2 min-w-[180px] justify-center"
                        >
                            {isSubmitting ? (
                                <>
                                    <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin"></div>
                                    <span>Creating...</span>
                                </>
                            ) : (
                                <>
                                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
                                    </svg>
                                    <span>Create Organization</span>
                                </>
                            )}
                        </button>
                    </div>
                </form>
            </div>
        </div>,
        document.body
    );
};
