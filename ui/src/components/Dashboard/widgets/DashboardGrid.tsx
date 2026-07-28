import React from 'react';
import { Responsive, WidthProvider } from 'react-grid-layout/legacy';
import type { Layout } from 'react-grid-layout/legacy';
import 'react-grid-layout/css/styles.css';
import 'react-resizable/css/styles.css';
import { Button, Icons, Popover, Tooltip } from '../../UIPrimitives';
import { useDashboardLayout } from './useDashboardLayout';
import { WidgetChrome } from './WidgetChrome';
import { DashboardPeriodProvider } from './DashboardPeriod';
import { PeriodSelector } from './PeriodSelector';
import './dashboardGrid.css';

const ResponsiveGridLayout = WidthProvider(Responsive);

interface DashboardGridProps {
    userId?: number | string;
}

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

    return (
        <DashboardPeriodProvider>
        <div className="mb-6">
            <div className="sticky top-16 z-30 -mx-4 px-4 py-2 mb-3 bg-[#0d1017]/90 backdrop-blur-sm border-b border-slate-800/80 flex flex-wrap items-center justify-between gap-2">
                <div className="flex items-center gap-3">
                    <h2 className="text-lg font-semibold text-white">Engineering Dashboard</h2>
                    <PeriodSelector />
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
            <ResponsiveGridLayout
                className="layout"
                layouts={{ lg: layout }}
                breakpoints={{ lg: 0 }}
                cols={{ lg: 12 }}
                rowHeight={30}
                margin={[16, 16]}
                isDraggable={editMode}
                isResizable={editMode}
                draggableHandle=".widget-drag-handle"
                onLayoutChange={(currentLayout: Layout) => handleLayoutChange(currentLayout)}
            >
                {activeWidgets.map((widget, index) => (
                    <div key={widget.id}>
                        <WidgetChrome
                            title={widget.title}
                            category={widget.category}
                            editMode={editMode}
                            animationIndex={index}
                            onRemove={() => removeWidget(widget.id)}
                        >
                            <widget.component />
                        </WidgetChrome>
                    </div>
                ))}
            </ResponsiveGridLayout>
        </div>
        </DashboardPeriodProvider>
    );
};
