import { createSlice, PayloadAction } from '@reduxjs/toolkit';
import { AddNotificationInput, initialNotificationsState, Notification, NotificationsState } from './types';
import { loadPersistedNotifications, savePersistedMap } from './storage';

const generateId = (): string => {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
        return crypto.randomUUID();
    }
    return `n_${Date.now()}_${Math.random().toString(36).slice(2)}`;
};

const getInitialState = (): NotificationsState => ({
    ...initialNotificationsState,
    persisted: loadPersistedNotifications(),
});

const slice = createSlice({
    name: 'Notifications',
    initialState: getInitialState,
    reducers: {
        hydrate(state, action: PayloadAction<{ userId?: number | string }>) {
            state.persisted = loadPersistedNotifications(action.payload.userId);
            state.hydratedUserId = action.payload.userId;
            state.items.forEach((item) => {
                const p = state.persisted[item.id];
                if (p && p.dismissed) {
                    const resurfaced = item.expiresAt !== undefined && Date.now() >= item.expiresAt;
                    if (!resurfaced) {
                        item.dismissed = true;
                        item.read = p.read;
                        item.persistDismiss = true;
                    }
                }
            });
        },
        add: {
            reducer(state: NotificationsState, action: PayloadAction<Notification>) {
                const n = action.payload;
                const existingIdx = state.items.findIndex((i) => i.id === n.id);

                if (existingIdx >= 0) {
                    const existing = state.items[existingIdx];
                    const persisted = state.persisted[n.id];
                    const wasDismissed = Boolean(existing.dismissed || persisted?.dismissed);
                    const resurfaced =
                        wasDismissed && n.expiresAt !== undefined && Date.now() >= n.expiresAt;
                    const nextDismissed = resurfaced ? false : wasDismissed;
                    const nextRead = resurfaced ? false : Boolean(existing.read || persisted?.read);

                    state.items[existingIdx] = {
                        ...existing,
                        severity: n.severity,
                        title: n.title,
                        message: n.message,
                        source: n.source,
                        actions: n.actions,
                        expiresAt: n.expiresAt,
                        persistDismiss: existing.persistDismiss || n.persistDismiss || wasDismissed,
                        toast: n.toast,
                        dedupeKey: n.dedupeKey,
                        read: nextRead,
                        dismissed: nextDismissed,
                        createdAt: resurfaced ? n.createdAt : existing.createdAt,
                    };
                    if (n.toast && !nextDismissed && !state.toastQueue.includes(n.id)) {
                        state.toastQueue.push(n.id);
                    }
                    return;
                }

                const persisted = state.persisted[n.id];
                let read = n.read;
                let dismissed = n.dismissed;
                let persistDismissFlag = n.persistDismiss;
                if (persisted) {
                    const resurfaced =
                        persisted.dismissed && n.expiresAt !== undefined && Date.now() >= n.expiresAt;
                    read = resurfaced ? false : persisted.read;
                    dismissed = resurfaced ? false : persisted.dismissed;
                    // restore persistDismiss too, or the next save erases this record
                    if (!resurfaced) persistDismissFlag = true;
                }

                state.items.unshift({ ...n, read, dismissed, persistDismiss: persistDismissFlag });
                if (n.toast && !dismissed) {
                    state.toastQueue.push(n.id);
                }
            },
            prepare(input: AddNotificationInput) {
                const id = input.dedupeKey ?? generateId();
                const notification: Notification = {
                    id,
                    severity: input.severity,
                    title: input.title,
                    message: input.message,
                    source: input.source,
                    createdAt: Date.now(),
                    read: false,
                    dismissed: false,
                    persistDismiss: input.persistDismiss ?? false,
                    toast: input.toast ?? false,
                    actions: input.actions,
                    expiresAt: input.expiresAt,
                    dedupeKey: input.dedupeKey,
                };
                return { payload: notification };
            },
        },
        markRead(state, action: PayloadAction<string>) {
            const item = state.items.find((i) => i.id === action.payload);
            if (item) item.read = true;
        },
        markAllRead(state) {
            state.items.forEach((i) => {
                if (!i.dismissed) i.read = true;
            });
        },
        dismiss(state, action: PayloadAction<string>) {
            const item = state.items.find((i) => i.id === action.payload);
            if (item) {
                item.dismissed = true;
                item.read = true;
                item.persistDismiss = true;
            }
            state.persisted[action.payload] = { read: true, dismissed: true };
            savePersistedMap(state.hydratedUserId, state.persisted);
            state.toastQueue = state.toastQueue.filter((id) => id !== action.payload);
        },
        dismissPermanently(state, action: PayloadAction<string>) {
            const item = state.items.find((i) => i.id === action.payload);
            if (item) {
                item.dismissed = true;
                item.read = true;
                item.persistDismiss = true;
            }
            state.persisted[action.payload] = { read: true, dismissed: true };
            savePersistedMap(state.hydratedUserId, state.persisted);
            state.toastQueue = state.toastQueue.filter((id) => id !== action.payload);
        },
        toastShown(state, action: PayloadAction<string>) {
            state.toastQueue = state.toastQueue.filter((id) => id !== action.payload);
        },
    },
});

export const { hydrate, add, markRead, markAllRead, dismiss, dismissPermanently, toastShown } = slice.actions;
export default slice.reducer;
