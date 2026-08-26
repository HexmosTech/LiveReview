const prefetchCache = new Map<string, Promise<unknown>>();

const routeImports: Record<string, () => Promise<unknown>> = {
    '/dashboard': () => import('../components/Dashboard/Dashboard'),
    '/reviews': () => import('../pages/Reviews/ReviewsRoutes'),
    '/explore': () => import('../pages/Explore/ExploreRoutes'),
    '/git': () => import('../pages/GitProviders/GitProviders'),
    '/ai': () => import('../pages/AIProviders/AIProviders'),
    '/settings': () => import('../pages/Settings/Settings'),
    '/reports': () => import('../pages/Reports/TaxonomyReports'),
    '/chat': () => import('../pages/Chatbot/ChatbotRoutes'),
    '/subscribe': () => import('../pages/Subscribe/Subscribe'),
    '/admin/billing-portfolio': () => import('../pages/Admin/BillingPortfolio'),
};

export function prefetchRoute(path: string): void {
    const normalized = '/' + (path.replace(/^\//, '').split('/')[0] || '');
    const importer = routeImports[normalized];
    if (!importer) return;
    if (prefetchCache.has(normalized)) return;
    const promise = importer().catch(() => {});
    prefetchCache.set(normalized, promise);
}

export function prefetchRoutes(paths: string[]): void {
    for (const p of paths) prefetchRoute(p);
}
