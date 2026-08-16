import React from 'react';
import ReactECharts from 'echarts-for-react/lib/core';
import { useNavigate } from 'react-router-dom';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS, LR_ECHARTS_CORE } from './echartsTheme';
import { useChartResize } from './useChartResize';
import { useDashboardPeriod } from './DashboardPeriod';
import { usePeople, contributorColor } from './PeopleData';
import { EmptyState, Icons } from '../../UIPrimitives';
import { ChartSkeleton } from './ChartSkeleton';

const TOP_N = 5;

// Always buckets to top N + "Others" regardless of team size, so this stays
// readable whether the org has 12 contributors or 1,200 — the point is "who's
// driving usage," not a full per-person breakdown.
export const UsageShareDonut: React.FC = () => {
    const { containerRef, chartRef } = useChartResize();
    const navigate = useNavigate();
    const { period } = useDashboardPeriod();
    const { people, loading } = usePeople();

    if (loading) {
        return <ChartSkeleton />;
    }

    const contributors = people?.contributors[period] ?? [];
    const total = contributors.reduce((sum, c) => sum + c.loc_reviewed, 0);
    if (contributors.length === 0 || total === 0) {
        return (
            <EmptyState
                icon={<Icons.EmptyState />}
                title="No usage data yet"
                description="Usage share by contributor will appear here once reviews start running."
            />
        );
    }

    // A contributor with loc_reviewed=0 has nothing to show — drop it rather than render an
    // invisible zero-degree slice that still occupies a legend entry (same reasoning as the
    // repo hierarchy sunburst's pr_count=0 filter).
    const byUsage = [...contributors]
        .filter((c) => c.loc_reviewed > 0)
        .sort((a, b) => b.loc_reviewed - a.loc_reviewed);
    const top = byUsage.slice(0, TOP_N);
    const others = byUsage.slice(TOP_N);
    const othersTotal = others.reduce((sum, c) => sum + c.loc_reviewed, 0);

    const pieData = [
        ...top.map((c) => ({ name: c.name.split(' ')[0], value: c.loc_reviewed, itemStyle: { color: contributorColor(c.email) } })),
        ...(othersTotal > 0 ? [{ name: 'Others', value: othersTotal, itemStyle: { color: '#475569' } }] : []),
    ];

    const topShare = Math.round((top[0].loc_reviewed / total) * 100);

    const option = {
        ...ECHARTS_ANIMATION_DEFAULTS,
        tooltip: { trigger: 'item', formatter: '{b}: {d}%' },
        legend: { bottom: 0, textStyle: { color: '#CBD5E1', fontSize: 11 } },
        title: {
            text: top[0].name.split(' ')[0],
            subtext: `${topShare}% of usage`,
            left: 'center',
            top: '38%',
            textStyle: { color: '#F1F5F9', fontSize: 16, fontWeight: 700 },
            subtextStyle: { color: '#94A3B8', fontSize: 11 },
            itemGap: 6,
        },
        series: [{
            type: 'pie',
            radius: ['48%', '72%'],
            center: ['50%', '46%'],
            avoidLabelOverlap: true,
            label: { show: false },
            labelLine: { show: false },
            data: pieData,
        }],
    };

    const onEvents = {
        click: () => navigate('/settings#users'),
    };

    return (
        <div className="w-full h-full flex flex-col">
            <div ref={containerRef} className="flex-1 min-h-0">
                <ReactECharts
                    ref={chartRef}
                    echarts={LR_ECHARTS_CORE}
                    option={option}
                    theme={LIVEREVIEW_ECHARTS_THEME}
                    style={{ height: '100%', width: '100%' }}
                    onEvents={onEvents}
                    notMerge
                />
            </div>
            <p className="text-center text-[11px] text-slate-500 shrink-0 -mt-1">
                Top {TOP_N} of {byUsage.length} contributors shown
            </p>
        </div>
    );
};
