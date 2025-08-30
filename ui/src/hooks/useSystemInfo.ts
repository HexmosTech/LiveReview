import { useState, useEffect } from 'react';
import { getSystemInfo } from '../api/auth';

interface SystemInfo {
  dev_mode: boolean;
  version: any;
}

/**
 * Hook to fetch and track system information including dev mode status
 */
export const useSystemInfo = () => {
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchSystemInfo = async () => {
      try {
        setLoading(true);
        setError(null);
        const info = await getSystemInfo();
        setSystemInfo(info);
      } catch (err) {
        console.error('Failed to fetch system info:', err);
        setError(err instanceof Error ? err.message : 'Failed to fetch system info');
        // Set default values if API fails
        setSystemInfo({
          dev_mode: false,
          version: null
        });
      } finally {
        setLoading(false);
      }
    };

    fetchSystemInfo();
  }, []);

  return {
    systemInfo,
    loading,
    error,
    isDevMode: systemInfo?.dev_mode ?? false
  };
};