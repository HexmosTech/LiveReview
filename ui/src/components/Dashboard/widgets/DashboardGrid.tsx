import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import classNames from 'classnames';
import { useSearchParams } from 'react-router-dom';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Responsive } from 'react-grid-layout/legacy';
import type { Layout } from 'react-grid-layout/legacy';
import 'react-grid-layout/css/styles.css';
import 'react-resizable/css/styles.css';
import { Button, Icons, Popover, Tooltip } from '../../UIPrimitives';
import { useDashboardLayout } from './useDashboardLayout';
import { WidgetChrome } from './WidgetChrome';
import { ChartSkeleton } from './ChartSkeleton';
import { DashboardPeriodProvider } from './DashboardPeriod';
import { ReviewLayersProvider } from './ReviewLayersData';
import { SystemOverviewProvider } from './SystemOverviewData';
import { PeopleProvider } from './PeopleData';
import { PeriodSelector } from './PeriodSelector';
import { CATEGORY_BADGE_CLASSES, CATEGORY_LABELS, WidgetCategory } from './registry';
import { DASHBOARD_QUERY_KEY, refreshDashboardData } from '../../../api/dashboard';
import './dashboardGrid.css';

interface DashboardGridProps {
    userId?: number | string;
}

// Rendered inside all three providers (not DashboardGrid itself) so it sits alongside the
// widgets it refreshes. The dashboard query itself is a plain cheap GET (see useDashboardQuery
// in api/dashboard.ts) - this button is the explicit, user-triggered path to the expensive
// server-side recompute (POST /api/v1/dashboard/refresh), which everything else deliberately
// avoids calling automatically now that a server-side background job keeps the cache fresh.
const RefreshWidgetsButton: React.FC = () => {
    const queryClient = useQueryClient();
    const { mutate: refresh, isPending } = useMutation({
        mutationFn: refreshDashboardData,
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: DASHBOARD_QUERY_KEY });
        },
    });
    return (
        <Tooltip content={isPending ? 'Refreshing…' : 'Refresh widget data'}>
            <Button
                variant="outline"
                size="sm"
                onClick={() => refresh()}
                disabled={isPending}
                aria-label="Refresh dashboard widgets"
            >
                <span className={isPending ? 'animate-spin' : undefined}><Icons.Refresh /></span>
            </Button>
        </Tooltip>
    );
};

export const DashboardGrid: React.FC<DashboardGridProps> = ({ userId }) => {
    const {
        layout,
        activeWidgets,
        hiddenWidgets,
        editMode,
        setEditMode,
        handleLayoutChange,
        removeWidget,
        addWidget,
        resetLayout,
    } = useDashboardLayout(userId);

    // The first (topmost, leftmost) widget of each category becomes that section's
    // scroll anchor, so the quick-nav pills work regardless of how widgets are
    // interleaved in a user's custom drag/drop layout.
    const anchorByCategory = useMemo(() => {
        const layoutById = new Map(layout.map((item) => [item.i, item]));
        const result: Partial<Record<WidgetCategory, { id: string; y: number; x: number }>> = {};
        activeWidgets.forEach((widget) => {
            const pos = layoutById.get(widget.id);
            if (!pos) return;
            const existing = result[widget.category];
            if (!existing || pos.y < existing.y || (pos.y === existing.y && pos.x < existing.x)) {
                result[widget.category] = { id: widget.id, y: pos.y, x: pos.x };
            }
        });
        return result;
    }, [activeWidgets, layout]);

    const orderedCategories = useMemo(() => {
        return (Object.keys(anchorByCategory) as WidgetCategory[]).sort((a, b) => {
            const anchorA = anchorByCategory[a]!;
            const anchorB = anchorByCategory[b]!;
            return anchorA.y - anchorB.y || anchorA.x - anchorB.x;
        });
    }, [anchorByCategory]);

    // Section deep-linking via a `?section=` query param rather than a URL fragment:
    // this app uses HashRouter, so the fragment is already owned by the router for
    // path routing (e.g. #/dashboard) and can't double as a page anchor.
    const [searchParams, setSearchParams] = useSearchParams();
    const activeSection = searchParams.get('section');

    useEffect(() => {
        if (!activeSection) return;
        const timer = window.setTimeout(() => {
            document.getElementById(`dash-section-${activeSection}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
        }, 400);
        return () => window.clearTimeout(timer);
    }, [activeSection]);

    const goToSection = (category: WidgetCategory) => {
        setSearchParams((prev) => {
            const next = new URLSearchParams(prev);
            next.set('section', category);
            return next;
        });
    };

    // Measured ourselves (not via WidthProvider) so the real width is known before the grid's first paint, not corrected after it.
    const gridWrapperRef = useRef<HTMLDivElement>(null);
    const [gridWidth, setGridWidth] = useState<number | null>(null);
    useLayoutEffect(() => {
        const node = gridWrapperRef.current;
        if (!node) return;
        const measure = () => setGridWidth(node.getBoundingClientRect().width);
        measure();
        const observer = new ResizeObserver(measure);
        observer.observe(node);
        return () => observer.disconnect();
    }, []);

    return (
        <DashboardPeriodProvider>
        <ReviewLayersProvider>
        <SystemOverviewProvider>
        <PeopleProvider>
        <div className="mb-6">
            <div className="sticky top-16 z-30 py-2 mb-3 bg-slate-800/90 backdrop-blur-sm border-b border-slate-700/80 rounded-lg flex flex-wrap items-center justify-between gap-2 px-4">
                <div className="flex flex-wrap items-center gap-3">
                    <PeriodSelector />
                    {orderedCategories.length > 0 && (
                        <div className="flex flex-wrap items-center gap-1.5">
                            {orderedCategories.map((category) => (
                                <button
                                    key={category}
                                    type="button"
                                    onClick={() => goToSection(category)}
                                    className={classNames(
                                        'rounded-full px-2.5 py-1 text-[11px] font-medium uppercase tracking-wide transition-opacity hover:opacity-75',
                                        CATEGORY_BADGE_CLASSES[category]
                                    )}
                                >
                                    {CATEGORY_LABELS[category]}
                                </button>
                            ))}
                        </div>
                    )}
                </div>
                <div className="flex items-center gap-2">
                    {editMode && hiddenWidgets.length > 0 && (
                        <Popover
                            align="right"
                            trigger={
                                <Button variant="outline" size="sm" icon={<Icons.Add />}>
                                    Add Widget
                                </Button>
                            }
                        >
                            <div className="max-h-80 overflow-y-auto -m-4 divide-y divide-slate-700/60">
                                {hiddenWidgets.map((widget) => (
                                    <button
                                        key={widget.id}
                                        type="button"
                                        onClick={() => addWidget(widget.id)}
                                        className="w-full text-left px-4 py-3 hover:bg-slate-700/50 transition-colors"
                                    >
                                        <p className="text-sm font-medium text-slate-100">{widget.title}</p>
                                        <p className="text-xs text-slate-400 mt-0.5">{widget.description}</p>
                                    </button>
                                ))}
                            </div>
                        </Popover>
                    )}
                    {editMode && (
                        <Tooltip content="Clears saved positions and restores every widget to its default spot">
                            <Button variant="ghost" size="sm" onClick={resetLayout}>
                                Reset Layout
                            </Button>
                        </Tooltip>
                    )}
                    <RefreshWidgetsButton />
                    <Button
                        variant={editMode ? 'primary' : 'outline'}
                        size="sm"
                        icon={<Icons.Grid />}
                        onClick={() => setEditMode((prev) => !prev)}
                    >
                        {editMode ? 'Done' : 'Customize Dashboard'}
                    </Button>
                </div>
            </div>

            {/*
                Single fixed breakpoint on purpose: this is a dense analytics dashboard meant
                for desktop use, and react-grid-layout's cross-breakpoint layout regeneration
                (deriving md/sm column counts from the lg layout) was producing broken chart
                renders (e.g. the Sankey collapsing into stacked color blocks) at narrower
                widths. Keeping cols fixed at 12 means widgets only ever get narrower in pixels,
                never reflow into a different column count, so every chart keeps its intended
                relative width and layout.
            */}
            <div ref={gridWrapperRef}>
                {gridWidth !== null && (
                    <Responsive
                        className="layout"
                        width={gridWidth}
                        layouts={{ lg: layout }}
                        breakpoints={{ lg: 0 }}
                        cols={{ lg: 12 }}
                        rowHeight={30}
                        margin={[16, 16]}
                        containerPadding={[0, 0]}
                        isDraggable={editMode}
                        isResizable={editMode}
                        draggableHandle=".widget-drag-handle"
                        onLayoutChange={(currentLayout: Layout) => handleLayoutChange(currentLayout)}
                    >
                        {activeWidgets.map((widget, index) => {
                            const isSectionAnchor = anchorByCategory[widget.category]?.id === widget.id;
                            return (
                                <div
                                    key={widget.id}
                                    id={isSectionAnchor ? `dash-section-${widget.category}` : undefined}
                                    className={isSectionAnchor ? 'scroll-mt-[140px]' : undefined}
                                >
                                    <WidgetChrome
                                        title={widget.title}
                                        category={widget.category}
                                        editMode={editMode}
                                        animationIndex={index}
                                        onRemove={() => removeWidget(widget.id)}
                                    >
                                        {/* Per-widget boundary, not the outer route-level Suspense in App.tsx -
                                            without this, one lazy widget (e.g. an echarts one) suspending would
                                            unmount the entire dashboard back to the route-level fallback instead
                                            of just showing this one widget's own skeleton. */}
                                        <React.Suspense fallback={<ChartSkeleton />}>
                                            <widget.component />
                                        </React.Suspense>
                                    </WidgetChrome>
                                </div>
                            );
                        })}
                    </Responsive>
                )}
            </div>
        </div>
        </PeopleProvider>
        </SystemOverviewProvider>
        </ReviewLayersProvider>
        </DashboardPeriodProvider>
    );
};
