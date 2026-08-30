import React from 'react';
import { LayerStageCards } from './LayerStageCards';
import { SystemKpiRow } from './SystemKpiRow';
import { ConnectedProviders } from './ConnectedProviders';
import { AverageReviewsStat } from './AverageReviewsStat';
import { TopReviewersLeaderboard } from './TopReviewersLeaderboard';

// The 7 echarts-based widgets are lazy-loaded (not statically imported like the ones above):
// registry.ts is itself statically imported by DashboardGrid.tsx, which Dashboard.tsx statically
// imports - so without this, echarts (600KB+) had to finish downloading before ANY widget (or
// Dashboard's own data-fetching, which lives in the same React.lazy() boundary) could even start,
// even though Dashboard's data queries have zero dependency on echarts. Each of these now
// resolves independently once its own chunk arrives, behind the per-widget Suspense boundary in
// DashboardGrid.tsx - see docs/perf-improvement.md.
const ReviewPipelineSankey = React.lazy(() => import('./ReviewPipelineSankey').then((m) => ({ default: m.ReviewPipelineSankey })));
const IssueCategoryTreemap = React.lazy(() => import('./IssueCategoryTreemap').then((m) => ({ default: m.IssueCategoryTreemap })));
const IssueCategoryRadar = React.lazy(() => import('./IssueCategoryRadar').then((m) => ({ default: m.IssueCategoryRadar })));
const ReviewVolumeBar = React.lazy(() => import('./ReviewVolumeBar').then((m) => ({ default: m.ReviewVolumeBar })));
const RepoHierarchySunburst = React.lazy(() => import('./RepoHierarchySunburst').then((m) => ({ default: m.RepoHierarchySunburst })));
const CoverageGauge = React.lazy(() => import('./CoverageGauge').then((m) => ({ default: m.CoverageGauge })));
const UsageShareDonut = React.lazy(() => import('./UsageShareDonut').then((m) => ({ default: m.UsageShareDonut })));
const ContributionCalendarHeatmap = React.lazy(() => import('./ContributionCalendarHeatmap').then((m) => ({ default: m.ContributionCalendarHeatmap })));

export type WidgetCategory = 'layers' | 'system' | 'people';

export interface GridPosition {
    x: number;
    y: number;
    w: number;
    h: number;
}

export interface WidgetDefinition {
    id: string;
    title: string;
    category: WidgetCategory;
    description: string;
    defaultLayout: GridPosition;
    minW?: number;
    minH?: number;
    component: React.ComponentType;
}

export const CATEGORY_LABELS: Record<WidgetCategory, string> = {
    layers: 'Review Layers',
    system: 'System Overview',
    people: 'People',
};

export const CATEGORY_BADGE_CLASSES: Record<WidgetCategory, string> = {
    layers: 'bg-blue-500/15 text-blue-300 border border-blue-500/30',
    system: 'bg-purple-500/15 text-purple-300 border border-purple-500/30',
    people: 'bg-green-500/15 text-green-300 border border-green-500/30',
};

export const WIDGET_REGISTRY: WidgetDefinition[] = [
    // --- Group A: Layered view of reviews ---
    {
        id: 'review-pipeline-sankey',
        title: 'Review Pipeline',
        category: 'layers',
        description: 'How reviews from each trigger source break down by issue category found.',
        defaultLayout: { x: 0, y: 0, w: 12, h: 10 },
        minW: 6, minH: 7,
        component: ReviewPipelineSankey,
    },
    {
        id: 'issue-category-treemap',
        title: 'Issue Distribution',
        category: 'layers',
        description: 'Hierarchical treemap of issue categories and subcategories, sized by count.',
        defaultLayout: { x: 0, y: 10, w: 12, h: 24 },
        minW: 6, minH: 12,
        component: IssueCategoryTreemap,
    },
    {
        id: 'issue-category-radar',
        title: 'Issue Mix by Stage',
        category: 'layers',
        description: 'Compares the kinds of issues each review stage tends to catch.',
        defaultLayout: { x: 0, y: 34, w: 12, h: 10 },
        minW: 6, minH: 7,
        component: IssueCategoryRadar,
    },
    {
        id: 'layer-stage-cards',
        title: 'Reviews by Stage',
        category: 'layers',
        description: 'Volume and issue counts for each review trigger source.',
        defaultLayout: { x: 0, y: 44, w: 12, h: 5 },
        minW: 6, minH: 5,
        component: LayerStageCards,
    },
    {
        id: 'review-volume-bar',
        title: 'Stage Volume Comparison',
        category: 'layers',
        description: 'Quick side-by-side read of review volume per stage.',
        defaultLayout: { x: 0, y: 48, w: 12, h: 7 },
        minW: 5, minH: 5,
        component: ReviewVolumeBar,
    },

    // --- Group B: System overview ---
    {
        id: 'system-kpi-row',
        title: 'System at a Glance',
        category: 'system',
        description: 'Git hosts, AI connectors, repos, and PRs tracked by LiveReview.',
        defaultLayout: { x: 0, y: 55, w: 12, h: 4 },
        minW: 6, minH: 3,
        component: SystemKpiRow,
    },
    {
        id: 'repo-hierarchy-sunburst',
        title: 'PR Count by Repo / Host',
        category: 'system',
        description: 'Git host → repository → PR volume, sized by activity.',
        defaultLayout: { x: 0, y: 59, w: 7, h: 10 },
        minW: 5, minH: 7,
        component: RepoHierarchySunburst,
    },
    {
        id: 'connected-providers',
        title: 'Connected Providers',
        category: 'system',
        description: 'Every git host and AI provider currently connected.',
        defaultLayout: { x: 7, y: 59, w: 5, h: 10 },
        minW: 4, minH: 6,
        component: ConnectedProviders,
    },
    {
        id: 'coverage-gauge',
        title: 'Review Coverage',
        category: 'system',
        description: 'Share of PRs/MRs that received at least one AI review in the selected period.',
        defaultLayout: { x: 0, y: 69, w: 4, h: 7 },
        minW: 3, minH: 5,
        component: CoverageGauge,
    },
    {
        id: 'avg-reviews-stat',
        title: 'Review Averages',
        category: 'system',
        description: 'Average reviews per PR/MR and per commit.',
        defaultLayout: { x: 4, y: 69, w: 8, h: 7 },
        minW: 5, minH: 5,
        component: AverageReviewsStat,
    },

    // --- Group C: People perspective ---
    {
        id: 'top-reviewers-leaderboard',
        title: 'Top Reviewers',
        category: 'people',
        description: 'Ranked by number of reviews given this period.',
        defaultLayout: { x: 0, y: 76, w: 6, h: 9 },
        minW: 4, minH: 6,
        component: TopReviewersLeaderboard,
    },
    {
        id: 'usage-share-donut',
        title: 'Usage Share',
        category: 'people',
        description: 'Share of review volume by contributor (top 5 + others).',
        defaultLayout: { x: 6, y: 76, w: 6, h: 9 },
        minW: 4, minH: 6,
        component: UsageShareDonut,
    },
    {
        id: 'contribution-calendar-heatmap',
        title: 'Contribution Activity',
        category: 'people',
        description: 'Daily review activity over the last 7 months.',
        defaultLayout: { x: 0, y: 85, w: 12, h: 6 },
        minW: 8, minH: 4,
        component: ContributionCalendarHeatmap,
    },
];
