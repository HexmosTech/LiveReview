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
  const [isDismissed, setIsDismissed] = useState<boolean>(() => {
    try {
      return localStorage.getItem('lr_demo_banner_dismissed') === '1';
    } catch {
      return false;
    }
  });
  const [isCollapsed, setIsCollapsed] = useState<boolean>(() => {
    try {
      return localStorage.getItem('lr_demo_banner_collapsed') === '1';
    } catch {
      return false;
    }
  });
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
    try { localStorage.setItem('lr_demo_banner_dismissed', '1'); } catch {}
  };

  const toggleCollapsed = () => {
    setIsCollapsed((c) => {
      const next = !c;
      try { localStorage.setItem('lr_demo_banner_collapsed', next ? '1' : '0'); } catch {}
      return next;
    });
  };

  const handleUpgrade = () => {
    // Open documentation in new tab
    window.open('/docs/deployment', '_blank');
  };

  return (
    <div className="bg-amber-600 border-b border-amber-700 px-4 py-2 text-white">
      <div className="flex items-center justify-between max-w-7xl mx-auto">
        <div className="flex items-center space-x-3 min-w-0">
          <button
            onClick={toggleCollapsed}
            className="text-amber-100 hover:text-white transition-colors duration-200 focus:outline-none"
            aria-label={isCollapsed ? 'Expand demo banner' : 'Collapse demo banner'}
            title={isCollapsed ? 'Expand' : 'Collapse'}
          >
            {isCollapsed ? (
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="m6 15 6-6 6 6"/></svg>
            ) : (
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="m18 9-6 6-6-6"/></svg>
            )}
          </button>
          <div className="text-amber-100 flex-shrink-0">
            <Icons.Warning />
          </div>
          <div className="flex-1 overflow-hidden">
            <p className="text-sm font-medium truncate">
              Demo Mode: Running in localhost-only mode
            </p>
            {!isCollapsed && (
              <p className="text-xs text-amber-100 mt-1">
                Webhooks are disabled. Only manual review triggers are available. This mode is for local testing and demonstration purposes only.
              </p>
            )}
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