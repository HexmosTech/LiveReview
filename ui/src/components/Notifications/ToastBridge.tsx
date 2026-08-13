import React, { useEffect, useRef } from 'react';
import toast from 'react-hot-toast';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { selectActiveToastCandidate } from '../../store/Notifications/selectors';
import { hydrate, toastShown } from '../../store/Notifications/slice';
import { NotificationSeverity } from '../../store/Notifications/types';
import { savePersistedNotifications } from '../../store/Notifications/storage';

// How long a single toast stays on screen before the queue advances to the
// next one — enforces the "at most one active toast" policy.
const TOAST_DURATION_MS: Record<NotificationSeverity, number> = {
    info: 4000,
    success: 4000,
    warning: 5000,
    error: 6000,
};

const showToast = (severity: NotificationSeverity, text: string, durationMs: number): void => {
    const opts = { duration: durationMs };
    switch (severity) {
        case 'success':
            toast.success(text, opts);
            break;
        case 'error':
            toast.error(text, opts);
            break;
        case 'warning':
            toast(text, { ...opts, icon: '⚠️' });
            break;
        default:
            toast(text, opts);
    }
};

// Mounted once in App.tsx alongside <Toaster />. Bridges the Notifications
// slice (single source of truth + history) to react-hot-toast's renderer,
// showing one toast at a time and persisting dismiss state per user.
export const ToastBridge: React.FC = () => {
    const dispatch = useAppDispatch();
    const candidateId = useAppSelector(selectActiveToastCandidate);
    const items = useAppSelector((state) => state.Notifications.items);
    const userId = useAppSelector((state) => state.Auth.user?.id);
    const shownIdRef = useRef<string | null>(null);
    const timerRef = useRef<number | null>(null);

    useEffect(() => {
        dispatch(hydrate({ userId }));
    }, [dispatch, userId]);

    useEffect(() => {
        savePersistedNotifications(userId, items);
    }, [userId, items]);

    useEffect(() => {
        if (!candidateId || shownIdRef.current === candidateId) return;
        const item = items.find((n) => n.id === candidateId);
        if (!item) {
            dispatch(toastShown(candidateId));
            return;
        }

        shownIdRef.current = candidateId;
        const duration = TOAST_DURATION_MS[item.severity];
        const text = item.title ? `${item.title}: ${item.message}` : item.message;
        showToast(item.severity, text, duration);

        timerRef.current = window.setTimeout(() => {
            shownIdRef.current = null;
            dispatch(toastShown(candidateId));
        }, duration);

        return () => {
            if (timerRef.current) window.clearTimeout(timerRef.current);
        };
    }, [candidateId, items, dispatch]);

    return null;
};

export default ToastBridge;
