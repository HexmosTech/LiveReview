import React, { createContext, useCallback, useContext, useEffect, useState } from 'react';
import { getDashboardData, refreshDashboardData, SystemOverview } from '../../../api/dashboard';

// An org with nothing connected yet has every count at 0 regardless of period — checking "all" (the broadest window) is enough to know the whole widget set is empty.
export function hasNoSystemOverviewData(overview: SystemOverview | null): boolean {
    if (!overview) return true;
    return overview.git_hosts.all === 0 && overview.ai_connectors.all === 0 && overview.total_repos.all === 0 && overview.total_prs.all === 0;
}

// Fetches system_overview once for the whole widget grid, not once per widget.
interface SystemOverviewContextValue {
    systemOverview: SystemOverview | null;
    loading: boolean;
    error: string | null;
    refetch: () => void;
}

const SystemOverviewContext = createContext<SystemOverviewContextValue | null>(null);

export const SystemOverviewProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [systemOverview, setSystemOverview] = useState<SystemOverview | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    // Passive load: reads whatever dashboard_cache already has — cheap, no live query.
    useEffect(() => {
        let cancelled = false;
        getDashboardData()
            .then((data) => {
                if (cancelled) return;
                setSystemOverview(data.system_overview ?? null);
            })
            .catch((err) => {
                if (cancelled) return;
                setError(err instanceof Error ? err.message : 'Failed to load system overview');
            })
            .finally(() => {
                if (!cancelled) setLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, []);

    // Explicit refresh: recomputes and stores this org's system_overview on demand, then returns the fresh result.
    const refetch = useCallback(() => {
        let cancelled = false;
        setLoading(true);
        refreshDashboardData()
            .then((data) => {
                if (cancelled) return;
                setSystemOverview(data.system_overview ?? null);
                setError(null);
            })
            .catch((err) => {
                if (cancelled) return;
                setError(err instanceof Error ? err.message : 'Failed to refresh system overview');
            })
            .finally(() => {
                if (!cancelled) setLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, []);

    return (
        <SystemOverviewContext.Provider value={{ systemOverview, loading, error, refetch }}>
            {children}
        </SystemOverviewContext.Provider>
    );
};

export function useSystemOverview(): SystemOverviewContextValue {
    const context = useContext(SystemOverviewContext);
    if (!context) {
        throw new Error('useSystemOverview must be used within a SystemOverviewProvider');
    }
    return context;
}
