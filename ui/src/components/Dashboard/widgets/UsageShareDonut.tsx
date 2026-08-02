import React from 'react';
import ReactECharts from 'echarts-for-react';
import { useNavigate } from 'react-router-dom';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS } from './echartsTheme';
import { useChartResize } from './useChartResize';
import { MOCK_CONTRIBUTORS } from './mockData';

const TOP_N = 5;

// Always buckets to top N + "Others" regardless of team size, so this stays
// readable whether the org has 12 contributors or 1,200 — the point is "who's
// driving usage," not a full per-person breakdown.
export const UsageShareDonut: React.FC = () => {
    const { containerRef, chartRef } = useChartResize();
    const navigate = useNavigate();

    const byUsage = [...MOCK_CONTRIBUTORS].sort((a, b) => b.locReviewed - a.locReviewed);
    const top = byUsage.slice(0, TOP_N);
    const others = byUsage.slice(TOP_N);
    const othersTotal = others.reduce((sum, c) => sum + c.locReviewed, 0);
    const total = byUsage.reduce((sum, c) => sum + c.locReviewed, 0);

    const pieData = [
        ...top.map((c) => ({ name: c.name.split(' ')[0], value: c.locReviewed, itemStyle: { color: c.color } })),
        ...(othersTotal > 0 ? [{ name: 'Others', value: othersTotal, itemStyle: { color: '#475569' } }] : []),
    ];

    const topShare = Math.round((top[0].locReviewed / total) * 100);

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
