import React, { useMemo } from 'react';
import ReactECharts from 'echarts-for-react/lib/core';
import { useNavigate } from 'react-router-dom';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS, LR_ECHARTS_CORE } from './echartsTheme';
import { useChartResize } from './useChartResize';
import { usePeople } from './PeopleData';
import { EmptyState, Icons } from '../../UIPrimitives';
import { ChartSkeleton } from './ChartSkeleton';

const LOOKBACK_DAYS = 210; // matches calendarLookbackDays server-side

const toISODate = (date: Date): string => date.toISOString().slice(0, 10);

// Not period-scoped — always the full lookback range, regardless of the Day/Week/Month/All selector.
export const ContributionCalendarHeatmap: React.FC = () => {
    const { containerRef, chartRef } = useChartResize();
    const navigate = useNavigate();
    const { people, loading } = usePeople();
    const calendar = people?.contribution_calendar ?? [];

    // Memoized so echarts only replays its entrance animation when the underlying data actually
    // changes, not on every unrelated re-render (e.g. DashboardGrid's ResizeObserver-driven
    // gridWidth updates) - an unstable option reference plus `notMerge` below meant the chart
    // was redrawing (and re-animating) itself a second time shortly after its real first
    // render. See docs/perf-improvement.md ("chart animation plays twice"). Computed
    // unconditionally (before the loading/empty early returns below) so the hook call stays
    // unconditional, same pattern as the other chart widgets.
    const option = useMemo(() => {
        const maxCount = calendar.length > 0 ? Math.max(...calendar.map((d) => d.count)) : 0;
        const today = new Date();
        const rangeStart = new Date(today.getTime() - LOOKBACK_DAYS * 24 * 60 * 60 * 1000);

        return {
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
                range: [toISODate(rangeStart), toISODate(today)],
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
                data: calendar.map((day) => [day.date, day.count]),
            }],
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [calendar]);

    const onEvents = useMemo(() => ({
        click: (params: { data?: [string, number] }) => {
            const date = params.data?.[0];
            if (date) navigate(`/reports?mode=explore&since=${date}&until=${date}`);
        },
    }), [navigate]);

    if (loading) {
        return <ChartSkeleton />;
    }

    if (calendar.length === 0) {
        return (
            <EmptyState
                icon={<Icons.EmptyState />}
                title="No activity yet"
                description="Daily review activity will appear here once reviews start running."
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
