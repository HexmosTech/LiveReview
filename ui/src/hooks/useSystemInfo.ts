import { useQuery } from '@tanstack/react-query';
import { getSystemInfo } from '../api/auth';

interface SystemInfo {
  dev_mode: boolean;
  version: any;
}

// Exported so other components fetching the same /system/info endpoint (e.g.
// URLMismatchBanner, which needs fields beyond dev_mode/version) can share this cache
// entry via the same queryKey instead of firing their own duplicate request.
export const SYSTEM_INFO_QUERY_KEY = ['system-info'] as const;

/**
 * Hook to fetch and track system information including dev mode status. Backed by react-query
 * so every component calling this hook shares one cached request/result instead of each firing
 * its own fetch on mount (previously the biggest source of it) - see docs/perf-improvement.md.
 */
export const useSystemInfo = () => {
  const { data, isLoading, error } = useQuery<SystemInfo>({
    queryKey: SYSTEM_INFO_QUERY_KEY,
    queryFn: getSystemInfo,
    staleTime: 5 * 60_000, // barely changes within a session
  });

  // Preserve the original hook's error fallback: show a default (non-null) systemInfo rather
  // than leaving consumers with null when the fetch fails.
  const systemInfo = data ?? (error ? { dev_mode: false, version: null } : null);

  return {
    systemInfo,
    loading: isLoading,
    error: error ? (error instanceof Error ? error.message : 'Failed to fetch system info') : null,
    isDevMode: systemInfo?.dev_mode ?? false
  };
};