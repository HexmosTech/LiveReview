import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';

export type DashboardPeriod = 'day' | 'week' | 'month' | 'all';

export const PERIOD_LABELS: Record<DashboardPeriod, string> = {
    day: 'Today',
    week: 'This Week',
    month: 'This Month',
    all: 'All Time',
};

const DEFAULT_PERIOD: DashboardPeriod = 'all';

// Matches the `lr_<feature>_${user.id}` localStorage convention already used by
// the dashboard layout and notifications (see useDashboardLayout.ts).
const storageKeyFor = (userId?: number | string): string =>
    userId ? `lr_dashboard_period_${userId}` : 'lr_dashboard_period';

function loadStoredPeriod(key: string): DashboardPeriod | null {
    try {
        const raw = localStorage.getItem(key);
        return raw && raw in PERIOD_LABELS ? (raw as DashboardPeriod) : null;
    } catch {
        return null;
    }
}

function saveStoredPeriod(key: string, period: DashboardPeriod): void {
    try {
        localStorage.setItem(key, period);
    } catch {
        // no-op: keep the dashboard functional when localStorage is unavailable
    }
}

// The mock volume numbers elsewhere in this feature represent a ~1 month baseline.
// These multipliers rescale them for the other period options so the selector
// actually changes what's on screen, without needing real time-series data yet.
const PERIOD_MULTIPLIERS: Record<DashboardPeriod, number> = {
    day: 1 / 30,
    week: 1 / 4.3,
    month: 1,
    all: 7, // matches the ~7 months of mock history in the contribution calendar
};

interface DashboardPeriodContextValue {
    period: DashboardPeriod;
    setPeriod: (period: DashboardPeriod) => void;
    label: string;
    scale: (monthlyValue: number) => number;
}

const DashboardPeriodContext = createContext<DashboardPeriodContextValue | null>(null);

interface DashboardPeriodProviderProps {
    children: React.ReactNode;
    userId?: number | string;
}

export const DashboardPeriodProvider: React.FC<DashboardPeriodProviderProps> = ({ children, userId }) => {
    const storageKey = storageKeyFor(userId);
    const [period, setPeriodState] = useState<DashboardPeriod>(() => loadStoredPeriod(storageKey) ?? DEFAULT_PERIOD);

    // userId is undefined until /auth/me resolves, so re-read once the real
    // per-user key is known - otherwise the saved range is never restored.
    useEffect(() => {
        setPeriodState(loadStoredPeriod(storageKey) ?? DEFAULT_PERIOD);
    }, [storageKey]);

    // Persist on every change so the selected range survives a refresh.
    const setPeriod = useCallback((next: DashboardPeriod) => {
        setPeriodState(next);
        saveStoredPeriod(storageKey, next);
    }, [storageKey]);

    // Memoized so this only produces a new object when `period` actually changes - otherwise
    // every unrelated re-render higher up the tree (e.g. DashboardGrid's ResizeObserver-driven
    // gridWidth updates) creates a new context value, which re-renders every consumer, including
    // every echarts widget - each of which then rebuilds its `option` object and re-triggers
    // echarts' entrance animation even though nothing it actually draws changed. See
    // docs/perf-improvement.md ("chart animation plays twice").
    const value: DashboardPeriodContextValue = useMemo(() => ({
        period,
        setPeriod,
        label: PERIOD_LABELS[period],
        scale: (monthlyValue: number) => Math.max(0, Math.round(monthlyValue * PERIOD_MULTIPLIERS[period])),
    }), [period, setPeriod]);

    return (
        <DashboardPeriodContext.Provider value={value}>
            {children}
        </DashboardPeriodContext.Provider>
    );
};

export function useDashboardPeriod(): DashboardPeriodContextValue {
    const context = useContext(DashboardPeriodContext);
    if (!context) {
        throw new Error('useDashboardPeriod must be used within a DashboardPeriodProvider');
    }
    return context;
}
