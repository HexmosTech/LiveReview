import { useEffect, useRef } from 'react';
import type ReactECharts from 'echarts-for-react';

/**
 * echarts-for-react auto-resizes on *window* resize, but dragging/resizing a widget
 * inside react-grid-layout changes the container size without a window resize event —
 * without this, a resized widget leaves its chart canvas stretched/stale.
 */
export function useChartResize<T extends HTMLElement = HTMLDivElement>() {
    const containerRef = useRef<T | null>(null);
    const chartRef = useRef<ReactECharts | null>(null);

    useEffect(() => {
        const container = containerRef.current;
        if (!container) return;

        const observer = new ResizeObserver(() => {
            chartRef.current?.getEchartsInstance().resize();
        });
        observer.observe(container);

        return () => observer.disconnect();
    }, []);

    return { containerRef, chartRef };
}
