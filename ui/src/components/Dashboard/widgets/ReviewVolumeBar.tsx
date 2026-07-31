import React from 'react';
import ReactECharts from 'echarts-for-react';
import { useNavigate } from 'react-router-dom';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS } from './echartsTheme';
import { useChartResize } from './useChartResize';
import { MOCK_REVIEW_LAYERS } from './mockData';
import { useDashboardPeriod } from './DashboardPeriod';

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
    const { scale } = useDashboardPeriod();

    const layers = [...MOCK_REVIEW_LAYERS].sort((a, b) => b.reviewsRun - a.reviewsRun);
    const issuesByLabel: Record<string, number> = MOCK_REVIEW_LAYERS.reduce((acc, layer) => {
        acc[layer.label] = scale(layer.issuesFound);
        return acc;
    }, {} as Record<string, number>);

    const option = {
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
            data: layers.map((layer) => scale(layer.reviewsRun)),
        }],
    };

    const onEvents = {
        click: () => navigate('/reports?mode=explore'),
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
