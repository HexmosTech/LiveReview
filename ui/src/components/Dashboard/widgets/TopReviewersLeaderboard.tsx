import React from 'react';
import { useNavigate } from 'react-router-dom';
import { MOCK_CONTRIBUTORS } from './mockData';

const RANK_BADGES = ['🥇', '🥈', '🥉'];

export const TopReviewersLeaderboard: React.FC = () => {
    const navigate = useNavigate();
    const top = MOCK_CONTRIBUTORS.slice(0, 8);
    const maxReviews = Math.max(...top.map((c) => c.reviewsGiven));

    return (
        <div className="h-full overflow-y-auto space-y-2 pr-1">
            {top.map((contributor, index) => (
                <button
                    key={contributor.id}
                    type="button"
                    onClick={() => navigate('/settings#users')}
                    className="w-full flex items-center gap-3 text-left rounded-lg hover:bg-slate-900/40 p-1.5 transition-colors"
                >
                    <span className="w-5 text-center text-xs text-slate-500 shrink-0">
                        {RANK_BADGES[index] || index + 1}
                    </span>
                    <div
                        className="w-9 h-9 rounded-full flex items-center justify-center text-xs font-semibold shrink-0"
                        style={{ backgroundColor: `${contributor.color}26`, color: contributor.color }}
                    >
                        {contributor.initials}
                    </div>
                    <div className="flex-1 min-w-0">
                        <div className="flex items-center justify-between gap-2">
                            <p className="text-sm text-slate-100 truncate">{contributor.name}</p>
                            <p className="text-xs font-medium text-slate-300 shrink-0">{contributor.reviewsGiven} reviews</p>
                        </div>
                        <div className="mt-1 h-1.5 rounded-full bg-slate-700/60 overflow-hidden">
                            <div
                                className="h-full rounded-full"
                                style={{
                                    width: `${(contributor.reviewsGiven / maxReviews) * 100}%`,
                                    backgroundColor: contributor.color,
                                }}
                            />
                        </div>
                    </div>
                </button>
            ))}
        </div>
    );
};
