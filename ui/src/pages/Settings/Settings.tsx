import React, { useState, useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { PageHeader, Card, Button, Icons, Input, Alert } from '../../components/UIPrimitives';
import { UserManagement } from '../../components/UserManagement';
import { useOrgContext } from '../../hooks/useOrgContext';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { updateDomain } from '../../store/Settings/reducer';
import apiClient from '../../api/apiClient';

// Custom styled alerts for dark mode
interface AlertProps {
    children: React.ReactNode;
    onClose?: () => void;
}

const SuccessAlert: React.FC<AlertProps> = ({ children, onClose }) => (
    <div className="bg-green-900 bg-opacity-50 text-green-100 border border-green-600 rounded-lg p-4 mb-4" style={{ backgroundColor: 'rgba(21, 128, 61, 0.9)' }}>
        <div className="flex items-center">
            <div className="mr-3 text-green-300">
                <Icons.Success />
            </div>
            <div className="flex-grow">
                {children}
            </div>
            {onClose && (
                <button
                    type="button"
                    onClick={onClose}
                    className="bg-green-800 text-green-100 rounded-md p-1.5 hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-green-500"
                >
                    <span className="sr-only">Dismiss</span>
                    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor">
                        <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                    </svg>
                </button>
            )}
        </div>
    </div>
);

const ErrorAlert: React.FC<AlertProps> = ({ children, onClose }) => (
    <div className="bg-red-900 bg-opacity-50 text-red-100 border border-red-600 rounded-lg p-4 mb-4" style={{ backgroundColor: 'rgba(153, 27, 27, 0.9)' }}>
        <div className="flex items-center">
            <div className="mr-3 text-red-300">
                <Icons.Error />
            </div>
            <div className="flex-grow">
                {children}
            </div>
            {onClose && (
                <button
                    type="button"
                    onClick={onClose}
                    className="bg-red-800 text-red-100 rounded-md p-1.5 hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-500"
                >
                    <span className="sr-only">Dismiss</span>
                    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor">
                        <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                    </svg>
                </button>
            )}
        </div>
    </div>
);

interface ProductionURLResponse {
    url: string;
    success: boolean;
    message: string;
}

const Settings = () => {
    const dispatch = useAppDispatch();
    const location = useLocation();
    const navigate = useNavigate();
    const { domain } = useAppSelector((state) => state.Settings);
    const { isSuperAdmin, canManageCurrentOrg } = useOrgContext();
    
    const activeTab = location.hash.substring(1) || 'general';
    const [productionUrl, setProductionUrl] = useState('');
    const [showSaved, setShowSaved] = useState(false);
    const [error, setError] = useState('');
    const [isLoading, setIsLoading] = useState(false);

    useEffect(() => {
        if (!location.hash) {
            // Default to first available tab based on permissions
            const defaultTab = isSuperAdmin ? 'instance' : 'ai';
            navigate(`#${defaultTab}`, { replace: true });
        }
    }, [location.hash, navigate, isSuperAdmin]);

    // Available tabs based on permissions
    const tabs = [
        ...(isSuperAdmin ? [{ id: 'instance', name: 'Instance', icon: <Icons.Settings /> }] : []),
        { id: 'ai', name: 'AI Configuration', icon: <Icons.AI /> },
        { id: 'ui', name: 'UI Preferences', icon: <Icons.Dashboard /> },
        ...(canManageCurrentOrg ? [{ 
            id: 'users', 
            name: 'User Management', 
            icon: (
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197m13.5-9a2.5 2.5 0 11-5 0 2.5 2.5 0 015 0z" />
                </svg>
            )
        }] : []),
        ...(isSuperAdmin ? [{ 
            id: 'admin', 
            name: 'Global Admin', 
            icon: (
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
                </svg>
            )
        }] : []),
    ];

    // URL validation - must be http/https and not localhost
    const validateUrl = (url: string): boolean => {
        if (!url) return false;
        
        try {
            const urlObj = new URL(url);
            const isValidProtocol = urlObj.protocol === 'http:' || urlObj.protocol === 'https:';
            const isLocalhost = urlObj.hostname === 'localhost' || 
                               urlObj.hostname === '127.0.0.1' ||
                               urlObj.hostname.startsWith('192.168.') ||
                               urlObj.hostname.startsWith('10.') ||
                               urlObj.hostname.startsWith('172.16.'); 
            
            return isValidProtocol && !isLocalhost;
        } catch (e) {
            return false;
        }
    };

    const handleUrlChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        // Allow users to type whatever they want, including trailing slashes
        const newValue = e.target.value;
        setProductionUrl(newValue);
        
        // Clear any previous error messages
        if (error) {
            setError('');
        }
    };

    // Get the production URL on component mount
    useEffect(() => {
        const fetchProductionUrl = async () => {
            setIsLoading(true);
            try {
                const response = await apiClient.get<ProductionURLResponse>('/api/v1/production-url');
                console.log('Production URL response:', response); // Debug log
                
                if (response && response.url) {
                    const trimmedUrl = response.url.replace(/\/+$/, '');
                    setProductionUrl(trimmedUrl);
                    dispatch(updateDomain(trimmedUrl)); // Update Redux state
                }
            } catch (error) {
                console.error('Failed to fetch production URL:', error);
                // Show a less intrusive message for initial load
                if ((error as any)?.status === 404) {
                    console.warn('API endpoint not found. The server may not be running or the endpoint may be incorrect.');
                }
                // Don't show error to user on initial load, just use empty string
            } finally {
                setIsLoading(false);
            }
        };
        
        fetchProductionUrl();
    }, [dispatch]);

    const handleSaveDomain = async () => {
        // Remove trailing slashes from the URL only when saving
        const trimmedUrl = productionUrl.replace(/\/+$/, '');
        
        // If the URL was changed, update the state
        if (trimmedUrl !== productionUrl) {
            setProductionUrl(trimmedUrl);
        }
        
        // Validate URL
        if (!validateUrl(trimmedUrl)) {
            setError('Please enter a valid URL (https://example.com). Local addresses are not allowed.');
            return;
        }
        
        setIsLoading(true);
        setError('');
        
        console.log('Sending request to update production URL:', trimmedUrl); // Debug log
        
        try {
            // Use the correct property name "url" as defined in the Go struct
            const response = await apiClient.put<ProductionURLResponse>('/api/v1/production-url', {
                url: trimmedUrl  // The server expects this exact field name
            });
            
            console.log('Production URL update response:', response); // Debug log
            
            if (response && response.success) {
                dispatch(updateDomain(trimmedUrl)); // Use trimmedUrl here to ensure consistency
                setShowSaved(true);
                setTimeout(() => setShowSaved(false), 3000);
            } else {
                setError((response && response.message) || 'Failed to update production URL');
            }
        } catch (error) {
            console.error('Failed to save production URL:', error);
            
            // Provide more detailed error information to help with debugging
            let errorMessage = 'Failed to save production URL.';
            
            if (error instanceof Error) {
                errorMessage = error.message;
            }
            
            // If it's a 404 error, it means the endpoint doesn't exist or is incorrect
            if ((error as any)?.status === 404) {
                errorMessage = 'The API endpoint could not be found. Please ensure the server is running and configured correctly.';
            }
            
            setError(errorMessage);
        } finally {
            setIsLoading(false);
        }
    };

    return (
        <div className="container mx-auto px-4 py-8">
            <PageHeader 
                title="Settings" 
                description="Configure application preferences and behaviors"
            />
            
            <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
                <div className="md:col-span-1">
                    <Card>
                        <div className="space-y-2">
                            {tabs.map((tab) => (
                                <Button 
                                    key={tab.id}
                                    variant={activeTab === tab.id ? "primary" : "ghost"} 
                                    fullWidth 
                                    className="justify-start"
                                    icon={tab.icon}
                                    onClick={() => navigate(`#${tab.id}`)}
                                >
                                    {tab.name}
                                </Button>
                            ))}
                        </div>
                    </Card>
                </div>
                
                <div className="md:col-span-3">
                    {activeTab === 'instance' && isSuperAdmin && (
                        <Card>
                            <div className="flex items-center mb-6">
                                <img src="assets/logo.svg" alt="LiveReview Logo" className="h-8 w-auto mr-3" />
                                <div>
                                    <h3 className="font-medium text-white">LiveReview v1.0.0</h3>
                                    <p className="text-sm text-slate-300">Automated code reviews powered by AI</p>
                                </div>
                            </div>

                            {showSaved && (
                                <SuccessAlert onClose={() => setShowSaved(false)}>
                                    Settings saved successfully!
                                </SuccessAlert>
                            )}
                            
                            <div className="space-y-6">
                                <div>
                                    <h3 className="text-lg font-medium text-white mb-2">LiveReview Production URL</h3>
                                    <p className="text-sm text-slate-300 mb-4">
                                        Configure your LiveReview production URL. This is required for setting up OAuth 
                                        connections with services like GitLab and GitHub.
                                    </p>
                                    <p className="text-xs text-slate-400 mb-4">
                                        API Endpoint: /api/v1/production-url
                                    </p>
                                    
                                    {error && (
                                        <ErrorAlert onClose={() => setError('')}>
                                            {error}
                                        </ErrorAlert>
                                    )}
                                    
                                    <div className="space-y-4">
                                        <Input
                                            label="Production URL"
                                            placeholder="https://livereview.your-company.com"
                                            value={productionUrl}
                                            onChange={handleUrlChange}
                                            helperText="Enter the full URL where your LiveReview instance is hosted (must be https://). Trailing slashes will be removed when saved."
                                            disabled={isLoading}
                                        />
                                        <div className="flex justify-end">
                                            <Button 
                                                onClick={handleSaveDomain}
                                                variant="primary"
                                                isLoading={isLoading}
                                                disabled={isLoading}
                                            >
                                                Save
                                            </Button>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </Card>
                    )}

                    {activeTab === 'ai' && (
                        <Card>
                            <h3 className="text-lg font-medium text-white mb-4">AI Configuration</h3>
                            <p className="text-slate-400">AI configuration options will be available here.</p>
                        </Card>
                    )}

                    {activeTab === 'ui' && (
                        <Card>
                            <h3 className="text-lg font-medium text-white mb-4">UI Preferences</h3>
                            <p className="text-slate-400">UI preference options will be available here.</p>
                        </Card>
                    )}

                    {activeTab === 'users' && canManageCurrentOrg && (
                        <Card>
                            <UserManagement isSuperAdminView={false} />
                        </Card>
                    )}

                    {activeTab === 'admin' && isSuperAdmin && (
                        <Card>
                            <UserManagement isSuperAdminView={true} />
                        </Card>
                    )}
                </div>
            </div>
        </div>
    );
};

export default Settings;
