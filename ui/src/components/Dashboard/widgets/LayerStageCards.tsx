import React from 'react';
import { useNavigate } from 'react-router-dom';
import { MOCK_REVIEW_LAYERS } from './mockData';
import { useDashboardPeriod } from './DashboardPeriod';

export const LayerStageCards: React.FC = () => {
    const navigate = useNavigate();
    const { scale } = useDashboardPeriod();

    return (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 h-full">
            {MOCK_REVIEW_LAYERS.map((layer) => {
                const reviewsRun = scale(layer.reviewsRun);
                const issuesFound = scale(layer.issuesFound);
                return (
                    <button
                        key={layer.id}
                        type="button"
                        onClick={() => navigate('/reports?mode=explore')}
                        className="text-left rounded-lg bg-slate-900/50 border border-slate-700/60 hover:border-slate-600 hover:bg-slate-900/70 transition-colors p-3.5 flex flex-col justify-between"
                    >
                        <p className="text-sm font-medium text-slate-300">{layer.label}</p>
                        <div className="mt-2">
                            <div className="flex items-baseline gap-2">
                                <span className="text-2xl font-bold text-white">{reviewsRun.toLocaleString()}</span>
                                <span
                                    className={
                                        layer.trendPct >= 0
                                            ? 'text-xs font-medium text-green-400'
                                            : 'text-xs font-medium text-red-400'
                                    }
                                >
                                    {layer.trendPct >= 0 ? '↑' : '↓'} {Math.abs(layer.trendPct)}%
                                </span>
                            </div>
                            <p className="mt-1 text-xs text-slate-400">{issuesFound.toLocaleString()} issues found</p>
                        </div>
                    </button>
                );
            })}
        </div>
    );
};
