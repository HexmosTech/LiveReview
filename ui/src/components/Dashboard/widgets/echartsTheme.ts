import * as echarts from 'echarts';

// Registered once at module load. Matches the app's existing dark slate/blue/purple/green
// palette (see tailwind.config.js `livereview` colors + UIPrimitives Badge variants) so
// ECharts widgets look native inside the existing Card chrome rather than boxed-in.
export const LIVEREVIEW_ECHARTS_THEME = 'livereview-dark';

const AXIS_LINE_COLOR = '#334155'; // slate-700
const AXIS_LABEL_COLOR = '#94A3B8'; // slate-400
const TEXT_COLOR = '#CBD5E1'; // slate-300
const EMPHASIS_TEXT_COLOR = '#F1F5F9'; // slate-100

echarts.registerTheme(LIVEREVIEW_ECHARTS_THEME, {
    color: ['#3B82F6', '#7C3AED', '#22C55E', '#F59E0B', '#F43F5E', '#06B6D4', '#A855F7'],
    backgroundColor: 'transparent',
    textStyle: {
        color: TEXT_COLOR,
        fontFamily: 'inherit',
    },
    title: {
        textStyle: { color: EMPHASIS_TEXT_COLOR, fontSize: 14, fontWeight: 600 },
        subtextStyle: { color: AXIS_LABEL_COLOR },
    },
    line: { itemStyle: { borderWidth: 2 }, lineStyle: { width: 2 }, symbolSize: 6, smooth: true },
    categoryAxis: {
        axisLine: { lineStyle: { color: AXIS_LINE_COLOR } },
        axisTick: { lineStyle: { color: AXIS_LINE_COLOR } },
        axisLabel: { color: AXIS_LABEL_COLOR, fontSize: 12 },
        splitLine: { lineStyle: { color: [AXIS_LINE_COLOR], opacity: 0.4 } },
    },
    valueAxis: {
        axisLine: { show: false },
        axisTick: { show: false },
        axisLabel: { color: AXIS_LABEL_COLOR, fontSize: 12 },
        splitLine: { lineStyle: { color: [AXIS_LINE_COLOR], opacity: 0.4 } },
    },
    legend: {
        textStyle: { color: TEXT_COLOR, fontSize: 12 },
        pageTextStyle: { color: TEXT_COLOR },
    },
    tooltip: {
        backgroundColor: 'rgba(15, 23, 42, 0.95)',
        borderColor: AXIS_LINE_COLOR,
        borderWidth: 1,
        textStyle: { color: EMPHASIS_TEXT_COLOR, fontSize: 12 },
        extraCssText: 'border-radius: 8px; backdrop-filter: blur(4px); box-shadow: 0 10px 30px rgba(0,0,0,0.35);',
    },
    visualMap: {
        textStyle: { color: TEXT_COLOR },
    },
    calendar: {
        dayLabel: { color: AXIS_LABEL_COLOR },
        monthLabel: { color: TEXT_COLOR },
        itemStyle: { color: 'transparent', borderColor: AXIS_LINE_COLOR },
    },
});

export const ECHARTS_ANIMATION_DEFAULTS = {
    animationDuration: 800,
    animationEasing: 'cubicOut' as const,
};
