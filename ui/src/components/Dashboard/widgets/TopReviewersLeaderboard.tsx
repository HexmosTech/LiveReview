import React from 'react';
import { useNavigate } from 'react-router-dom';
import { EmptyState, Icons } from '../../UIPrimitives';
import { useDashboardPeriod } from './DashboardPeriod';
import { usePeople, contributorColor, contributorInitials } from './PeopleData';
import { ChartSkeleton } from './ChartSkeleton';

const RANK_BADGES = ['🥇', '🥈', '🥉'];

export const TopReviewersLeaderboard: React.FC = () => {
    const navigate = useNavigate();
    const { period } = useDashboardPeriod();
    const { people, loading } = usePeople();

    if (loading) {
        return <ChartSkeleton />;
    }

    const contributors = people?.contributors[period] ?? [];
    if (contributors.length === 0) {
        return (
            <EmptyState
                icon={<Icons.EmptyState />}
                title="No reviews yet"
                description="Top reviewers will appear here once reviews start running."
            />
        );
    }

    const top = contributors.slice(0, 8);
    const maxReviews = Math.max(...top.map((c) => c.reviews_given));

    return (
        <div className="h-full overflow-y-auto space-y-2 pr-1">
            {top.map((contributor, index) => {
                const color = contributorColor(contributor.email);
                return (
                    <button
                        key={contributor.email}
                        type="button"
                        onClick={() => navigate('/settings#users')}
                        className="w-full flex items-center gap-3 text-left rounded-lg hover:bg-slate-900/40 p-1.5 transition-colors"
                    >
                        <span className="w-5 text-center text-xs text-slate-500 shrink-0">
                            {RANK_BADGES[index] || index + 1}
                        </span>
                        <div
                            className="w-9 h-9 rounded-full flex items-center justify-center text-xs font-semibold shrink-0"
                            style={{ backgroundColor: `${color}26`, color }}
                        >
                            {contributorInitials(contributor.name)}
                        </div>
                        <div className="flex-1 min-w-0">
                            <div className="flex items-center justify-between gap-2">
                                <p className="text-sm text-slate-100 truncate">{contributor.name}</p>
                                <p className="text-xs font-medium text-slate-300 shrink-0">{contributor.reviews_given} reviews</p>
                            </div>
                            <div className="mt-1 h-1.5 rounded-full bg-slate-700/60 overflow-hidden">
                                <div
                                    className="h-full rounded-full"
                                    style={{
                                        width: `${(contributor.reviews_given / maxReviews) * 100}%`,
                                        backgroundColor: color,
                                    }}
                                />
                            </div>
                        </div>
                    </button>
                );
            })}
        </div>
    );
};
