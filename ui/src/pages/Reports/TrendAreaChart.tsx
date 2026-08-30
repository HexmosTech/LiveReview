import React from 'react';
import ReactECharts from 'echarts-for-react/lib/core';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS, LR_ECHARTS_CORE } from '../../components/Dashboard/widgets/echartsTheme';
import { useChartResize } from '../../components/Dashboard/widgets/useChartResize';
import type { FilledTrendRow } from './TaxonomyReports';

// Composed area+line trend chart. Previously a recharts component - ported to echarts (already
// loaded for the Dashboard widgets, so this doesn't add a separate charting library just for one
// report page) since recharts here was otherwise this app's only consumer of that entire
// dependency. See docs/perf-improvement.md for the investigation behind this.
export const TrendAreaChart: React.FC<{
    rows: FilledTrendRow[];
    height?: number;
    showBrush?: boolean;
    showLegend?: boolean;
}> = ({ rows, height = 120, showBrush = true, showLegend = true }) => {
    const { containerRef, chartRef } = useChartResize();

    if (rows.length === 0) {
        return <p className="text-slate-400 text-xs">No trend data in the selected range.</p>;
    }

    const option = {
        ...ECHARTS_ANIMATION_DEFAULTS,
        grid: { left: 0, right: 12, top: 8, bottom: showBrush ? 34 : 8, containLabel: true },
        tooltip: { trigger: 'axis' },
        legend: showLegend ? { bottom: showBrush ? 22 : 0, textStyle: { fontSize: 11 } } : undefined,
        xAxis: { type: 'category', data: rows.map((r) => r.bucket), boundaryGap: false },
        yAxis: { type: 'value' },
        dataZoom: showBrush ? [{ type: 'slider', height: 14, bottom: 0 }] : undefined,
        series: [
            {
                name: 'Findings',
                type: 'line',
                data: rows.map((r) => r.count),
                areaStyle: { opacity: 0.2 },
                itemStyle: { color: '#3b82f6' },
                smooth: true,
                showSymbol: false,
            },
            {
                name: 'Reviews',
                type: 'line',
                data: rows.map((r) => r.review_count),
                itemStyle: { color: '#22c55e' },
                smooth: true,
                showSymbol: false,
            },
        ],
    };

    return (
        <div className="space-y-2">
            <div className="w-full rounded bg-slate-900/50 border border-slate-700 p-2" style={{ height }}>
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
            </div>
        </div>
    );
};
