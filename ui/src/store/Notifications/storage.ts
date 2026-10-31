import { Notification, PersistedNotificationState } from './types';

export const keyFor = (userId?: number | string): string => {
    if (userId !== undefined && userId !== null && String(userId).trim() !== '') {
        return `lr_notifications_${userId}`;
    }
    return 'lr_notifications_anon';
};

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

export function savePersistedMap(userId: number | string | undefined, map: Record<string, PersistedNotificationState>): void {
    try {
        const effectiveKey = keyFor(userId);
        const existing = loadPersistedNotifications(userId);
        const merged: Record<string, PersistedNotificationState> = { ...existing, ...map };
        localStorage.setItem(effectiveKey, JSON.stringify(merged));
    } catch {
        // no-op: keep UI functional when localStorage is unavailable
    }
}

export function savePersistedNotifications(userId: number | string | undefined, items: Notification[]): void {
    try {
        const effectiveKey = keyFor(userId);
        const existing = loadPersistedNotifications(userId);
        const persisted: Record<string, PersistedNotificationState> = { ...existing };
        items.forEach((n) => {
            const resurfaced = n.expiresAt !== undefined && Date.now() >= n.expiresAt;
            if (n.dismissed && !resurfaced) {
                persisted[n.id] = { read: n.read, dismissed: true };
            } else if (resurfaced) {
                persisted[n.id] = { read: false, dismissed: false };
            } else if (n.persistDismiss && !persisted[n.id]?.dismissed) {
                // Prevent an active notification from overwriting a previously dismissed state
                persisted[n.id] = { read: n.read, dismissed: false };
            }
        });
        localStorage.setItem(effectiveKey, JSON.stringify(persisted));
    } catch {
        // no-op: keep UI functional when localStorage is unavailable
    }
}
