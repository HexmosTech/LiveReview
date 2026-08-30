import { RootState } from '../configureStore';
import { NotificationSource } from './types';

export const selectUnreadCount = (state: RootState): number =>
    state.Notifications.items.filter((n) => !n.dismissed && !n.read).length;

export const selectVisibleList = (state: RootState) =>
    state.Notifications.items
        .filter((n) => !n.dismissed)
        .slice()
        .sort((a, b) => {
            if (a.read !== b.read) return a.read ? 1 : -1;
            return b.createdAt - a.createdAt;
        });

export const selectActiveToastCandidate = (state: RootState): string | null => {
    const id = state.Notifications.toastQueue.find((toastId) => {
        const item = state.Notifications.items.find((n) => n.id === toastId);
        return !!item && !item.dismissed;
    });
    return id ?? null;
};

export const selectBySource = (source: NotificationSource) => (state: RootState) =>
    state.Notifications.items.filter((n) => n.source === source && !n.dismissed);
