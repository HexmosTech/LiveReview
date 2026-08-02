import React from 'react';
import { useNavigate } from 'react-router-dom';
import { StatCard, Icons } from '../../UIPrimitives';
import { MOCK_SYSTEM_KPIS } from './mockData';

export const AverageReviewsStat: React.FC = () => {
    const navigate = useNavigate();

    return (
        <div className="grid grid-cols-2 gap-3 h-full">
            <button type="button" onClick={() => navigate('/reports?mode=explore')} className="text-left h-full">
                <StatCard
                    title="Avg Reviews / PR"
                    value={MOCK_SYSTEM_KPIS.avgReviewsPerPR.toFixed(1)}
                    description="Across all connected repositories"
                    icon={<Icons.Reviews />}
                    className="h-full cursor-pointer"
                />
            </button>
            <button type="button" onClick={() => navigate('/reports?mode=explore')} className="text-left h-full">
                <StatCard
                    title="Avg Reviews / Commit"
                    value={MOCK_SYSTEM_KPIS.avgReviewsPerCommit.toFixed(2)}
                    description="Pre-commit + scheduled combined"
                    icon={<Icons.Reviews />}
                    className="h-full cursor-pointer"
                />
            </button>
        </div>
    );
};
