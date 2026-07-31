import React from 'react';
import ReactECharts from 'echarts-for-react';
import { useNavigate } from 'react-router-dom';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS } from './echartsTheme';
import { useChartResize } from './useChartResize';
import { MOCK_CALENDAR_ACTIVITY } from './mockData';

export const ContributionCalendarHeatmap: React.FC = () => {
    const { containerRef, chartRef } = useChartResize();
    const navigate = useNavigate();

    const maxCount = Math.max(...MOCK_CALENDAR_ACTIVITY.map((d) => d.count));

    const option = {
        ...ECHARTS_ANIMATION_DEFAULTS,
        tooltip: {
            formatter: (params: { data: [string, number] }) =>
                `${params.data[0]}<br/><b>${params.data[1]}</b> reviews`,
        },
        visualMap: {
            min: 0,
            max: maxCount,
            show: false,
            calculable: false,
            inRange: { color: ['#1e293b', '#1d4ed8', '#7C3AED'] },
        },
        calendar: {
            top: 24,
            left: 32,
            right: 16,
            bottom: 8,
            range: ['2026-01-01', '2026-07-28'],
            cellSize: ['auto', 15],
            itemStyle: { borderWidth: 3, borderColor: 'transparent' },
            yearLabel: { show: false },
            dayLabel: { color: '#94A3B8', fontSize: 10, nameMap: 'en' },
            monthLabel: { color: '#CBD5E1', fontSize: 11 },
            splitLine: { show: false },
        },
        series: [{
            type: 'heatmap',
            coordinateSystem: 'calendar',
            data: MOCK_CALENDAR_ACTIVITY.map((day) => [day.date, day.count]),
        }],
    };

    const onEvents = {
        click: (params: { data?: [string, number] }) => {
            const date = params.data?.[0];
            if (date) navigate(`/reports?mode=explore&since=${date}&until=${date}`);
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
