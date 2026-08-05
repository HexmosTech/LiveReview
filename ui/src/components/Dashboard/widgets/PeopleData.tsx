import React, { createContext, useCallback, useContext, useEffect, useState } from 'react';
import { getDashboardData, refreshDashboardData, People } from '../../../api/dashboard';

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

// Fetches people once for the whole widget grid, not once per widget.
interface PeopleContextValue {
    people: People | null;
    loading: boolean;
    error: string | null;
    refetch: () => void;
}

const PeopleContext = createContext<PeopleContextValue | null>(null);

export const PeopleProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [people, setPeople] = useState<People | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    // Passive load: reads whatever dashboard_cache already has — cheap, no live query.
    useEffect(() => {
        let cancelled = false;
        getDashboardData()
            .then((data) => {
                if (cancelled) return;
                setPeople(data.people ?? null);
            })
            .catch((err) => {
                if (cancelled) return;
                setError(err instanceof Error ? err.message : 'Failed to load people data');
            })
            .finally(() => {
                if (!cancelled) setLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, []);

    // Explicit refresh: recomputes and stores this org's people data on demand, then returns the fresh result.
    const refetch = useCallback(() => {
        let cancelled = false;
        setLoading(true);
        refreshDashboardData()
            .then((data) => {
                if (cancelled) return;
                setPeople(data.people ?? null);
                setError(null);
            })
            .catch((err) => {
                if (cancelled) return;
                setError(err instanceof Error ? err.message : 'Failed to refresh people data');
            })
            .finally(() => {
                if (!cancelled) setLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, []);

    return (
        <PeopleContext.Provider value={{ people, loading, error, refetch }}>
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
