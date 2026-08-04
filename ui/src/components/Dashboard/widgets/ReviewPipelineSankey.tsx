import React, { useMemo } from 'react';
import ReactECharts from 'echarts-for-react';
import { useNavigate } from 'react-router-dom';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS } from './echartsTheme';
import { useChartResize } from './useChartResize';
import { ISSUE_CATEGORIES, CATEGORY_COLORS } from './mockData';
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

export const ReviewPipelineSankey: React.FC = () => {
    const { containerRef, chartRef } = useChartResize();
    const navigate = useNavigate();
    const { period } = useDashboardPeriod();
    const { reviewLayers, loading } = useReviewLayers();

    // Already period-scoped by the backend — no client-side rescaling needed.
    const layers = reviewLayers?.[period] ?? [];

    // A layer only gets a node if it actually ran reviews.
    const activeLayers = useMemo(() => layers.filter((layer) => layer.reviews_run > 0), [layers]);

    // depth pins each node to its column explicitly, since a linkless node can't infer one from topology.
    const nodes = useMemo(() => [
        ...activeLayers.map((layer) => ({ name: layer.label, depth: 0, itemStyle: { color: LAYER_COLORS[layer.id] } })),
        ...ISSUE_CATEGORIES.map((category) => ({ name: category, depth: 1, itemStyle: { color: CATEGORY_COLORS[category] } })),
    ], [activeLayers]);

    const links = useMemo(() => activeLayers.flatMap((layer) =>
        layer.categories
            .filter((c) => c.count > 0)
            .map((c) => ({ source: layer.label, target: c.category, value: c.count }))
    ), [activeLayers]);

    const option = {
        ...ECHARTS_ANIMATION_DEFAULTS,
        tooltip: { trigger: 'item', triggerOn: 'mousemove' },
        series: [{
            type: 'sankey',
            emphasis: { focus: 'adjacency' },
            nodeWidth: 16,
            nodeGap: 12,
            data: nodes,
            links,
            label: { color: '#CBD5E1', fontSize: 12 },
            lineStyle: { color: 'gradient', curveness: 0.5, opacity: 0.4 },
        }],
    };

    const onEvents = {
        click: (params: { dataType: string; name: string }) => {
            if (params.dataType === 'node' && (ISSUE_CATEGORIES as readonly string[]).includes(params.name)) {
                navigate(`/reports?mode=explore&category=${encodeURIComponent(params.name)}`);
            }
        },
    };

    if (loading) {
        return <ChartSkeleton />;
    }

    if (hasNoReviewLayerData(layers)) {
        return (
            <EmptyState
                icon={<Icons.EmptyState />}
                title="No review pipeline data yet"
                description="The pipeline breakdown will appear here once reviews run for this org and period."
            />
        );
    }

    return (
        <div ref={containerRef} className="w-full h-full">
            <ReactECharts
                ref={chartRef}
                option={option}
                theme={LIVEREVIEW_ECHARTS_THEME}
                style={{ height: '100%', width: '100%' }}
                onEvents={onEvents}
                notMerge
            />
        </div>
    );
};
