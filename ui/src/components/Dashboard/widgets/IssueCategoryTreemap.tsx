import React, { useMemo } from 'react';
import ReactECharts from 'echarts-for-react/lib/core';
import { useNavigate } from 'react-router-dom';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS, LR_ECHARTS_CORE } from './echartsTheme';
import { useChartResize } from './useChartResize';
import { useDashboardPeriod } from './DashboardPeriod';
import { useIssueTreemap } from './IssueTreemapData';
import { EmptyState, Icons } from '../../UIPrimitives';
import { ChartSkeleton } from './ChartSkeleton';

// Base hues — these get darkened below before hitting the chart.
const CATEGORY_HUES: Record<string, [number, number, number]> = {
    'Security':                [232, 64, 87],
    'Reliability':             [14, 165, 199],
    'Correctness':             [59, 130, 246],
    'Performance':             [232, 155, 11],
    'Cost':                    [32, 189, 90],
    'Scalability':             [147, 51, 234],
    'Maintainability':         [109, 40, 217],
    'Architecture':            [219, 39, 119],
    'Developer Experience':    [13, 148, 136],
    'Compliance & Governance': [234, 88, 12],
};

const FALLBACK_HUES: Array<[number, number, number]> = [
    [59, 130, 246], [109, 40, 217], [32, 189, 90], [232, 155, 11],
    [232, 64, 87], [14, 165, 199], [147, 51, 234], [219, 39, 119],
    [13, 148, 136], [234, 88, 12],
];

function hueForCategory(name: string, index: number): [number, number, number] {
    return CATEGORY_HUES[name] || FALLBACK_HUES[index % FALLBACK_HUES.length];
}

// Darken an RGB triple by a factor (0 = black, 1 = unchanged).
function darken([r, g, b]: [number, number, number], factor: number): string {
    const nr = Math.round(r * factor);
    const ng = Math.round(g * factor);
    const nb = Math.round(b * factor);
    return `rgb(${nr},${ng},${nb})`;
}

// Build a subtle diagonal gradient: darker top-left → slightly lighter bottom-right.
// The shift is small so it reads as a flat dark tile with just a hint of depth.
function gradient(hue: [number, number, number], darkFactor: number, lightBoost: number) {
    return {
        type: 'linear' as const,
        x: 0, y: 0, x2: 1, y2: 1,
        colorStops: [
            { offset: 0, color: darken(hue, darkFactor) },
            { offset: 1, color: darken(hue, darkFactor + lightBoost) },
        ],
    };
}

// Format percentage with dynamic precision.
function formatPct(value: number, total: number): string {
    if (total <= 0) return '0';
    const pct = (value / total) * 100;
    if (pct === 0) return '0';
    if (pct < 0.1) return pct.toFixed(2);
    if (pct < 1) return pct.toFixed(1);
    return Math.round(pct).toString();
}

export const IssueCategoryTreemap: React.FC = () => {
    const { containerRef, chartRef } = useChartResize();
    const { period } = useDashboardPeriod();
    const { issueTreemap, loading } = useIssueTreemap();
    const navigate = useNavigate();

    const periodData = issueTreemap?.[period];
    const categories = periodData?.categories ?? [];

    // Build echarts-compatible tree data with dark gradient fills.
    const treeData = useMemo(() => categories
        .filter((cat) => cat.value > 0)
        .map((cat, catIndex) => {
            const hue = hueForCategory(cat.name, catIndex);
            return {
                name: cat.name,
                value: cat.value,
                itemStyle: {
                    color: gradient(hue, 0.5, 0.15),
                    borderColor: 'rgba(15, 23, 42, 0.6)',
                    borderWidth: 2,
                    gapWidth: 5,
                    borderRadius: 6,
                },
                children: cat.children
                    .filter((child) => child.value > 0)
                    .map((child) => ({
                        name: child.name,
                        value: child.value,
                        itemStyle: {
                            color: gradient(hue, 0.58, 0.18),
                            borderColor: 'rgba(15, 23, 42, 0.4)',
                            borderWidth: 1,
                            gapWidth: 2,
                            borderRadius: 4,
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
                const depth = params.treePathInfo?.length ?? 0;
                const pct = formatPct(d.value ?? 0, totalIssues);
                // depth==1 means we're hovering the root container itself
                if (depth <= 1) {
                    return `<div style="font-weight:600;margin-bottom:4px">All Issues</div>` +
                        `<div style="color:#94A3B8;font-size:12px">${totalIssues.toLocaleString()} total issues</div>`;
                }
                if (d.children) {
                    // Category node (depth==2: root → category)
                    return `<div style="font-weight:600;margin-bottom:4px">${d.name}</div>` +
                        `<div style="color:#94A3B8;font-size:12px">${(d.value ?? 0).toLocaleString()} issues (${pct}%)</div>` +
                        `<div style="color:#64748B;font-size:11px;margin-top:4px">Click to zoom into subcategories</div>`;
                }
                // Subcategory leaf (depth==3: root → category → subcategory)
                const catName = params.treePathInfo?.[1]?.name ?? '';
                return `<div style="font-weight:600;margin-bottom:2px">${d.name}</div>` +
                    (catName ? `<div style="color:#64748B;font-size:11px;margin-bottom:4px">${catName}</div>` : '') +
                    `<div style="color:#94A3B8;font-size:12px">${(d.value ?? 0).toLocaleString()} issues (${pct}%)</div>` +
                    `<div style="color:#64748B;font-size:11px;margin-top:4px">Click to view in Impact Report</div>`;
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
            squareRatio: 0.5 * (1 + Math.sqrt(5)),
            breadcrumb: {
                show: true,
                bottom: 4,
                left: 8,
                height: 22,
                itemStyle: {
                    color: '#1E293B',
                    borderColor: '#334155',
                    borderWidth: 1,
                    borderRadius: 4,
                    textStyle: { color: '#CBD5E1', fontSize: 12 },
                },
            },
            label: {
                show: true,
                formatter: '{b}',
                color: '#E2E8F0',
                fontSize: 11,
                fontWeight: 500,
                textShadowColor: 'rgba(0,0,0,0.8)',
                textShadowBlur: 4,
                overflow: 'truncate',
            },
            upperLabel: {
                show: true,
                height: 24,
                color: '#F1F5F9',
                fontSize: 12,
                fontWeight: 600,
                borderColor: 'transparent',
                textShadowColor: 'rgba(0,0,0,0.7)',
                textShadowBlur: 3,
            },
            itemStyle: {
                borderColor: 'rgba(15, 23, 42, 0.6)',
                borderWidth: 2,
                gapWidth: 5,
                borderRadius: 6,
            },
            levels: [
                {},
                // Category level — parent tiles
                {
                    itemStyle: {
                        borderColor: 'rgba(15, 23, 42, 0.7)',
                        borderWidth: 3,
                        gapWidth: 6,
                        borderRadius: 8,
                    },
                    upperLabel: { show: true },
                },
                // Subcategory level — child tiles
                {
                    itemStyle: {
                        borderColor: 'rgba(15, 23, 42, 0.4)',
                        borderWidth: 1,
                        gapWidth: 2,
                        borderRadius: 4,
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
                label: { show: true, fontSize: 13, fontWeight: 700 },
                upperLabel: { show: true },
                itemStyle: {
                    borderColor: 'rgba(255,255,255,0.2)',
                    borderWidth: 2,
                    shadowBlur: 12,
                    shadowColor: 'rgba(0,0,0,0.3)',
                },
            },
        }],
    }), [treeData, totalIssues]);

    const onEvents = useMemo(() => ({
        click: (params: { data?: { name?: string; children?: unknown[] }; treePathInfo?: Array<{ name: string }> }) => {
            const d = params.data;
            if (!d) return;
            const path = params.treePathInfo?.map((p) => p.name).filter(Boolean) ?? [];
            if (d.children) {
                navigate(`/reports?category=${encodeURIComponent(d.name ?? '')}`);
            } else if (path.length >= 2) {
                navigate(`/reports?category=${encodeURIComponent(path[1])}&subcategory=${encodeURIComponent(d.name ?? '')}`);
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
