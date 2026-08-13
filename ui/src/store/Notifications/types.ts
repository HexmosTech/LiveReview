export type NotificationSeverity = 'info' | 'success' | 'warning' | 'error';

export type NotificationSource =
    | 'quota'
    | 'connector'
    | 'onboarding'
    | 'license'
    | 'demo'
    | 'url-mismatch'
    | 'system'
    | 'toast-migrated';

export interface NotificationAction {
    label: string;
    actionId: string;
}

export interface Notification {
    id: string;
    severity: NotificationSeverity;
    title?: string;
    message: string;
    source: NotificationSource;
    createdAt: number;
    read: boolean;
    dismissed: boolean;
    // true = dismissal survives reload (persisted to localStorage); false = session-only
    persistDismiss: boolean;
    // whether this notification should also surface as a transient toast
    toast: boolean;
    actions?: NotificationAction[];
    // when set, a dismissed notification with this dedupeKey resurfaces once
    // Date.now() passes this timestamp (e.g. license-expiry re-showing daily)
    expiresAt?: number;
    // stable identity for a recurring notification (e.g. `connector-${id}`).
    // Used both to prevent duplicate stacking and as the notification's id.
    dedupeKey?: string;
}

// What add() accepts — id/createdAt/read/dismissed are computed by the slice;
// persistDismiss/toast default to false when omitted.
export type AddNotificationInput = Omit<
    Notification,
    'id' | 'createdAt' | 'read' | 'dismissed' | 'persistDismiss' | 'toast'
> & {
    persistDismiss?: boolean;
    toast?: boolean;
};

// What gets persisted to localStorage per-user — only for persistDismiss items.
export interface PersistedNotificationState {
    read: boolean;
    dismissed: boolean;
}

export interface NotificationsState {
    items: Notification[];
    toastQueue: string[];
    persisted: Record<string, PersistedNotificationState>;
    hydratedUserId?: number | string;
}

export const initialNotificationsState: NotificationsState = {
    items: [],
    toastQueue: [],
    persisted: {},
    hydratedUserId: undefined,
};
