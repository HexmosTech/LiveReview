import { defineNotification, matchesRoutePattern, injectNotificationStore } from '../registry';
import { ConnectorAuthAlert, ConnectorSetupProgressAlert } from '../catalog';
import configureAppStore from '../../configureStore';

describe('Notifications Registry & Catalog Module', () => {
    describe('matchesRoutePattern', () => {
        it('matches exact and prefix routes', () => {
            const patterns = ['/dashboard', '/reviews', '/explore/repositories'];

            expect(matchesRoutePattern('/dashboard', patterns)).toBe(true);
            expect(matchesRoutePattern('/', patterns)).toBe(true); // /dashboard matches root
            expect(matchesRoutePattern('/reviews', patterns)).toBe(true);
            expect(matchesRoutePattern('/reviews/pull-requests', patterns)).toBe(true);
            expect(matchesRoutePattern('/explore/repositories', patterns)).toBe(true);

            expect(matchesRoutePattern('/settings', patterns)).toBe(false);
            expect(matchesRoutePattern('/explore/issues', patterns)).toBe(false);
        });

        it('supports regex and function matchers', () => {
            const patterns = [
                /\/admin\/.*/,
                (path: string) => path.endsWith('/edit'),
            ];

            expect(matchesRoutePattern('/admin/users', patterns)).toBe(true);
            expect(matchesRoutePattern('/posts/1/edit', patterns)).toBe(true);
            expect(matchesRoutePattern('/posts/1', patterns)).toBe(false);
        });
    });

    describe('defineNotification', () => {
        const TestAlert = defineNotification<{ itemId: number; itemName: string }>({
            source: 'system',
            severity: 'warning',
            id: ({ itemId }) => `test-item-${itemId}`,
            title: 'Test Warning',
            message: ({ itemName }) => `Item ${itemName} requires attention.`,
            bannerRoutes: ['/dashboard', '/test'],
            persistDismiss: true,
            actions: ({ itemId }) => [{ label: 'Fix', actionId: `/fix/${itemId}` }],
        });

        it('generates the correct action payload with defaults', () => {
            const action = TestAlert.create({ itemId: 42, itemName: 'Database' });

            expect(action.type).toBe('Notifications/add');
            expect(action.payload.dedupeKey).toBe('test-item-42');
            expect(action.payload.id).toBe('test-item-42');
            expect(action.payload.title).toBe('Test Warning');
            expect(action.payload.message).toBe('Item Database requires attention.');
            expect(action.payload.severity).toBe('warning');
            expect(action.payload.source).toBe('system');
            expect(action.payload.persistDismiss).toBe(true);
            expect(action.payload.actions).toEqual([{ label: 'Fix', actionId: '/fix/42' }]);
        });

        it('dispatches to Redux store via publish() and dismiss()', () => {
            const store = configureAppStore({
                Notifications: {
                    items: [],
                    toastQueue: [],
                    persisted: {},
                    hydratedUserId: undefined,
                },
            });
            injectNotificationStore(store);

            TestAlert.publish({ itemId: 99, itemName: 'Server' });
            let state = store.getState().Notifications;
            expect(state.items).toHaveLength(1);
            expect(state.items[0].id).toBe('test-item-99');
            expect(state.items[0].dismissed).toBe(false);

            TestAlert.dismiss({ itemId: 99, itemName: 'Server' });
            state = store.getState().Notifications;
            expect(state.items[0].dismissed).toBe(true);
            expect(state.persisted['test-item-99']?.dismissed).toBe(true);

            // Re-publishing respects dismissal
            TestAlert.publish({ itemId: 99, itemName: 'Server' });
            state = store.getState().Notifications;
            expect(state.items[0].dismissed).toBe(true);
        });

        it('correctly matches routes via matchesRoute()', () => {
            expect(TestAlert.matchesRoute('/dashboard')).toBe(true);
            expect(TestAlert.matchesRoute('/test')).toBe(true);
            expect(TestAlert.matchesRoute('/other')).toBe(false);
        });
    });

    describe('Predefined Catalog Notifications', () => {
        it('ConnectorAuthAlert matches intended routes', () => {
            expect(ConnectorAuthAlert.matchesRoute('/dashboard')).toBe(true);
            expect(ConnectorAuthAlert.matchesRoute('/')).toBe(true);
            expect(ConnectorAuthAlert.matchesRoute('/reviews')).toBe(true);
            expect(ConnectorAuthAlert.matchesRoute('/reviews/123')).toBe(true);
            expect(ConnectorAuthAlert.matchesRoute('/explore/repositories')).toBe(true);
            expect(ConnectorAuthAlert.matchesRoute('/git')).toBe(true);
            expect(ConnectorAuthAlert.matchesRoute('/settings')).toBe(false);
        });

        it('ConnectorSetupProgressAlert only matches dashboard', () => {
            expect(ConnectorSetupProgressAlert.matchesRoute('/dashboard')).toBe(true);
            expect(ConnectorSetupProgressAlert.matchesRoute('/')).toBe(true);
            expect(ConnectorSetupProgressAlert.matchesRoute('/reviews')).toBe(false);
            expect(ConnectorSetupProgressAlert.matchesRoute('/git')).toBe(false);
        });
    });
});
