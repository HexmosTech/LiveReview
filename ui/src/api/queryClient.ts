import { QueryClient } from '@tanstack/react-query';

// Shared cache so components that independently need the same server data (billing status,
// quota status, system info, dashboard, ...) share one in-flight request and one cached result
// instead of each firing its own fetch on mount. See docs/perf-improvement.md ("Fix 2").
export const queryClient = new QueryClient({
    defaultOptions: {
        queries: {
            staleTime: 30_000,
            gcTime: 5 * 60_000,
            refetchOnWindowFocus: false,
            retry: 1,
        },
    },
});
