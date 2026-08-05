import React from 'react';
import { useNavigate } from 'react-router-dom';
import { StatCard, Icons, EmptyState } from '../../UIPrimitives';
import { useCountUp } from './useCountUp';
import { useDashboardPeriod } from './DashboardPeriod';
import { useSystemOverview, hasNoSystemOverviewData } from './SystemOverviewData';
import { ChartSkeleton } from './ChartSkeleton';

interface KpiTileProps {
    title: string;
    value: number;
    icon: React.ReactNode;
    onClick: () => void;
}

const KpiTile: React.FC<KpiTileProps> = ({ title, value, icon, onClick }) => {
    const animated = useCountUp(value);
    return (
        <button type="button" onClick={onClick} className="text-left h-full">
            <StatCard
                variant="primary"
                title={title}
                value={Math.round(animated).toLocaleString()}
                icon={icon}
                className="h-full cursor-pointer hover:border-l-blue-400"
            />
        </button>
    );
};

export const SystemKpiRow: React.FC = () => {
    const navigate = useNavigate();
    const { period } = useDashboardPeriod();
    const { systemOverview, loading } = useSystemOverview();

    if (loading) {
        return <ChartSkeleton />;
    }

    if (hasNoSystemOverviewData(systemOverview)) {
        return (
            <EmptyState
                icon={<Icons.EmptyState />}
                title="Nothing connected yet"
                description="Git hosts, AI connectors, and repositories will appear here once you connect them."
            />
        );
    }

    const overview = systemOverview!;

    return (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 h-full">
            <KpiTile title="Git Hosts" value={overview.git_hosts[period]} icon={<Icons.Git />} onClick={() => navigate('/git')} />
            <KpiTile title="AI Connectors" value={overview.ai_connectors[period]} icon={<Icons.AI />} onClick={() => navigate('/ai')} />
            <KpiTile title="Repositories" value={overview.total_repos[period]} icon={<Icons.Folder />} onClick={() => navigate('/explore/repositories')} />
            <KpiTile title="PRs / MRs Tracked" value={overview.total_prs[period]} icon={<Icons.Layers />} onClick={() => navigate('/explore/merge-requests')} />
        </div>
    );
};
