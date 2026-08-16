import React, { createContext, useContext } from 'react';
import { useDashboardQuery, People } from '../../../api/dashboard';

// Backend only sends email/name/counts — color and initials are derived client-side, same as the old mock data did.
const CONTRIBUTOR_COLORS = ['#3B82F6', '#7C3AED', '#22C55E', '#F59E0B', '#F43F5E', '#06B6D4', '#A855F7'];

// Hashes email into the palette so the same person gets the same color in both Top Reviewers and Usage Share, regardless of each widget's own sort order.
export function contributorColor(email: string): string {
    let hash = 0;
    for (let i = 0; i < email.length; i++) {
        hash = (hash * 31 + email.charCodeAt(i)) | 0;
    }
    return CONTRIBUTOR_COLORS[Math.abs(hash) % CONTRIBUTOR_COLORS.length];
}

export function contributorInitials(name: string): string {
    return name.split(/\s+/).filter(Boolean).map((part) => part[0]).slice(0, 2).join('').toUpperCase();
}

interface PeopleContextValue {
    people: People | null;
    loading: boolean;
    error: string | null;
    refetch: () => void;
}

const PeopleContext = createContext<PeopleContextValue | null>(null);

// Reads people data off the shared dashboard query (see useDashboardQuery) instead of fetching
// it independently - the whole widget grid (this + ReviewLayers + SystemOverview + Dashboard
// itself) share one cached request/result rather than each firing its own.
export const PeopleProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const { data, isLoading, error, refetch } = useDashboardQuery();

    const value: PeopleContextValue = {
        people: data?.people ?? null,
        loading: isLoading,
        error: error ? (error instanceof Error ? error.message : 'Failed to load people data') : null,
        refetch: () => { void refetch(); },
    };

    return (
        <PeopleContext.Provider value={value}>
            {children}
        </PeopleContext.Provider>
    );
};

export function usePeople(): PeopleContextValue {
    const context = useContext(PeopleContext);
    if (!context) {
        throw new Error('usePeople must be used within a PeopleProvider');
    }
    return context;
}
