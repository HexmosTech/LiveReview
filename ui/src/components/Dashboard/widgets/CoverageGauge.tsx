import React, { useMemo } from 'react';
import ReactECharts from 'echarts-for-react/lib/core';
import { useNavigate } from 'react-router-dom';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS, LR_ECHARTS_CORE } from './echartsTheme';
import { useChartResize } from './useChartResize';
import { useDashboardPeriod } from './DashboardPeriod';
import { useSystemOverview } from './SystemOverviewData';
import { EmptyState, Icons } from '../../UIPrimitives';
import { ChartSkeleton } from './ChartSkeleton';

export const CoverageGauge: React.FC = () => {
    const { containerRef, chartRef } = useChartResize();
    const navigate = useNavigate();
    const { period, label } = useDashboardPeriod();
    const { systemOverview, loading } = useSystemOverview();

    // Memoized so echarts only replays its entrance animation when the underlying data actually
    // changes, not on every unrelated re-render (e.g. DashboardGrid's ResizeObserver-driven
    // gridWidth updates) - an unstable option reference plus `notMerge` below meant the chart
    // was redrawing (and re-animating) itself a second time shortly after its real first
    // render. See docs/perf-improvement.md ("chart animation plays twice"). Computed
    // unconditionally (before the loading/empty early returns below) so the hook call stays
    // unconditional, same pattern as the other chart widgets.
    const coveragePct = Math.round(systemOverview?.coverage_pct[period] ?? 0);
    const option = useMemo(() => ({
        ...ECHARTS_ANIMATION_DEFAULTS,
        series: [{
            type: 'gauge',
            startAngle: 200,
            endAngle: -20,
            min: 0,
            max: 100,
            splitNumber: 5,
            radius: '95%',
            axisLine: {
                lineStyle: {
                    width: 14,
                    color: [[0.6, '#F43F5E'], [0.85, '#F59E0B'], [1, '#22C55E']],
                },
            },
            pointer: { itemStyle: { color: '#CBD5E1' } },
            axisTick: { distance: -14, length: 6, lineStyle: { color: '#0f172a', width: 1 } },
            splitLine: { distance: -16, length: 14, lineStyle: { color: '#0f172a', width: 2 } },
            axisLabel: { distance: -32, color: '#94A3B8', fontSize: 10 },
            anchor: { show: false },
            title: { show: true, offsetCenter: [0, '55%'], color: '#94A3B8', fontSize: 12 },
            detail: {
                valueAnimation: true,
                formatter: '{value}%',
                color: '#F1F5F9',
                fontSize: 26,
                fontWeight: 700,
                offsetCenter: [0, '25%'],
            },
            data: [{ value: coveragePct, name: `Coverage — ${label}` }],
        }],
    }), [coveragePct, label]);

    const onEvents = useMemo(() => ({
        click: () => navigate('/reports?mode=overview'),
    }), [navigate]);

    if (loading) {
        return <ChartSkeleton />;
    }

    if (!systemOverview || systemOverview.total_prs.all === 0) {
        return (
            <EmptyState
                icon={<Icons.EmptyState />}
                title="No PR data yet"
                description="Review coverage will appear here once PRs/MRs are tracked."
            />
        );
    }

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
        </div>
    );
};
