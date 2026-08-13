import { createSlice, PayloadAction } from '@reduxjs/toolkit';
import { AddNotificationInput, initialNotificationsState, Notification, NotificationsState } from './types';
import { loadPersistedNotifications } from './storage';

const generateId = (): string => {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
        return crypto.randomUUID();
    }
    return `n_${Date.now()}_${Math.random().toString(36).slice(2)}`;
};

const slice = createSlice({
    name: 'Notifications',
    initialState: initialNotificationsState,
    reducers: {
        hydrate(state, action: PayloadAction<{ userId?: number | string }>) {
            state.persisted = loadPersistedNotifications(action.payload.userId);
            state.hydratedUserId = action.payload.userId;
        },
        add: {
            reducer(state: NotificationsState, action: PayloadAction<Notification>) {
                const n = action.payload;
                const existingIdx = state.items.findIndex((i) => i.id === n.id);

                if (existingIdx >= 0) {
                    const existing = state.items[existingIdx];
                    const resurfaced =
                        existing.dismissed && n.expiresAt !== undefined && Date.now() >= n.expiresAt;
                    state.items[existingIdx] = {
                        ...existing,
                        severity: n.severity,
                        title: n.title,
                        message: n.message,
                        source: n.source,
                        actions: n.actions,
                        expiresAt: n.expiresAt,
                        persistDismiss: n.persistDismiss,
                        toast: n.toast,
                        dedupeKey: n.dedupeKey,
                        read: resurfaced ? false : existing.read,
                        dismissed: resurfaced ? false : existing.dismissed,
                        createdAt: resurfaced ? n.createdAt : existing.createdAt,
                    };
                    if (n.toast && !state.items[existingIdx].dismissed && !state.toastQueue.includes(n.id)) {
                        state.toastQueue.push(n.id);
                    }
                    return;
                }

                const persisted = state.persisted[n.id];
                let read = n.read;
                let dismissed = n.dismissed;
                if (persisted) {
                    const resurfaced =
                        persisted.dismissed && n.expiresAt !== undefined && Date.now() >= n.expiresAt;
                    read = resurfaced ? false : persisted.read;
                    dismissed = resurfaced ? false : persisted.dismissed;
                }

                state.items.unshift({ ...n, read, dismissed });
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
            }
            state.toastQueue = state.toastQueue.filter((id) => id !== action.payload);
        },
        dismissPermanently(state, action: PayloadAction<string>) {
            const item = state.items.find((i) => i.id === action.payload);
            if (item) {
                item.dismissed = true;
                item.read = true;
                item.persistDismiss = true;
            }
            state.toastQueue = state.toastQueue.filter((id) => id !== action.payload);
        },
        toastShown(state, action: PayloadAction<string>) {
            state.toastQueue = state.toastQueue.filter((id) => id !== action.payload);
        },
    },
});

export const { hydrate, add, markRead, markAllRead, dismiss, dismissPermanently, toastShown } = slice.actions;
export default slice.reducer;
