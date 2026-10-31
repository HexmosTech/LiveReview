import { AddNotificationInput, NotificationAction, NotificationSeverity, NotificationSource } from './types';
import { add, dismissPermanently } from './slice';
import type { AppStore, RootState } from '../configureStore';
import { useAppSelector } from '../configureStore';

let globalStore: AppStore | undefined;

/**
 * Injects the central Redux store so notifications can be published or
 * dismissed from outside React (e.g. background polling, API client, utils).
 */
export const injectNotificationStore = (store: AppStore): void => {
    globalStore = store;
};

export type RoutePattern = string | RegExp | ((pathname: string) => boolean);

export interface NotificationConfig<P = void> {
    /** Category identifier (e.g. 'connector', 'license', 'system', 'quota', etc.) */
    source: NotificationSource;
    /** Severity level */
    severity: NotificationSeverity;
    /** Unique dedupe key generator. Returns a stable string ID for this notification instance */
    id: (params: P) => string;
    /** Title text or generator function */
    title?: string | ((params: P) => string | undefined);
    /** Body message or generator function */
    message: string | ((params: P) => string);
    /** Whether to also pop up as a transient screen toast (default: false) */
    toast?: boolean;
    /** Whether user dismissal persists in localStorage across page reloads (default: true) */
    persistDismiss?: boolean;
    /** Action buttons with route links or custom action IDs */
    actions?: NotificationAction[] | ((params: P) => NotificationAction[]);
    /** Routes where this notification renders as a prominent top banner (e.g. ['/dashboard', '/reviews']) */
    bannerRoutes?: RoutePattern[];
    /** Future timestamp when a dismissed notification resurfaces (e.g. once per day) */
    expiresAt?: (params: P) => number;
}

export function matchesRoutePattern(pathname: string, patterns?: RoutePattern[]): boolean {
    if (!patterns || patterns.length === 0) return false;
    return patterns.some((p) => {
        if (typeof p === 'function') return p(pathname);
        if (p instanceof RegExp) return p.test(pathname);
        if (p === '/') return pathname === '/';
        // Prefix matching for routes: '/dashboard' matches '/dashboard' or '/dashboard/...'
        return pathname === p || pathname.startsWith(`${p}/`) || (p === '/dashboard' && pathname === '/');
    });
}

export interface NotificationModule<P = void> {
    config: NotificationConfig<P>;
    /** Computes the dedupe key / ID for given parameters */
    getId: (params: P) => string;
    /** Creates the raw Redux action payload to dispatch */
    create: (params: P) => ReturnType<typeof add>;
    /** Publishes this notification to the Redux store */
    publish: (params: P) => void;
    /** Dismisses this notification permanently across reloads */
    dismiss: (paramsOrId: P | string) => void;
    /** React hook: reactively returns true if this notification is currently dismissed */
    useIsDismissed: (paramsOrId: P | string) => boolean;
    /** Checks whether this notification's banner is active on the given pathname */
    matchesRoute: (pathname: string) => boolean;
}

/**
 * Creates a strongly-typed, reusable notification module.
 *
 * Example:
 * ```ts
 * export const PasswordExpiringAlert = defineNotification<{ daysLeft: number }>({
 *     source: 'system',
 *     severity: 'warning',
 *     id: () => 'password-expiry-alert',
 *     title: 'Password Expiring Soon',
 *     message: ({ daysLeft }) => `Your password expires in ${daysLeft} days. Please update it.`,
 *     bannerRoutes: ['/dashboard', '/settings'],
 *     actions: [{ label: 'Update Password', actionId: '/settings#security' }],
 * });
 *
 * // In any component or service:
 * PasswordExpiringAlert.publish({ daysLeft: 3 });
 * const isDismissed = PasswordExpiringAlert.useIsDismissed();
 * PasswordExpiringAlert.dismiss();
 * ```
 */
export function defineNotification<P = void>(config: NotificationConfig<P>): NotificationModule<P> {
    const resolveId = (params: P): string => {
        return config.id(params);
    };

    const resolveTitle = (params: P): string | undefined => {
        return typeof config.title === 'function' ? config.title(params) : config.title;
    };

    const resolveMessage = (params: P): string => {
        return typeof config.message === 'function' ? config.message(params) : config.message;
    };

    const resolveActions = (params: P): NotificationAction[] | undefined => {
        return typeof config.actions === 'function' ? config.actions(params) : config.actions;
    };

    const create = (params: P): ReturnType<typeof add> => {
        const id = resolveId(params);
        return add({
            dedupeKey: id,
            severity: config.severity,
            title: resolveTitle(params),
            message: resolveMessage(params),
            source: config.source,
            toast: config.toast ?? false,
            persistDismiss: config.persistDismiss ?? true,
            actions: resolveActions(params),
            expiresAt: config.expiresAt ? config.expiresAt(params) : undefined,
        });
    };

    const publish = (params: P): void => {
        if (!globalStore) {
            console.warn('[Notification] publish called before store was injected:', resolveId(params));
            return;
        }
        globalStore.dispatch(create(params));
    };

    const resolveTargetId = (target: P | string): string => {
        if (typeof target === 'string') return target;
        return resolveId(target);
    };

    const dismiss = (paramsOrId: P | string): void => {
        const id = resolveTargetId(paramsOrId);
        if (globalStore) {
            globalStore.dispatch(dismissPermanently(id));
        }
    };

    const useIsDismissed = (paramsOrId: P | string): boolean => {
        const id = resolveTargetId(paramsOrId);
        const item = useAppSelector((state: RootState) => state.Notifications.items.find((i) => i.id === id));
        const persisted = useAppSelector((state: RootState) => state.Notifications.persisted[id]);
        if (item?.dismissed) return true;
        return !!persisted?.dismissed;
    };

    const matchesRoute = (pathname: string): boolean => {
        return matchesRoutePattern(pathname, config.bannerRoutes);
    };

    return {
        config,
        getId: resolveId,
        create,
        publish,
        dismiss,
        useIsDismissed,
        matchesRoute,
    };
}
