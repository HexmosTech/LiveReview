import { prefetchRoute, prefetchRoutes, awaitRoutePrefetch } from '../utils/routePrefetch';

describe('routePrefetch utility', () => {
    test('safely handles registered routes', () => {
        expect(() => prefetchRoute('/dashboard')).not.toThrow();
        expect(() => prefetchRoute('/reviews/123')).not.toThrow();
        expect(() => prefetchRoute('/admin/billing-portfolio')).not.toThrow();
    });

    test('safely ignores unregistered routes', () => {
        expect(() => prefetchRoute('/unknown-route')).not.toThrow();
        expect(() => prefetchRoute('')).not.toThrow();
    });

    test('safely handles prototype names without dynamic dispatch errors', () => {
        expect(() => prefetchRoute('toString')).not.toThrow();
        expect(() => prefetchRoute('valueOf')).not.toThrow();
        expect(() => prefetchRoute('__proto__')).not.toThrow();
        expect(() => prefetchRoute('constructor')).not.toThrow();
    });

    test('prefetchRoutes batch calls prefetchRoute safely', () => {
        expect(() => prefetchRoutes(['/dashboard', '/explore', '/settings', 'toString'])).not.toThrow();
    });

    test('awaitRoutePrefetch resolves promptly for unknown routes', async () => {
        await expect(awaitRoutePrefetch('/non-existent')).resolves.toBeUndefined();
        await expect(awaitRoutePrefetch('toString')).resolves.toBeUndefined();
    });

    test('awaitRoutePrefetch resolves for known routes', async () => {
        await expect(awaitRoutePrefetch('/settings', 1000)).resolves.toBeUndefined();
    });
});
