import { Notification, PersistedNotificationState } from './types';

const keyFor = (userId?: number | string): string => `lr_notifications_${userId ?? 'anon'}`;

export function loadPersistedNotifications(userId?: number | string): Record<string, PersistedNotificationState> {
    try {
        const raw = localStorage.getItem(keyFor(userId));
        if (!raw) return {};
        const parsed = JSON.parse(raw);
        return parsed && typeof parsed === 'object' ? parsed : {};
    } catch {
        return {};
    }
}

export function savePersistedNotifications(userId: number | string | undefined, items: Notification[]): void {
    try {
        const persisted: Record<string, PersistedNotificationState> = {};
        items.forEach((n) => {
            if (n.persistDismiss) {
                persisted[n.id] = { read: n.read, dismissed: n.dismissed };
            }
        });
        localStorage.setItem(keyFor(userId), JSON.stringify(persisted));
    } catch {
        // no-op: keep UI functional when localStorage is unavailable
    }
}
