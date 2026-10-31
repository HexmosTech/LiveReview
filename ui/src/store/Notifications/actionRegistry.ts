// Notification `actions[].actionId` resolves to a handler registered here at
// render time, keeping the Redux Notifications state serializable (no
// functions stored) while still letting tray rows trigger real behavior.
const handlers: Record<string, () => void | Promise<void>> = {};

export function registerNotificationAction(actionId: string, handler: () => void | Promise<void>): () => void {
    handlers[actionId] = handler;
    return () => {
        if (handlers[actionId] === handler) delete handlers[actionId];
    };
}

export function runNotificationAction(actionId: string): void {
    const handler = handlers[actionId];
    if (handler) {
        void handler();
        return;
    }
    // Fallback: If actionId is an external URL, open in a new tab.
    if (actionId.startsWith('http://') || actionId.startsWith('https://')) {
        window.open(actionId, '_blank', 'noopener,noreferrer');
        return;
    }
    // Fallback: If actionId is an internal hash/pathname route (e.g. /git).
    // Use assign() so the HashRouter picks up the hashchange event properly
    // instead of bypassing React Router's history state with a direct assignment.
    if (actionId.startsWith('/')) {
        window.location.assign(`#${actionId}`);
        return;
    }
}
