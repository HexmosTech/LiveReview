import React, { createContext, useContext } from 'react';
import { useDashboardQuery, SystemOverview } from '../../../api/dashboard';

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

// Reads system_overview off the shared dashboard query (see useDashboardQuery) instead of
// fetching it independently - the whole widget grid shares one cached request/result.
export const SystemOverviewProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const { data, isLoading, error, refetch } = useDashboardQuery();

    const value: SystemOverviewContextValue = {
        systemOverview: data?.system_overview ?? null,
        loading: isLoading,
        error: error ? (error instanceof Error ? error.message : 'Failed to load system overview') : null,
        refetch: () => { void refetch(); },
    };

    return (
        <SystemOverviewContext.Provider value={value}>
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
