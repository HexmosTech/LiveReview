import React, { useMemo } from 'react';
import ReactECharts from 'echarts-for-react/lib/core';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS, LR_ECHARTS_CORE } from './echartsTheme';
import { useChartResize } from './useChartResize';
import { ISSUE_CATEGORIES } from './mockData';
import { useDashboardPeriod } from './DashboardPeriod';
import { useReviewLayers, hasNoReviewLayerData, layersWithCategoryData } from './ReviewLayersData';
import { EmptyState, Icons } from '../../UIPrimitives';
import { ChartSkeleton } from './ChartSkeleton';

const SERIES_COLORS: Record<string, string> = {
    'precommit': '#3B82F6',
    'mr-pr': '#7C3AED',
    'api-mcp': '#06B6D4',
    'scheduled': '#22C55E',
};

export const IssueCategoryRadar: React.FC = () => {
    const { containerRef, chartRef } = useChartResize();
    const { period } = useDashboardPeriod();
    const { reviewLayers, loading } = useReviewLayers();

    const allLayers = reviewLayers?.[period] ?? [];
    // A layer with reviews but zero issues has nothing to plot, so it's dropped rather than shown as an empty legend entry.
    const comparedLayers = layersWithCategoryData(allLayers);

    // Memoized so echarts only replays its entrance animation when the underlying data actually
    // changes, not on every unrelated re-render (e.g. DashboardGrid's ResizeObserver-driven
    // gridWidth updates) - an unstable option reference plus `notMerge` below meant the chart
    // was redrawing (and re-animating) itself a second time shortly after its real first
    // render. See docs/perf-improvement.md ("chart animation plays twice"). Keyed on
    // `reviewLayers`/`period` (the stable upstream inputs) rather than `comparedLayers` itself,
    // which is recomputed fresh every render.
    const option = useMemo(() => {
        const maxPerCategory = Math.max(
            0,
            ...comparedLayers.flatMap((layer) => layer.categories.map((c) => c.count))
        );
        const indicatorMax = Math.ceil(maxPerCategory * 1.2 / 10) * 10;

        return {
            ...ECHARTS_ANIMATION_DEFAULTS,
            // confine keeps the tooltip within this chart's own box — without it, the radar's
            // full category breakdown (tall content) can spill outside the widget card and
            // overlap whatever's above it in the dashboard grid.
            tooltip: { confine: true },
            legend: { bottom: 8, textStyle: { color: '#CBD5E1', fontSize: 13 }, itemWidth: 16, itemHeight: 16 },
            radar: {
                center: ['50%', '48%'],
                radius: '58%',
                nameGap: 10,
                indicator: ISSUE_CATEGORIES.map((category) => ({ name: category, max: indicatorMax })),
                axisName: { color: '#CBD5E1', fontSize: 12 },
                splitLine: { lineStyle: { color: '#334155' } },
                splitArea: { areaStyle: { color: ['rgba(255,255,255,0.015)', 'rgba(255,255,255,0.04)'] } },
                axisLine: { lineStyle: { color: '#334155' } },
            },
            series: [{
                type: 'radar',
                data: comparedLayers.map((layer) => ({
                    name: layer.label,
                    value: ISSUE_CATEGORIES.map((category) => layer.categories.find((c) => c.category === category)?.count || 0),
                    areaStyle: { opacity: 0.18 },
                    lineStyle: { color: SERIES_COLORS[layer.id], width: 3 },
                    itemStyle: { color: SERIES_COLORS[layer.id] },
                    symbolSize: 7,
                })),
            }],
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [reviewLayers, period]);

    if (loading) {
        return <ChartSkeleton />;
    }

    if (hasNoReviewLayerData(comparedLayers)) {
        return (
            <EmptyState
                icon={<Icons.EmptyState />}
                title="No issue data yet"
                description="Issue category comparisons will appear here once reviews have run for this period."
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
                notMerge
            />
        </div>
    );
};
