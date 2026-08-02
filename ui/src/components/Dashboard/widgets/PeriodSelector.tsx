import React from 'react';
import classNames from 'classnames';
import { useDashboardPeriod, DashboardPeriod } from './DashboardPeriod';

const OPTIONS: { value: DashboardPeriod; label: string }[] = [
    { value: 'day', label: 'Day' },
    { value: 'week', label: 'Week' },
    { value: 'month', label: 'Month' },
    { value: 'all', label: 'All' },
];

export const PeriodSelector: React.FC = () => {
    const { period, setPeriod } = useDashboardPeriod();

    return (
        <div className="inline-flex items-center rounded-full bg-slate-900/70 border border-slate-700/80 p-1 gap-0.5">
            {OPTIONS.map((option) => (
                <button
                    key={option.value}
                    type="button"
                    onClick={() => setPeriod(option.value)}
                    className={classNames(
                        'px-3 py-1 text-xs font-medium rounded-full transition-colors',
                        period === option.value
                            ? 'bg-blue-600 text-white'
                            : 'text-slate-400 hover:text-slate-200'
                    )}
                >
                    {option.label}
                </button>
            ))}
        </div>
    );
};
