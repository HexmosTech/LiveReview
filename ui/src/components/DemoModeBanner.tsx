import React, { useState, useEffect } from 'react';
import apiClient from '../api/apiClient';
import { Icons } from './UIPrimitives';

interface SystemInfo {
  deployment_mode: 'demo' | 'production';
  capabilities: {
    webhooks_enabled: boolean;
    manual_triggers_only: boolean;
    external_access: boolean;
    proxy_mode: boolean;
  };
}

// Simple X close icon component
const CloseIcon: React.FC<{ className?: string }> = ({ className = "w-4 h-4" }) => (
  <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
  </svg>
);

export const DemoModeBanner: React.FC = () => {
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null);
  const [isDismissed, setIsDismissed] = useState(false);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const fetchSystemInfo = async () => {
      try {
        const info = await apiClient.get<SystemInfo>('/system/info');
        setSystemInfo(info);
      } catch (error) {
        console.warn('Failed to fetch system info for demo banner:', error);
        // Fallback: detect demo mode based on URL
        const hostname = window.location.hostname;
        if (hostname === 'localhost' || hostname === '127.0.0.1') {
          setSystemInfo({
            deployment_mode: 'demo',
            capabilities: {
              webhooks_enabled: false,
              manual_triggers_only: true,
              external_access: false,
              proxy_mode: false,
            },
          });
        }
      } finally {
        setIsLoading(false);
      }
    };

    fetchSystemInfo();
  }, []);

  // Don't render anything while loading
  if (isLoading) {
    return null;
  }

  // Don't render if not in demo mode or if dismissed
  if (!systemInfo || systemInfo.deployment_mode !== 'demo' || isDismissed) {
    return null;
  }

  const handleDismiss = () => {
    setIsDismissed(true);
  };

  const handleUpgrade = () => {
    // Open documentation in new tab
    window.open('/docs/deployment', '_blank');
  };

  return (
    <div className="bg-amber-600 border-b border-amber-700 px-4 py-3 text-white">
      <div className="flex items-center justify-between max-w-7xl mx-auto">
        <div className="flex items-center space-x-3">
          <div className="text-amber-100 flex-shrink-0">
            <Icons.Warning />
          </div>
          <div className="flex-1">
            <p className="text-sm font-medium">
              Demo Mode: Running in localhost-only mode
            </p>
            <p className="text-xs text-amber-100 mt-1">
              Webhooks are disabled. Only manual review triggers are available. 
              This mode is for local testing and demonstration purposes only.
            </p>
          </div>
        </div>
        <div className="flex items-center space-x-2 ml-4">
          <button
            onClick={handleUpgrade}
            className="text-xs bg-amber-700 hover:bg-amber-800 text-white px-3 py-1 rounded font-medium transition-colors duration-200"
          >
            Upgrade to Production
          </button>
          <button
            onClick={handleDismiss}
            className="text-amber-100 hover:text-white transition-colors duration-200"
            aria-label="Dismiss banner"
          >
            <CloseIcon />
          </button>
        </div>
      </div>
    </div>
  );
};