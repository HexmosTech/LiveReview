const prefetchCache = new Map<string, Promise<unknown>>();

const routeImports = new Map<string, () => Promise<unknown>>([
    ['/dashboard', () => import('../components/Dashboard/Dashboard')],
    ['/reviews', () => import('../pages/Reviews/ReviewsRoutes')],
    ['/explore', () => import('../pages/Explore/ExploreRoutes')],
    ['/git', () => import('../pages/GitProviders/GitProviders')],
    ['/ai', () => import('../pages/AIProviders/AIProviders')],
    ['/settings', () => import('../pages/Settings/Settings')],
    ['/reports', () => import('../pages/Reports/TaxonomyReports')],
    ['/chat', () => import('../pages/Chatbot/ChatbotRoutes')],
    ['/subscribe', () => import('../pages/Subscribe/Subscribe')],
    ['/admin/billing-portfolio', () => import('../pages/Admin/BillingPortfolio')],
]);

function normalize(path: string): string {
    const clean = '/' + path.replace(/^\//, '');
    if (clean.startsWith('/admin/billing-portfolio')) {
        return '/admin/billing-portfolio';
    }
    return '/' + (clean.replace(/^\//, '').split('/')[0] || '');
}

export function prefetchRoute(path: string): void {
    const normalized = normalize(path);
    if (!routeImports.has(normalized)) return;
    const importer = routeImports.get(normalized);
    if (!importer) return;
    if (typeof importer !== 'function') return;
    if (prefetchCache.has(normalized)) return;
    const promise = importer().catch(() => {});
    prefetchCache.set(normalized, promise);
}

export function prefetchRoutes(paths: string[]): void {
    for (const p of paths) prefetchRoute(p);
}

// Starts (or reuses) the prefetch for `path` and resolves once that chunk has actually finished
// downloading - not just been kicked off. A fire-and-forget prefetchRoute() only gives the chunk
// a head start; if the caller navigates before it lands, Suspense still flashes its fallback.
// Awaiting this before navigating makes that flash a hard guarantee rather than a race, at the
// cost of the navigation itself waiting up to `timeoutMs` for the head start to pay off. Resolves
// (never rejects) even on a failed/slow fetch, capped by the timeout, so a flaky chunk load can't
// hang navigation forever - the caller just falls back to Suspense's own fallback in that case.
export function awaitRoutePrefetch(path: string, timeoutMs = 4000): Promise<void> {
    const normalized = normalize(path);
    if (!routeImports.has(normalized)) return Promise.resolve();
    prefetchRoute(path);
    const chunkPromise = prefetchCache.get(normalized) ?? Promise.resolve();
    return Promise.race([
        chunkPromise.then((): void => undefined),
        new Promise<void>((resolve) => { setTimeout(resolve, timeoutMs); }),
    ]);
}
