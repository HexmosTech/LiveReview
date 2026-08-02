import { useCallback, useRef, useState } from 'react';
import type { Layout } from 'react-grid-layout/legacy';
import { WIDGET_REGISTRY, WidgetDefinition } from './registry';

const STORAGE_VERSION = 1;

interface StoredDashboardLayout {
    version: number;
    layout: Layout;
    widgetIds: string[];
}

// Matches the existing `lr_<feature>_${user.id}` localStorage convention already used
// elsewhere on the Dashboard (see onboarding-stepper / connector-warning dismissal).
const storageKeyFor = (userId?: number | string): string =>
    userId ? `lr_dashboard_layout_${userId}` : 'lr_dashboard_layout';

const defaultLayout = (): Layout =>
    WIDGET_REGISTRY.map((widget) => ({
        i: widget.id,
        x: widget.defaultLayout.x,
        y: widget.defaultLayout.y,
        w: widget.defaultLayout.w,
        h: widget.defaultLayout.h,
        minW: widget.minW,
        minH: widget.minH,
    }));

const defaultWidgetIds = (): string[] => WIDGET_REGISTRY.map((widget) => widget.id);

function loadStored(key: string): StoredDashboardLayout | null {
    try {
        const raw = localStorage.getItem(key);
        if (!raw) return null;
        const parsed = JSON.parse(raw);
        if (
            !parsed ||
            parsed.version !== STORAGE_VERSION ||
            !Array.isArray(parsed.layout) ||
            !Array.isArray(parsed.widgetIds)
        ) {
            return null;
        }
        return parsed as StoredDashboardLayout;
    } catch {
        return null;
    }
}

function saveStored(key: string, data: StoredDashboardLayout): void {
    try {
        localStorage.setItem(key, JSON.stringify(data));
    } catch {
        // no-op: keep the dashboard functional when localStorage is unavailable
    }
}

export function useDashboardLayout(userId?: number | string) {
    const storageKey = storageKeyFor(userId);
    const [layout, setLayout] = useState<Layout>(() => loadStored(storageKey)?.layout ?? defaultLayout());
    const [widgetIds, setWidgetIds] = useState<string[]>(() => loadStored(storageKey)?.widgetIds ?? defaultWidgetIds());
    const [editMode, setEditMode] = useState(false);
    const saveTimerRef = useRef<number | null>(null);

    const persist = useCallback((nextLayout: Layout, nextWidgetIds: string[]) => {
        if (saveTimerRef.current) window.clearTimeout(saveTimerRef.current);
        saveTimerRef.current = window.setTimeout(() => {
            saveStored(storageKey, { version: STORAGE_VERSION, layout: nextLayout, widgetIds: nextWidgetIds });
        }, 300);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [storageKey]);

    const handleLayoutChange = useCallback((nextLayout: Layout) => {
        setLayout(nextLayout);
        persist(nextLayout, widgetIds);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [persist, widgetIds]);

    const removeWidget = useCallback((id: string) => {
        setWidgetIds((prev) => {
            const next = prev.filter((widgetId) => widgetId !== id);
            persist(layout, next);
            return next;
        });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [layout, persist]);

    const addWidget = useCallback((id: string) => {
        setWidgetIds((prev) => {
            if (prev.includes(id)) return prev;
            const next = [...prev, id];
            const definition = WIDGET_REGISTRY.find((widget) => widget.id === id);
            const nextLayout: Layout = definition
                ? [...layout, {
                    i: id,
                    x: definition.defaultLayout.x,
                    y: definition.defaultLayout.y,
                    w: definition.defaultLayout.w,
                    h: definition.defaultLayout.h,
                    minW: definition.minW,
                    minH: definition.minH,
                }]
                : layout;
            setLayout(nextLayout);
            persist(nextLayout, next);
            return next;
        });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [layout, persist]);

    const resetLayout = useCallback(() => {
        try { localStorage.removeItem(storageKey); } catch { /* no-op */ }
        setLayout(defaultLayout());
        setWidgetIds(defaultWidgetIds());
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [storageKey]);

    const activeWidgets: WidgetDefinition[] = widgetIds
        .map((id) => WIDGET_REGISTRY.find((widget) => widget.id === id))
        .filter((widget): widget is WidgetDefinition => Boolean(widget));

    const hiddenWidgets: WidgetDefinition[] = WIDGET_REGISTRY.filter(
        (widget) => !widgetIds.includes(widget.id)
    );

    return {
        layout,
        activeWidgets,
        hiddenWidgets,
        editMode,
        setEditMode,
        handleLayoutChange,
        removeWidget,
        addWidget,
        resetLayout,
    };
}
