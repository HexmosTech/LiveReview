import { createContext, useContext, useEffect } from 'react';

// Lets an empty dashboard card tell DashboardGrid it has nothing to draw, so the grid can shrink that card.
export const WidgetIdContext = createContext<string | null>(null);
export const WidgetEmptyReporterContext = createContext<((widgetId: string, empty: boolean) => void) | null>(null);

// Call from an empty state. It does nothing outside a dashboard card.
export function useReportWidgetEmpty(): void {
    const widgetId = useContext(WidgetIdContext);
    const report = useContext(WidgetEmptyReporterContext);
    useEffect(() => {
        if (!widgetId || !report) return;
        report(widgetId, true);
        return () => report(widgetId, false);
    }, [widgetId, report]);
}
