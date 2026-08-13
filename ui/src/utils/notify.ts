import type { AppStore } from '../store/configureStore';
import { add } from '../store/Notifications/slice';
import { AddNotificationInput } from '../store/Notifications/types';

// Module-local store reference, populated via injectStore() from index.tsx —
// mirrors the pattern in ../api/apiClient.ts so notify.* works from plain
// functions (e.g. utils/authHelpers.ts) that render outside React.
let store: AppStore | undefined;

export const injectStore = (_store: AppStore): void => {
    store = _store;
};

type NotifyOptions = Partial<Omit<AddNotificationInput, 'severity' | 'message'>>;

const dispatchNotification = (input: AddNotificationInput): void => {
    if (!store) {
        console.warn('[LiveReview] notify called before store was injected:', input.message);
        return;
    }
    store.dispatch(add(input));
};

export const notify = {
    success(message: string, opts?: NotifyOptions): void {
        dispatchNotification({ severity: 'success', message, source: 'toast-migrated', toast: true, ...opts });
    },
    error(message: string, opts?: NotifyOptions): void {
        dispatchNotification({ severity: 'error', message, source: 'toast-migrated', toast: true, ...opts });
    },
    warning(message: string, opts?: NotifyOptions): void {
        dispatchNotification({ severity: 'warning', message, source: 'toast-migrated', toast: true, ...opts });
    },
};
