import React from 'react';
import ReactECharts from 'echarts-for-react';
import { useNavigate } from 'react-router-dom';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS } from './echartsTheme';
import { useChartResize } from './useChartResize';
import { MOCK_SYSTEM_KPIS } from './mockData';
import { useDashboardPeriod } from './DashboardPeriod';

export const CoverageGauge: React.FC = () => {
    const { containerRef, chartRef } = useChartResize();
    const navigate = useNavigate();
    const { label } = useDashboardPeriod();

    const option = {
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
            data: [{ value: MOCK_SYSTEM_KPIS.coverageScorePct, name: `Coverage — ${label}` }],
        }],
    };

    const onEvents = {
        click: () => navigate('/reports?mode=overview'),
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
        </div>
    );
};
