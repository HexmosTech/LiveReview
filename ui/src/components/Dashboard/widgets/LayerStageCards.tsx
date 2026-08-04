import React from 'react';
import { useNavigate } from 'react-router-dom';
import { useDashboardPeriod } from './DashboardPeriod';
import { useReviewLayers, hasNoReviewLayerData } from './ReviewLayersData';
import { EmptyState, Icons } from '../../UIPrimitives';
import { ChartSkeleton } from './ChartSkeleton';

export const LayerStageCards: React.FC = () => {
    const navigate = useNavigate();
    const { period } = useDashboardPeriod();
    const { reviewLayers, loading } = useReviewLayers();

    const layers = reviewLayers?.[period] ?? [];

    if (loading) {
        return <ChartSkeleton />;
    }

    if (hasNoReviewLayerData(layers)) {
        return (
            <EmptyState
                icon={<Icons.EmptyState />}
                title="No reviews yet"
                description="Review counts by stage will appear here once reviews run for this period."
            />
        );
    }

    return (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 h-full">
            {layers.map((layer) => (
                <button
                    key={layer.id}
                    type="button"
                    onClick={() => navigate('/reports?mode=explore')}
                    className="text-left rounded-lg bg-slate-900/50 border border-slate-700/60 hover:border-slate-600 hover:bg-slate-900/70 transition-colors p-3.5 flex flex-col justify-between"
                >
                    <p className="text-sm font-medium text-slate-300">{layer.label}</p>
                    <div className="mt-2">
                        {/* No trend indicator — backend doesn't compute period-over-period change yet. */}
                        <span className="text-2xl font-bold text-white">{layer.reviews_run.toLocaleString()}</span>
                        <p className="mt-1 text-xs text-slate-400">{layer.issues_found.toLocaleString()} issues found</p>
                    </div>
                </button>
            ))}
        </div>
    );
};
