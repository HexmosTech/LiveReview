import React, { useState } from 'react';
import ReactECharts from 'echarts-for-react';
import classNames from 'classnames';
import { useNavigate } from 'react-router-dom';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS } from './echartsTheme';
import { useChartResize } from './useChartResize';
import { MOCK_REPO_TREE } from './mockData';

const HOST_COLORS: Record<string, string> = {
    GitHub: '#3B82F6',
    GitLab: '#F59E0B',
    Bitbucket: '#06B6D4',
};

const coverageColor = (pct: number): string => {
    if (pct >= 90) return '#22C55E';
    if (pct >= 75) return '#7C3AED';
    return '#F43F5E';
};

const HIERARCHY_DATA = MOCK_REPO_TREE.map((host) => ({
    name: host.name,
    itemStyle: { color: HOST_COLORS[host.name] || '#3B82F6' },
    children: host.repos.map((repo) => ({
        name: repo.name,
        value: repo.prCount,
        itemStyle: { color: coverageColor(repo.coveragePct) },
    })),
}));

type ViewMode = 'sunburst' | 'treemap';

export const RepoHierarchySunburst: React.FC = () => {
    const { containerRef, chartRef } = useChartResize();
    const [view, setView] = useState<ViewMode>('sunburst');
    const navigate = useNavigate();

    const sunburstOption = {
        ...ECHARTS_ANIMATION_DEFAULTS,
        tooltip: { formatter: '{b}: {c} PRs' },
        series: [{
            type: 'sunburst',
            radius: ['12%', '90%'],
            data: HIERARCHY_DATA,
            label: { color: '#F1F5F9', fontSize: 11 },
            emphasis: { focus: 'ancestor' },
            levels: [
                {},
                { r0: '12%', r: '45%', itemStyle: { borderWidth: 2 } },
                { r0: '45%', r: '90%', label: { rotate: 'tangential' } },
            ],
        }],
    };

    const treemapOption = {
        ...ECHARTS_ANIMATION_DEFAULTS,
        tooltip: { formatter: '{b}: {c} PRs' },
        series: [{
            type: 'treemap',
            data: HIERARCHY_DATA,
            label: { color: '#F1F5F9', fontSize: 11 },
            upperLabel: { show: true, height: 22, color: '#F1F5F9' },
            breadcrumb: { show: false },
            itemStyle: { borderColor: 'rgba(15, 23, 42, 0.6)', borderWidth: 1, gapWidth: 1 },
            roam: false,
        }],
    };

    const onEvents = {
        click: (params: { data?: { children?: unknown[] } }) => {
            const isHostNode = Boolean(params.data?.children);
            navigate(isHostNode ? '/git' : '/explore/repositories');
        },
    };

    return (
        <div className="w-full h-full flex flex-col">
            <div className="flex justify-end gap-1 mb-1 shrink-0">
                {(['sunburst', 'treemap'] as ViewMode[]).map((mode) => (
                    <button
                        key={mode}
                        type="button"
                        onClick={() => setView(mode)}
                        className={classNames(
                            'text-[11px] px-2 py-0.5 rounded-full capitalize transition-colors',
                            view === mode ? 'bg-blue-500/20 text-blue-300' : 'text-slate-500 hover:text-slate-300'
                        )}
                    >
                        {mode}
                    </button>
                ))}
            </div>
            <div ref={containerRef} className="flex-1 min-h-0">
                <ReactECharts
                    ref={chartRef}
                    option={view === 'sunburst' ? sunburstOption : treemapOption}
                    theme={LIVEREVIEW_ECHARTS_THEME}
                    style={{ height: '100%', width: '100%' }}
                    onEvents={onEvents}
                    notMerge
                />
            </div>
        </div>
    );
};
