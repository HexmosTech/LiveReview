import React, { useMemo } from 'react';
import ReactECharts from 'echarts-for-react/lib/core';
import { useNavigate } from 'react-router-dom';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS, LR_ECHARTS_CORE } from './echartsTheme';
import { useChartResize } from './useChartResize';
import { useDashboardPeriod } from './DashboardPeriod';
import { useReviewLayers, hasNoReviewLayerData } from './ReviewLayersData';
import { EmptyState, Icons } from '../../UIPrimitives';
import { ChartSkeleton } from './ChartSkeleton';

const LAYER_COLORS: Record<string, string> = {
    'precommit': '#3B82F6',
    'mr-pr': '#7C3AED',
    'api-mcp': '#06B6D4',
    'scheduled': '#22C55E',
};

// Plain volume comparison, deliberately not a "funnel" — these four stages are
// independent trigger sources, not sequential steps in a single flow, so a
// tapering conversion-funnel shape would imply a drop-off relationship that
// doesn't exist between them.
export const ReviewVolumeBar: React.FC = () => {
    const { containerRef, chartRef } = useChartResize();
    const navigate = useNavigate();
    const { period } = useDashboardPeriod();
    const { reviewLayers, loading } = useReviewLayers();

    const allLayers = reviewLayers?.[period] ?? [];
    const layers = [...allLayers].sort((a, b) => b.reviews_run - a.reviews_run);

    // Memoized so echarts only replays its entrance animation when the underlying data actually
    // changes, not on every unrelated re-render (e.g. DashboardGrid's ResizeObserver-driven
    // gridWidth updates) - an unstable option reference plus `notMerge` below meant the chart
    // was redrawing (and re-animating) itself a second time shortly after its real first
    // render. See docs/perf-improvement.md ("chart animation plays twice"). Keyed on
    // `reviewLayers`/`period` (the stable upstream inputs) rather than `layers`/`allLayers`,
    // which are recomputed fresh every render.
    const option = useMemo(() => {
        const issuesByLabel: Record<string, number> = allLayers.reduce((acc, layer) => {
            acc[layer.label] = layer.issues_found;
            return acc;
        }, {} as Record<string, number>);

        return {
            ...ECHARTS_ANIMATION_DEFAULTS,
            grid: { left: 90, right: 24, top: 12, bottom: 12, containLabel: true },
            tooltip: {
                trigger: 'item',
                formatter: (params: { name: string; value: number }) =>
                    `${params.name}<br/>Reviews run: <b>${params.value.toLocaleString()}</b><br/>Issues found: <b>${(issuesByLabel[params.name] || 0).toLocaleString()}</b>`,
            },
            xAxis: { type: 'value', axisLabel: { color: '#94A3B8' } },
            yAxis: {
                type: 'category',
                data: layers.map((l) => l.label),
                inverse: true,
                axisLabel: { color: '#CBD5E1', fontSize: 12 },
                axisLine: { show: false },
                axisTick: { show: false },
            },
            series: [{
                type: 'bar',
                barWidth: '55%',
                label: { show: true, position: 'right', color: '#CBD5E1', formatter: (p: { value: number }) => p.value.toLocaleString() },
                itemStyle: {
                    borderRadius: [0, 4, 4, 0],
                    color: (p: { dataIndex: number }) => LAYER_COLORS[layers[p.dataIndex].id],
                },
                data: layers.map((layer) => layer.reviews_run),
            }],
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [reviewLayers, period]);

    const onEvents = {
        click: () => navigate('/reports?mode=explore'),
    };

    if (loading) {
        return <ChartSkeleton />;
    }

    if (hasNoReviewLayerData(layers)) {
        return (
            <EmptyState
                icon={<Icons.EmptyState />}
                title="No review volume data yet"
                description="Stage volume comparison will appear here once reviews run for this period."
            />
        );
    }

    return (
        <div ref={containerRef} className="w-full h-full">
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
    );
};
