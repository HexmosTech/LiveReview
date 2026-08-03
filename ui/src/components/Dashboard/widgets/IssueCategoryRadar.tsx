import React from 'react';
import ReactECharts from 'echarts-for-react';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS } from './echartsTheme';
import { useChartResize } from './useChartResize';
import { MOCK_REVIEW_LAYERS, ISSUE_CATEGORIES } from './mockData';

const COMPARED_LAYER_IDS: Array<typeof MOCK_REVIEW_LAYERS[number]['id']> = ['precommit', 'mr-pr', 'scheduled'];
const SERIES_COLORS: Record<string, string> = {
    'precommit': '#3B82F6',
    'mr-pr': '#7C3AED',
    'scheduled': '#22C55E',
};

export const IssueCategoryRadar: React.FC = () => {
    const { containerRef, chartRef } = useChartResize();

    const comparedLayers = MOCK_REVIEW_LAYERS.filter((layer) => COMPARED_LAYER_IDS.includes(layer.id));
    const maxPerCategory = Math.max(
        ...comparedLayers.flatMap((layer) => layer.categories.map((c) => c.count))
    );
    const indicatorMax = Math.ceil(maxPerCategory * 1.2 / 10) * 10;

    const option = {
        ...ECHARTS_ANIMATION_DEFAULTS,
        tooltip: {},
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

    return (
        <div ref={containerRef} className="w-full h-full">
            <ReactECharts
                ref={chartRef}
                option={option}
                theme={LIVEREVIEW_ECHARTS_THEME}
                style={{ height: '100%', width: '100%' }}
                notMerge
            />
        </div>
    );
};
