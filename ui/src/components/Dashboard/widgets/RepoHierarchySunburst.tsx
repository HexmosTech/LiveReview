import React, { useState } from 'react';
import ReactECharts from 'echarts-for-react';
import classNames from 'classnames';
import { useNavigate } from 'react-router-dom';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS } from './echartsTheme';
import { useChartResize } from './useChartResize';
import { useDashboardPeriod } from './DashboardPeriod';
import { useSystemOverview } from './SystemOverviewData';
import { EmptyState, Icons } from '../../UIPrimitives';
import { ChartSkeleton } from './ChartSkeleton';

const HOST_COLORS: Record<string, string> = {
    GitHub: '#3B82F6',
    GitLab: '#F59E0B',
    Bitbucket: '#06B6D4',
    Gitea: '#22C55E',
};

const coverageColor = (pct: number): string => {
    if (pct >= 90) return '#22C55E';
    if (pct >= 75) return '#7C3AED';
    return '#F43F5E';
};

// An org can have hundreds of repos per host — showing all of them makes the outer ring an illegible wall of rotated text, so each host caps to its top N by PR count, same "top N + Others" pattern as the Usage Share donut.
const TOP_REPOS_PER_HOST = 10;

type ViewMode = 'sunburst' | 'treemap';

export const RepoHierarchySunburst: React.FC = () => {
    const { containerRef, chartRef } = useChartResize();
    const [view, setView] = useState<ViewMode>('sunburst');
    const navigate = useNavigate();
    const { period } = useDashboardPeriod();
    const { systemOverview, loading } = useSystemOverview();

    if (loading) {
        return <ChartSkeleton />;
    }

    const repoHierarchy = systemOverview?.repo_hierarchy ?? [];
    if (repoHierarchy.length === 0) {
        return (
            <EmptyState
                icon={<Icons.EmptyState />}
                title="No repositories yet"
                description="PR volume by repository will appear here once repositories are synced."
            />
        );
    }

    // A repo with pr_count=0 for this period has nothing to show — drop it rather than render a zero-width sliver (same reasoning as review_layers' Sankey/Radar).
    const hierarchyData = repoHierarchy
        .map((host) => {
            const withPRs = [...host.repos]
                .filter((repo) => repo.pr_count[period] > 0)
                .sort((a, b) => b.pr_count[period] - a.pr_count[period]);
            const top = withPRs.slice(0, TOP_REPOS_PER_HOST);
            const others = withPRs.slice(TOP_REPOS_PER_HOST);
            const othersTotal = others.reduce((sum, repo) => sum + repo.pr_count[period], 0);

            return {
                name: host.host,
                itemStyle: { color: HOST_COLORS[host.host] || '#3B82F6' },
                children: [
                    ...top.map((repo) => ({
                        name: repo.name,
                        value: repo.pr_count[period],
                        itemStyle: { color: coverageColor(repo.coverage_pct[period]) },
                    })),
                    ...(othersTotal > 0 ? [{
                        name: `${others.length} more repos`,
                        value: othersTotal,
                        itemStyle: { color: '#475569' },
                    }] : []),
                ],
            };
        })
        .filter((host) => host.children.length > 0);

    if (hierarchyData.length === 0) {
        return (
            <EmptyState
                icon={<Icons.EmptyState />}
                title="No PR activity in this period"
                description="Try a wider period, or check back once new PRs come in."
            />
        );
    }

    const sunburstOption = {
        ...ECHARTS_ANIMATION_DEFAULTS,
        tooltip: { formatter: '{b}: {c} PRs' },
        series: [{
            type: 'sunburst',
            radius: ['12%', '90%'],
            data: hierarchyData,
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
            data: hierarchyData,
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
