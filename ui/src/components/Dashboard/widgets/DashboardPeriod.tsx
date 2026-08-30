import React, { createContext, useContext, useMemo, useState } from 'react';

export type DashboardPeriod = 'day' | 'week' | 'month' | 'all';

export const PERIOD_LABELS: Record<DashboardPeriod, string> = {
    day: 'Today',
    week: 'This Week',
    month: 'This Month',
    all: 'All Time',
};

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

export const DashboardPeriodProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [period, setPeriod] = useState<DashboardPeriod>('month');

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
    }), [period]);

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
