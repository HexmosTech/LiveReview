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
    if (handler) void handler();
}
