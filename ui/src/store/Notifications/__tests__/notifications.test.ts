import reducer, { add, dismiss, dismissPermanently, hydrate } from '../slice';
import { loadPersistedNotifications, savePersistedNotifications } from '../storage';
import { initialNotificationsState } from '../types';

describe('Notifications Storage & Slice', () => {
    beforeEach(() => {
        localStorage.clear();
    });

    describe('storage.ts: savePersistedNotifications and loadPersistedNotifications', () => {
        it('merges new persisted items without wiping previously persisted items', () => {
            // First session: item-1 is persisted
            savePersistedNotifications(123, [
                {
                    id: 'item-1',
                    severity: 'warning',
                    title: 'Alert 1',
                    message: 'Message 1',
                    source: 'connector',
                    createdAt: Date.now(),
                    read: true,
                    dismissed: true,
                    persistDismiss: true,
                    toast: false,
                },
            ]);

            let loaded = loadPersistedNotifications(123);
            expect(loaded['item-1']).toEqual({ read: true, dismissed: true });

            // Next session: items has only item-2 (or empty) — item-1 should NOT be wiped
            savePersistedNotifications(123, [
                {
                    id: 'item-2',
                    severity: 'error',
                    title: 'Alert 2',
                    message: 'Message 2',
                    source: 'connector',
                    createdAt: Date.now(),
                    read: false,
                    dismissed: true,
                    persistDismiss: true,
                    toast: false,
                },
            ]);

            loaded = loadPersistedNotifications(123);
            expect(loaded['item-1']).toEqual({ read: true, dismissed: true });
            expect(loaded['item-2']).toEqual({ read: false, dismissed: true });
        });

        it('does not wipe persisted items when items list is empty', () => {
            savePersistedNotifications(123, [
                {
                    id: 'item-1',
                    severity: 'warning',
                    title: 'Alert 1',
                    message: 'Message 1',
                    source: 'connector',
                    createdAt: Date.now(),
                    read: true,
                    dismissed: true,
                    persistDismiss: true,
                    toast: false,
                },
            ]);

            // Mount effect with empty items []
            savePersistedNotifications(123, []);

            const loaded = loadPersistedNotifications(123);
            expect(loaded['item-1']).toEqual({ read: true, dismissed: true });
        });

        it('enforces strict user isolation without cross-user leakage or heuristics', () => {
            // Save state for user 100
            savePersistedNotifications(100, [
                {
                    id: 'secret-alert-user-100',
                    severity: 'error',
                    title: 'User 100 Alert',
                    message: 'Private message',
                    source: 'connector',
                    createdAt: Date.now(),
                    read: false,
                    dismissed: true,
                    persistDismiss: true,
                    toast: false,
                },
            ]);

            // User 200 must NOT load User 100's persisted state
            const user200State = loadPersistedNotifications(200);
            expect(user200State['secret-alert-user-100']).toBeUndefined();

            // Anonymous user must NOT load User 100's persisted state
            const anonState = loadPersistedNotifications(undefined);
            expect(anonState['secret-alert-user-100']).toBeUndefined();
        });
    });

    describe('slice.ts: dismiss persistence', () => {
        it('sets persistDismiss to true when dismissed', () => {
            let state = reducer(
                initialNotificationsState,
                add({
                    dedupeKey: 'connector-1',
                    severity: 'warning',
                    message: 'Connector 1 in setup',
                    source: 'connector',
                    toast: false,
                    persistDismiss: false,
                })
            );

            expect(state.items[0].dismissed).toBe(false);
            expect(state.items[0].persistDismiss).toBe(false);

            state = reducer(state, dismiss('connector-1'));

            expect(state.items[0].dismissed).toBe(true);
            expect(state.items[0].persistDismiss).toBe(true);
        });

        it('re-adding an item respects persisted dismissal across reloads', () => {
            // Simulate hydration from localStorage
            let state = reducer(
                initialNotificationsState,
                hydrate({ userId: 123 })
            );
            state = {
                ...state,
                persisted: {
                    'connector-1': { read: true, dismissed: true },
                },
            };

            // Backend returns connector-1 again on dashboard refresh
            state = reducer(
                state,
                add({
                    dedupeKey: 'connector-1',
                    severity: 'warning',
                    message: 'Connector 1 in setup',
                    source: 'connector',
                    toast: false,
                    persistDismiss: true,
                })
            );

            expect(state.items[0].id).toBe('connector-1');
            expect(state.items[0].dismissed).toBe(true);
            expect(state.items[0].persistDismiss).toBe(true);
        });

        it('dismiss writes directly to localStorage and persists', () => {
            let state = reducer(
                initialNotificationsState,
                add({
                    dedupeKey: 'connector-auth-4',
                    severity: 'error',
                    message: 'Credential expired',
                    source: 'connector',
                    toast: false,
                    persistDismiss: true,
                })
            );

            state = reducer(state, dismiss('connector-auth-4'));

            const stored = loadPersistedNotifications();
            expect(stored['connector-auth-4']?.dismissed).toBe(true);
        });

        it('hydrate updates items that were added before user auth resolved', () => {
            // Setup persisted state in localStorage
            localStorage.setItem(
                'lr_notifications_42',
                JSON.stringify({ 'connector-auth-4': { read: true, dismissed: true } })
            );

            // Item added before hydrate (e.g. while auth is resolving)
            let state = reducer(
                initialNotificationsState,
                add({
                    dedupeKey: 'connector-auth-4',
                    severity: 'error',
                    message: 'Credential expired',
                    source: 'connector',
                    toast: false,
                    persistDismiss: true,
                })
            );
            expect(state.items[0].dismissed).toBe(false);

            // Auth resolves and dispatches hydrate
            state = reducer(state, hydrate({ userId: 42 }));

            expect(state.items[0].dismissed).toBe(true);
        });
    });
});
