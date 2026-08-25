import React, { createContext, useContext, useMemo } from 'react';
import { useDashboardQuery, IssueTreemapData } from '../../../api/dashboard';

interface IssueTreemapContextValue {
    issueTreemap: IssueTreemapData | null;
    loading: boolean;
    error: string | null;
    refetch: () => void;
}

const IssueTreemapContext = createContext<IssueTreemapContextValue | null>(null);

export const IssueTreemapProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const { data, isLoading, error, refetch } = useDashboardQuery();

    const issueTreemap = data?.issue_treemap ?? null;
    const value: IssueTreemapContextValue = useMemo(() => ({
        issueTreemap,
        loading: isLoading,
        error: error ? (error instanceof Error ? error.message : 'Failed to load issue treemap') : null,
        refetch: () => { void refetch(); },
    }), [issueTreemap, isLoading, error, refetch]);

    return (
        <IssueTreemapContext.Provider value={value}>
            {children}
        </IssueTreemapContext.Provider>
    );
};

export function useIssueTreemap(): IssueTreemapContextValue {
    const context = useContext(IssueTreemapContext);
    if (!context) {
        throw new Error('useIssueTreemap must be used within an IssueTreemapProvider');
    }
    return context;
}
