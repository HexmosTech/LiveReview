import React from 'react';
import { useNavigate } from 'react-router-dom';
import { StatCard, Icons } from '../../UIPrimitives';
import { useCountUp } from './useCountUp';
import { MOCK_SYSTEM_KPIS } from './mockData';

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

    return (
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3 h-full">
            <KpiTile title="Git Hosts" value={MOCK_SYSTEM_KPIS.gitHosts} icon={<Icons.Git />} onClick={() => navigate('/git')} />
            <KpiTile title="AI Connectors" value={MOCK_SYSTEM_KPIS.aiConnectors} icon={<Icons.AI />} onClick={() => navigate('/ai')} />
            <KpiTile title="Repositories" value={MOCK_SYSTEM_KPIS.totalRepos} icon={<Icons.Folder />} onClick={() => navigate('/explore/repositories')} />
            <KpiTile title="PRs / MRs Tracked" value={MOCK_SYSTEM_KPIS.totalPRs} icon={<Icons.Layers />} onClick={() => navigate('/explore/merge-requests')} />
            <KpiTile title="Third-Party Tools" value={15} icon={<Icons.Settings />} onClick={() => navigate('/settings#third-party-tools')} />
        </div>
    );
};
