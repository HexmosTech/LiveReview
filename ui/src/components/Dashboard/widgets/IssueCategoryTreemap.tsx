import React, { useMemo } from 'react';
import ReactECharts from 'echarts-for-react/lib/core';
import { useNavigate } from 'react-router-dom';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS, LR_ECHARTS_CORE } from './echartsTheme';
import { useChartResize } from './useChartResize';
import { useDashboardPeriod } from './DashboardPeriod';
import { useIssueTreemap } from './IssueTreemapData';
import { EmptyState, Icons } from '../../UIPrimitives';
import { ChartSkeleton } from './ChartSkeleton';
import type { IssueTreemapCategory } from '../../../api/dashboard';

// Rich, vibrant palette for the 10 top-level categories — tuned for dark backgrounds.
const TREEMAP_COLORS: Record<string, string> = {
    'Security':                 '#F43F5E',
    'Reliability':              '#06B6D4',
    'Correctness':              '#3B82F6',
    'Performance':              '#F59E0B',
    'Cost':                     '#22C55E',
    'Scalability':              '#A855F7',
    'Maintainability':          '#7C3AED',
    'Architecture':             '#EC4899',
    'Developer Experience':     '#14B8A6',
    'Compliance & Governance':  '#F97316',
};

// Fallback palette for categories not in the map above.
const FALLBACK_COLORS = ['#3B82F6', '#7C3AED', '#22C55E', '#F59E0B', '#F43F5E', '#06B6D4', '#A855F7', '#EC4899', '#14B8A6', '#F97316'];

function colorForCategory(name: string, index: number): string {
    return TREEMAP_COLORS[name] || FALLBACK_COLORS[index % FALLBACK_COLORS.length];
}

// Lighten a hex color by a factor (0–1) for subcategory children.
function lighten(hex: string, factor: number): string {
    const r = parseInt(hex.slice(1, 3), 16);
    const g = parseInt(hex.slice(3, 5), 16);
    const b = parseInt(hex.slice(5, 7), 16);
    const nr = Math.round(r + (255 - r) * factor);
    const ng = Math.round(g + (255 - g) * factor);
    const nb = Math.round(b + (255 - b) * factor);
    return `#${nr.toString(16).padStart(2, '0')}${ng.toString(16).padStart(2, '0')}${nb.toString(16).padStart(2, '0')}`;
}

export const IssueCategoryTreemap: React.FC = () => {
    const { containerRef, chartRef } = useChartResize();
    const { period } = useDashboardPeriod();
    const { issueTreemap, loading } = useIssueTreemap();
    const navigate = useNavigate();

    const periodData = issueTreemap?.[period];
    const categories = periodData?.categories ?? [];

    // Build echarts-compatible tree data with colors.
    const treeData = useMemo(() => categories
        .filter((cat) => cat.value > 0)
        .map((cat, catIndex) => {
            const baseColor = colorForCategory(cat.name, catIndex);
            return {
                name: cat.name,
                value: cat.value,
                itemStyle: {
                    color: baseColor,
                    borderColor: 'rgba(15, 23, 42, 0.7)',
                    borderWidth: 2,
                    gapWidth: 2,
                },
                children: cat.children
                    .filter((child) => child.value > 0)
                    .map((child) => ({
                        name: child.name,
                        value: child.value,
                        itemStyle: {
                            color: lighten(baseColor, 0.25),
                            borderColor: 'rgba(15, 23, 42, 0.5)',
                            borderWidth: 1,
                            gapWidth: 1,
                        },
                    })),
            };
        }),
        // eslint-disable-next-line react-hooks/exhaustive-deps
        [issueTreemap, period]);

    const totalIssues = useMemo(() =>
        treeData.reduce((sum, cat) => sum + cat.value, 0),
        [treeData],
    );

    const option = useMemo(() => ({
        ...ECHARTS_ANIMATION_DEFAULTS,
        tooltip: {
            formatter: (params: { data?: { name?: string; value?: number; children?: unknown[] }; treePathInfo?: Array<{ name: string }> }) => {
                const d = params.data;
                if (!d) return '';
                const path = params.treePathInfo?.map((p) => p.name).filter(Boolean).join(' > ') ?? d.name;
                const pct = totalIssues > 0 ? ((d.value ?? 0) / totalIssues * 100).toFixed(1) : '0';
                if (d.children) {
                    // Category node
                    return `<div style="font-weight:600;margin-bottom:4px">${d.name}</div>` +
                        `<div style="color:#94A3B8;font-size:12px">${d.value} issues (${pct}%)</div>` +
                        `<div style="color:#64748B;font-size:11px;margin-top:2px">Click to zoom into subcategories</div>`;
                }
                // Subcategory leaf
                return `<div style="font-weight:600;margin-bottom:4px">${path}</div>` +
                    `<div style="color:#94A3B8;font-size:12px">${d.value} issues (${pct}%)</div>` +
                    `<div style="color:#64748B;font-size:11px;margin-top:2px">Click to view in Impact Report</div>`;
            },
        },
        series: [{
            type: 'treemap',
            data: treeData,
            width: '100%',
            height: '100%',
            roam: false,
            nodeClick: 'zoomToNode',
            visibleMin: 0.1,
            breadcrumb: {
                show: true,
                bottom: 0,
                left: 10,
                itemStyle: {
                    color: '#1E293B',
                    borderColor: '#334155',
                    textStyle: { color: '#CBD5E1' },
                },
            },
            label: {
                show: true,
                formatter: '{b}',
                color: '#F1F5F9',
                fontSize: 11,
                fontWeight: 500,
                textShadowColor: 'rgba(0,0,0,0.6)',
                textShadowBlur: 3,
                overflow: 'truncate',
            },
            upperLabel: {
                show: true,
                height: 22,
                color: '#F1F5F9',
                fontSize: 12,
                fontWeight: 600,
                borderColor: 'transparent',
                textShadowColor: 'rgba(0,0,0,0.5)',
                textShadowBlur: 2,
            },
            itemStyle: {
                borderColor: 'rgba(15, 23, 42, 0.7)',
                borderWidth: 2,
                gapWidth: 2,
            },
            levels: [
                {},
                // Category level
                {
                    itemStyle: {
                        borderColor: 'rgba(15, 23, 42, 0.8)',
                        borderWidth: 3,
                        gapWidth: 3,
                    },
                    upperLabel: { show: true },
                },
                // Subcategory level
                {
                    itemStyle: {
                        borderColor: 'rgba(15, 23, 42, 0.5)',
                        borderWidth: 1,
                        gapWidth: 1,
                    },
                    label: {
                        show: true,
                        formatter: '{b}\n{c}',
                        fontSize: 10,
                        overflow: 'truncate',
                    },
                },
            ],
            emphasis: {
                label: { show: true, fontSize: 12, fontWeight: 700 },
                upperLabel: { show: true },
            },
        }],
    }), [treeData, totalIssues]);

    const onEvents = useMemo(() => ({
        click: (params: { data?: { name?: string; children?: unknown[] }; treePathInfo?: Array<{ name: string }> }) => {
            const d = params.data;
            if (!d) return;
            // Navigate to Impact Report with category/subcategory filters.
            const path = params.treePathInfo?.map((p) => p.name).filter(Boolean) ?? [];
            if (d.children) {
                // Category click — filter by category
                navigate(`/reports?category=${encodeURIComponent(d.name ?? '')}`);
            } else if (path.length >= 2) {
                // Subcategory click — filter by both
                navigate(`/reports?category=${encodeURIComponent(path[0])}&subcategory=${encodeURIComponent(d.name ?? '')}`);
            }
        },
    }), [navigate]);

    if (loading) {
        return <ChartSkeleton />;
    }

    if (treeData.length === 0) {
        return (
            <EmptyState
                icon={<Icons.EmptyState />}
                title="No issue data yet"
                description="Category and subcategory distribution will appear here once reviews have run for this period."
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
