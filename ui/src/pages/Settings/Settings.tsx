import React, { useState, useEffect, useCallback } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { PageHeader, Card, Button, Icons, Input, Alert, Badge } from '../../components/UIPrimitives';
import PromptsPage from '../Prompts';
import LicenseTab from './LicenseTab';
import LearningsTab from './LearningsTab';
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

// Deployment Settings Component
interface DeploymentSettingsProps {
    systemInfo: any;
    isLoading: boolean;
    onRefresh: () => void;
}

const DeploymentSettings: React.FC<DeploymentSettingsProps> = ({ systemInfo, isLoading, onRefresh }) => {
    // Don't auto-fetch on mount to avoid infinite loops
    // Users can manually refresh using the refresh button
    
    if (isLoading && !systemInfo) {
        return (
            <div className="flex items-center justify-center py-8">
                <div className="text-center">
                    <div className="w-8 h-8 border-2 border-blue-500 border-t-transparent rounded-full animate-spin mx-auto mb-2"></div>
                    <p className="text-slate-400">Loading deployment information...</p>
                </div>
            </div>
        );
    }

    if (!systemInfo) {
        return (
            <div className="text-center py-8">
                <div className="text-red-400 mb-2">
                    <Icons.Error />
                </div>
                <p className="text-slate-400 mb-4">Failed to load deployment information</p>
                <Button onClick={onRefresh} variant="outline" size="sm">
                    <Icons.Refresh />
                    Retry
                </Button>
            </div>
        );
    }

    const { deployment_mode, capabilities, version, api_url } = systemInfo;
    const isDemoMode = deployment_mode === 'demo';

    return (
        <div className="space-y-6">
            {/* Deployment Mode Status */}
            <div>
                <h4 className="text-sm font-medium text-slate-300 mb-3">Deployment Mode</h4>
                <div className="flex items-center space-x-3">
                    <Badge 
                        variant={isDemoMode ? 'warning' : 'success'}
                        className="text-sm"
                    >
                        {isDemoMode ? '🚧 Demo Mode' : '🚀 Production Mode'}
                    </Badge>
                    <span className="text-slate-400 text-sm">
                        {isDemoMode ? 'Demo Environment' : 'Production Environment'}
                    </span>
                </div>
            </div>

            {/* API Configuration */}
            <div>
                <h4 className="text-sm font-medium text-slate-300 mb-3">API Configuration</h4>
                <div className="bg-slate-800 rounded-lg p-4 space-y-2">
                    <div className="flex justify-between">
                        <span className="text-slate-400">API Endpoint:</span>
                        <span className="text-white font-mono text-sm">{api_url}</span>
                    </div>
                    <div className="flex justify-between">
                        <span className="text-slate-400">Reverse Proxy:</span>
                        <span className={capabilities.proxy_mode ? 'text-green-400' : 'text-slate-400'}>
                            {capabilities.proxy_mode ? 'Enabled' : 'Disabled'}
                        </span>
                    </div>
                </div>
            </div>

            {/* Capabilities */}
            <div>
                <h4 className="text-sm font-medium text-slate-300 mb-3">System Capabilities</h4>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div className="flex items-center justify-between p-3 bg-slate-800 rounded-lg">
                        <span className="text-slate-300">Webhooks</span>
                        <div className="flex items-center space-x-2">
                            {capabilities.webhooks_enabled ? (
                                <>
                                    <div className="w-2 h-2 bg-green-400 rounded-full"></div>
                                    <span className="text-green-400 text-sm">Enabled</span>
                                </>
                            ) : (
                                <>
                                    <div className="w-2 h-2 bg-red-400 rounded-full"></div>
                                    <span className="text-red-400 text-sm">Disabled</span>
                                </>
                            )}
                        </div>
                    </div>
                    <div className="flex items-center justify-between p-3 bg-slate-800 rounded-lg">
                        <span className="text-slate-300">External Access</span>
                        <div className="flex items-center space-x-2">
                            {capabilities.external_access ? (
                                <>
                                    <div className="w-2 h-2 bg-green-400 rounded-full"></div>
                                    <span className="text-green-400 text-sm">Available</span>
                                </>
                            ) : (
                                <>
                                    <div className="w-2 h-2 bg-amber-400 rounded-full"></div>
                                    <span className="text-amber-400 text-sm">Localhost Only</span>
                                </>
                            )}
                        </div>
                    </div>
                </div>
            </div>

            {/* Demo Mode Upgrade Instructions */}
            {isDemoMode && (
                <div className="bg-amber-900 bg-opacity-50 border border-amber-600 rounded-lg p-4">
                    <div className="flex items-start space-x-3">
                        <div className="text-amber-400 flex-shrink-0 mt-1">
                            <Icons.Warning />
                        </div>
                        <div>
                            <h5 className="text-amber-200 font-medium mb-2">Upgrade to Production Mode</h5>
                            <p className="text-amber-100 text-sm mb-3">
                                To enable webhooks and external access, upgrade to production mode by setting up a reverse proxy.
                            </p>
                            <div className="text-sm text-amber-100">
                                <p className="mb-2">Steps to upgrade:</p>
                                <ol className="list-decimal list-inside space-y-1 text-xs">
                                    <li>Set up nginx, caddy, or apache as reverse proxy (run <code className="bg-amber-800 px-1 rounded">./lrops.sh help</code> for guidance)</li>
                                    <li>Add <code className="bg-amber-800 px-1 rounded">LIVEREVIEW_REVERSE_PROXY=true</code> to .env</li>
                                    <li>Restart LiveReview services</li>
                                    <li>Configure your domain to point to the proxy</li>
                                </ol>
                            </div>
                        </div>
                    </div>
                </div>
            )}

            {/* Version Information */}
            {version && (
                <div>
                    <h4 className="text-sm font-medium text-slate-300 mb-3">Version Information</h4>
                    <div className="bg-slate-800 rounded-lg p-4 space-y-2">
                        <div className="flex justify-between">
                            <span className="text-slate-400">Version:</span>
                            <span className="text-white font-mono text-sm">{version.version || 'Unknown'}</span>
                        </div>
                        {version.gitCommit && (
                            <div className="flex justify-between">
                                <span className="text-slate-400">Git Commit:</span>
                                <span className="text-white font-mono text-sm">{version.gitCommit.substring(0, 8)}</span>
                            </div>
                        )}
                        {version.buildTime && (
                            <div className="flex justify-between">
                                <span className="text-slate-400">Build Time:</span>
                                <span className="text-white text-sm">{new Date(version.buildTime).toLocaleString()}</span>
                            </div>
                        )}
                    </div>
                </div>
            )}

            {/* Refresh Button */}
            <div className="flex justify-end">
                <Button 
                    onClick={onRefresh} 
                    variant="outline" 
                    size="sm"
                    isLoading={isLoading}
                >
                    <Icons.Refresh />
                    Refresh
                </Button>
            </div>
        </div>
    );
};

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
    const { isSuperAdmin, canManageCurrentOrg, currentOrg } = useOrgContext();
    const canAccessPrompts = isSuperAdmin || currentOrg?.role === 'owner';
    
    const activeTab = location.hash.substring(1) || 'general';
    const [productionUrl, setProductionUrl] = useState('');
    const [showSaved, setShowSaved] = useState(false);
    const [error, setError] = useState('');
    const [isLoading, setIsLoading] = useState(false);
    
    // Deployment tab state
    const [systemInfo, setSystemInfo] = useState<any>(null);
    const [deploymentLoading, setDeploymentLoading] = useState(false);

    // Tabs are defined below; we derive default tab after tabs array creation

    // Available tabs based on permissions
    const tabs = [
        ...(isSuperAdmin ? [{ id: 'instance', name: 'Instance', icon: <Icons.Settings /> }] : []),
        ...(isSuperAdmin ? [{ 
            id: 'deployment', 
            name: 'Deployment', 
            icon: (
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z" />
                </svg>
            )
        }] : []),
        // License tab visible only to super_admin or org owner
        ...((isSuperAdmin || currentOrg?.role === 'owner') ? [{ id: 'license', name: 'License', icon: (
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 7a4 4 0 10-4 4v1a1 1 0 001 1h1v1a1 1 0 001 1h1l3 3 3-3-3-3v-2a4 4 0 00-4-4z" />
            </svg>
        ) }] : []),
        ...(canAccessPrompts ? [{ id: 'prompts', name: 'Prompts', icon: <Icons.AI /> }] : []),
        // Learnings tab visible to org managers and above
        ...(canManageCurrentOrg ? [{ id: 'learnings', name: 'Learnings', icon: <Icons.List /> }] : []),
        ...(canManageCurrentOrg ? [{ 
            id: 'users', 
            name: 'User Management', 
            icon: (
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197m13.5-9a2.5 2.5 0 11-5 0 2.5 2.5 0 015 0z" />
                </svg>
            )
        }] : []),
    ];

    const tabIds = tabs.map(t => t.id);
    const firstTab = tabIds[0];

    // Ensure a valid default tab if hash missing or removed
    useEffect(() => {
        if (!location.hash && firstTab) {
            navigate(`#${firstTab}`, { replace: true });
        }
    }, [location.hash, firstTab, navigate]);

    // Redirect if current hash points to a hidden/removed tab
    useEffect(() => {
        if (activeTab && !tabIds.includes(activeTab) && firstTab) {
            navigate(`#${firstTab}`, { replace: true });
        }
    }, [activeTab, tabIds.join(':'), firstTab, navigate]);

    // Redirect away from prompts tab if user loses permission or navigates manually
    useEffect(() => {
        if (activeTab === 'prompts' && !canAccessPrompts) {
            const fallback = isSuperAdmin ? 'instance' : 'ai';
            navigate(`#${fallback}`, { replace: true });
        }
    }, [activeTab, canAccessPrompts, isSuperAdmin, navigate]);

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

    // Fetch system info for deployment tab (memoized to prevent infinite loops)
    const fetchSystemInfo = useCallback(async () => {
        try {
            const info = await apiClient.get('/system/info');
            setSystemInfo(info);
        } catch (error) {
            console.error('Failed to fetch system info:', error);
            setSystemInfo(null);
        }
    }, []);

    // Memoized refresh function to prevent infinite re-renders
    const handleRefreshSystemInfo = useCallback(() => {
        setDeploymentLoading(true);
        fetchSystemInfo().finally(() => setDeploymentLoading(false));
    }, [fetchSystemInfo]);

    // Fetch system info when deployment tab is first accessed
    useEffect(() => {
        if (activeTab === 'deployment' && !systemInfo && !deploymentLoading) {
            handleRefreshSystemInfo();
        }
    }, [activeTab, systemInfo, deploymentLoading, handleRefreshSystemInfo]);

    // Auto-populate production URL from browser if empty
    const getCurrentBrowserUrl = () => {
        const protocol = window.location.protocol;
        const hostname = window.location.hostname;
        const port = window.location.port;
        
        // Don't include port for standard ports (80, 443) or localhost
        if (port && port !== '80' && port !== '443' && hostname !== 'localhost') {
            return `${protocol}//${hostname}:${port}`;
        }
        return `${protocol}//${hostname}`;
    };

    const shouldShowAutoPopulateWarning = () => {
        const currentUrl = getCurrentBrowserUrl();
        const isLocalhost = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
        
        // Show warning if production URL is empty and we're not on localhost
        return !productionUrl && !isLocalhost && systemInfo?.deployment_mode === 'production';
    };

    const shouldShowDiscrepancyWarning = () => {
        const currentUrl = getCurrentBrowserUrl();
        const isLocalhost = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
        
        // Show warning if production URL doesn't match current URL
        // This should show in both demo and production modes when there's a mismatch
        return productionUrl && 
               !isLocalhost && 
               !productionUrl.includes(window.location.hostname);
    };

    const handleAutoPopulate = () => {
        const currentUrl = getCurrentBrowserUrl();
        setProductionUrl(currentUrl);
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
                } else {
                    // Auto-populate if empty and not localhost
                    const isLocalhost = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
                    if (!isLocalhost) {
                        const currentUrl = getCurrentBrowserUrl();
                        setProductionUrl(currentUrl);
                    }
                }
            } catch (error) {
                console.error('Failed to fetch production URL:', error);
                // Show a less intrusive message for initial load
                if ((error as any)?.status === 404) {
                    console.warn('API endpoint not found. The server may not be running or the endpoint may be incorrect.');
                }
                // Auto-populate if fetch failed and not localhost
                const isLocalhost = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
                if (!isLocalhost) {
                    const currentUrl = getCurrentBrowserUrl();
                    setProductionUrl(currentUrl);
                }
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
                    {activeTab === 'prompts' && canAccessPrompts && (
                        <Card>
                            <PromptsPage />
                        </Card>
                    )}
                    {activeTab === 'prompts' && !canAccessPrompts && (
                        <Card>
                            <div className="p-4 text-sm text-red-300">You do not have access to Prompts. Only organization owners and super administrators can view this section.</div>
                        </Card>
                    )}
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
                                    
                                    {shouldShowAutoPopulateWarning() && (
                                        <div className="mb-4 p-4 bg-yellow-900/50 border border-yellow-600 rounded-lg">
                                            <div className="flex items-start space-x-3">
                                                <div className="text-yellow-400 mt-0.5">
                                                    <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
                                                        <path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
                                                    </svg>
                                                </div>
                                                <div className="flex-1">
                                                    <p className="text-yellow-400 font-medium text-sm">Production URL Required</p>
                                                    <p className="text-yellow-300 text-sm mt-1">
                                                        You're running in production mode but no production URL is configured. 
                                                        This is required for OAuth integrations to work properly.
                                                    </p>
                                                    <button
                                                        onClick={handleAutoPopulate}
                                                        className="mt-2 text-xs text-yellow-400 hover:text-yellow-300 underline"
                                                    >
                                                        Auto-fill with current URL ({getCurrentBrowserUrl()})
                                                    </button>
                                                </div>
                                            </div>
                                        </div>
                                    )}

                                    {shouldShowDiscrepancyWarning() && (
                                        <div className="mb-4 p-4 bg-orange-900/50 border border-orange-600 rounded-lg">
                                            <div className="flex items-start space-x-3">
                                                <div className="text-orange-400 mt-0.5">
                                                    <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
                                                        <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd" />
                                                    </svg>
                                                </div>
                                                <div className="flex-1">
                                                    <p className="text-orange-400 font-medium text-sm">URL Mismatch Warning</p>
                                                    <p className="text-orange-300 text-sm mt-1">
                                                        Your production URL ({new URL(productionUrl).hostname}) doesn't match your current domain ({window.location.hostname}). 
                                                        {systemInfo?.deployment_mode === 'production' 
                                                            ? 'This may cause OAuth redirects to fail.' 
                                                            : 'You should update this when switching to production mode.'}
                                                    </p>
                                                    <button
                                                        onClick={handleAutoPopulate}
                                                        className="mt-2 text-xs text-orange-400 hover:text-orange-300 underline"
                                                    >
                                                        Update to current URL ({getCurrentBrowserUrl()})
                                                    </button>
                                                </div>
                                            </div>
                                        </div>
                                    )}
                                    
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

                    {activeTab === 'deployment' && isSuperAdmin && (
                        <Card>
                            <div className="flex items-center mb-6">
                                <div className="text-blue-400 mr-3">
                                    <svg className="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z" />
                                    </svg>
                                </div>
                                <div>
                                    <h3 className="font-medium text-white">Deployment Information</h3>
                                    <p className="text-sm text-slate-300">Current deployment mode and configuration</p>
                                </div>
                            </div>

                            <DeploymentSettings 
                                systemInfo={systemInfo}
                                isLoading={deploymentLoading}
                                onRefresh={handleRefreshSystemInfo}
                            />
                        </Card>
                    )}

                    {activeTab === 'license' && (isSuperAdmin || currentOrg?.role === 'owner') && (
                        <Card>
                            <LicenseTab />
                        </Card>
                    )}

                    {activeTab === 'learnings' && canManageCurrentOrg && (
                        <Card>
                            <LearningsTab />
                        </Card>
                    )}

                    {/* AI Configuration, UI Preferences, Global Admin temporarily hidden */}

                    {activeTab === 'users' && canManageCurrentOrg && (
                        <Card>
                            <UserManagement isSuperAdminView={false} />
                        </Card>
                    )}

                    {tabs.length === 0 && (
                        <Card>
                            <div className="text-slate-300 text-sm p-4">No settings available for your role right now.</div>
                        </Card>
                    )}
                </div>
            </div>
        </div>
    );
};

export default Settings;
