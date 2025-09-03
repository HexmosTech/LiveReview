import React, { useState, useEffect } from 'react';
import apiClient from '../api/apiClient';
import { Icons } from './UIPrimitives';

interface SystemInfo {
  deployment_mode: 'demo' | 'production';
  current_url?: string;
  capabilities: {
    webhooks_enabled: boolean;
    manual_triggers_only: boolean;
    external_access: boolean;
    proxy_mode: boolean;
  };
}

interface ProductionURLResponse {
  url: string;
  success: boolean;
  message: string;
}

// Simple X close icon component
const CloseIcon: React.FC<{ className?: string }> = ({ className = "w-4 h-4" }) => (
  <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
  </svg>
);

export const URLMismatchBanner: React.FC = () => {
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null);
  const [productionUrl, setProductionUrl] = useState<string>('');
  const [isDismissed, setIsDismissed] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [isFixing, setIsFixing] = useState(false);

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

  const shouldShowMismatchWarning = (): boolean => {
    const isLocalhost = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
    
    // Don't show warning for localhost
    if (isLocalhost) return false;
    
    // Don't show if no production URL is set
    if (!productionUrl) return false;
    
    // Show warning if production URL doesn't match current hostname
    try {
      const storedURL = new URL(productionUrl);
      const currentHostname = window.location.hostname;
      return storedURL.hostname !== currentHostname;
    } catch (error) {
      return false;
    }
  };

  useEffect(() => {
    const fetchData = async () => {
      try {
        // Fetch both system info and production URL
        const [systemInfoResponse, productionUrlResponse] = await Promise.all([
          apiClient.get<SystemInfo>('/system/info'),
          apiClient.get<ProductionURLResponse>('/production-url')
        ]);
        
        setSystemInfo(systemInfoResponse);
        if (productionUrlResponse && productionUrlResponse.url) {
          setProductionUrl(productionUrlResponse.url);
        }
      } catch (error) {
        console.warn('Failed to fetch data for URL mismatch banner:', error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchData();
  }, []);

  // Don't render anything while loading
  if (isLoading) {
    return null;
  }

  // Don't render if dismissed or no mismatch
  if (isDismissed || !shouldShowMismatchWarning()) {
    return null;
  }

  const handleDismiss = () => {
    setIsDismissed(true);
  };

  const handleFixURL = async () => {
    setIsFixing(true);
    try {
      const currentUrl = getCurrentBrowserUrl();
      await apiClient.put('/production-url', { url: currentUrl });
      setProductionUrl(currentUrl);
      setIsDismissed(true); // Hide banner after fixing
    } catch (error) {
      console.error('Failed to update production URL:', error);
    } finally {
      setIsFixing(false);
    }
  };

  const handleOpenSettings = () => {
    window.location.href = '/#/settings#instance';
  };

  const storedHostname = productionUrl ? new URL(productionUrl).hostname : '';
  const currentHostname = window.location.hostname;
  const isInProductionMode = systemInfo?.deployment_mode === 'production';

  return (
    <div className="bg-orange-600 border-b border-orange-700 px-4 py-3 text-white">
      <div className="flex items-center justify-between max-w-7xl mx-auto">
        <div className="flex items-center space-x-3">
          <div className="text-orange-100 flex-shrink-0">
            <Icons.Warning />
          </div>
          <div className="flex-1">
            <p className="text-sm font-medium">
              URL Mismatch Warning
            </p>
            <p className="text-xs text-orange-100 mt-1">
              Your production URL ({storedHostname}) doesn't match your current domain ({currentHostname}). 
              {isInProductionMode 
                ? ' This may cause OAuth redirects to fail.' 
                : ' You should update this when switching to production mode.'}
            </p>
          </div>
        </div>
        <div className="flex items-center space-x-2 ml-4">
          <button
            onClick={handleFixURL}
            disabled={isFixing}
            className="text-xs bg-orange-700 hover:bg-orange-800 disabled:bg-orange-800 text-white px-3 py-1 rounded font-medium transition-colors duration-200"
          >
            {isFixing ? 'Updating...' : 'Fix URL'}
          </button>
          <button
            onClick={handleOpenSettings}
            className="text-xs bg-transparent hover:bg-orange-700 text-orange-100 hover:text-white border border-orange-400 px-3 py-1 rounded font-medium transition-colors duration-200"
          >
            Settings
          </button>
          <button
            onClick={handleDismiss}
            className="text-orange-100 hover:text-white transition-colors duration-200"
            aria-label="Dismiss banner"
          >
            <CloseIcon />
          </button>
        </div>
      </div>
    </div>
  );
};