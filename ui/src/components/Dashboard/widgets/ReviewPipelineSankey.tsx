import React, { useMemo } from 'react';
import ReactECharts from 'echarts-for-react';
import { useNavigate } from 'react-router-dom';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS } from './echartsTheme';
import { useChartResize } from './useChartResize';
import { MOCK_REVIEW_LAYERS, ISSUE_CATEGORIES, CATEGORY_COLORS, IssueCategory } from './mockData';
import { useDashboardPeriod } from './DashboardPeriod';

const LAYER_COLORS: Record<string, string> = {
    'precommit': '#3B82F6',
    'mr-pr': '#7C3AED',
    'api-mcp': '#06B6D4',
    'scheduled': '#22C55E',
};

export const ReviewPipelineSankey: React.FC = () => {
    const { containerRef, chartRef } = useChartResize();
    const navigate = useNavigate();
    const { scale } = useDashboardPeriod();

    // Every issue found belongs to exactly one of these categories — there is no
    // separate "clean"/"no issues" bucket in this graph, only where issues landed.
    const nodes = useMemo(() => [
        ...MOCK_REVIEW_LAYERS.map((layer) => ({ name: layer.label, itemStyle: { color: LAYER_COLORS[layer.id] } })),
        ...ISSUE_CATEGORIES.map((category) => ({ name: category, itemStyle: { color: CATEGORY_COLORS[category] } })),
    ], []);

    const links = useMemo(() => MOCK_REVIEW_LAYERS.flatMap((layer) =>
        layer.categories
            .filter((c) => c.count > 0)
            .map((c) => ({ source: layer.label, target: c.category, value: scale(c.count) }))
    ), [scale]);

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
