import React, { useState, useEffect, useCallback } from 'react';
import { useQuery } from '@tanstack/react-query';
import apiClient from '../api/apiClient';
import { SYSTEM_INFO_QUERY_KEY } from '../hooks/useSystemInfo';
import { useAppDispatch } from '../store/configureStore';
import { add, dismiss } from '../store/Notifications/slice';
import { registerNotificationAction } from '../store/Notifications/actionRegistry';
import { notify } from '../utils/notify';

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

const URL_MISMATCH_ID = 'url-mismatch';

// Headless watcher (renders nothing) — detects a production-URL/hostname
// mismatch and surfaces it as a notification (tray + one toast) instead of
// its own inline banner, so it's discoverable/dismissible like everything
// else in the notification system.
export const URLMismatchBanner: React.FC = () => {
  const dispatch = useAppDispatch();
  const [productionUrl, setProductionUrl] = useState<string>('');

  // Shares the same queryKey as useSystemInfo() (Navbar.tsx) so the two never fire duplicate
  // /system/info requests on mount - see docs/perf-improvement.md "Finding C".
  const { data: systemInfo } = useQuery<SystemInfo>({
    queryKey: SYSTEM_INFO_QUERY_KEY,
    queryFn: () => apiClient.get<SystemInfo>('/system/info'),
    staleTime: 5 * 60_000,
  });

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

  useEffect(() => {
    const fetchProductionUrl = async () => {
      try {
        const productionUrlResponse = await apiClient.get<ProductionURLResponse>('/production-url');
        if (productionUrlResponse && productionUrlResponse.url) {
          setProductionUrl(productionUrlResponse.url);
        }
      } catch (error) {
        console.warn('Failed to fetch production URL for URL mismatch banner:', error);
      }
    };

    fetchProductionUrl();
  }, []);

  const handleFixURL = useCallback(async () => {
    try {
      const currentUrl = getCurrentBrowserUrl();
      await apiClient.put('/production-url', { url: currentUrl });
      setProductionUrl(currentUrl);
      dispatch(dismiss(URL_MISMATCH_ID));
    } catch (error) {
      console.error('Failed to update production URL:', error);
      notify.error('Failed to update production URL.');
    }
  }, [dispatch]);

  const handleOpenSettings = useCallback(() => {
    window.location.href = '/#/settings#instance';
  }, []);

  useEffect(() => {
    const unregisterFix = registerNotificationAction(`${URL_MISMATCH_ID}:fix`, handleFixURL);
    const unregisterSettings = registerNotificationAction(`${URL_MISMATCH_ID}:settings`, handleOpenSettings);
    return () => {
      unregisterFix();
      unregisterSettings();
    };
  }, [handleFixURL, handleOpenSettings]);

  useEffect(() => {
    const isLocalhost = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
    if (isLocalhost || !productionUrl) {
      return;
    }

    let storedHostname = '';
    let mismatched = false;
    try {
      const storedURL = new URL(productionUrl);
      storedHostname = storedURL.hostname;
      mismatched = storedHostname !== window.location.hostname;
    } catch {
      return;
    }

    if (!mismatched) {
      dispatch(dismiss(URL_MISMATCH_ID));
      return;
    }

    const isInProductionMode = systemInfo?.deployment_mode === 'production';
    const message = `Your production URL (${storedHostname}) doesn't match your current domain (${window.location.hostname}). ${isInProductionMode
      ? 'This may cause OAuth redirects to fail.'
      : 'You should update this when switching to production mode.'
      }`;

    dispatch(
      add({
        dedupeKey: URL_MISMATCH_ID,
        severity: 'warning',
        title: 'URL Mismatch Warning',
        message,
        source: 'url-mismatch',
        toast: true,
        persistDismiss: false,
        actions: [
          { label: 'Fix URL', actionId: `${URL_MISMATCH_ID}:fix` },
          { label: 'Settings', actionId: `${URL_MISMATCH_ID}:settings` },
        ],
      })
    );
  }, [dispatch, productionUrl, systemInfo]);

  return null;
};
