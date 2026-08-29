import React, { useState, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { Alert } from './UIPrimitives';

type ToastType = 'success' | 'error' | 'warning' | 'info';

interface Toast {
  id: number;
  type: ToastType;
  message: string;
}

let toastId = 0;

/**
 * Simple hook for showing transient toast notifications.
 * Returns a `showToast` function and a `ToastContainer` component to render.
 *
 * Usage:
 *   const { showToast, ToastContainer } = useToast();
 *   showToast('Review deleted', 'success');
 *   return <><ToastContainer /></>
 */
export function useToast() {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const showToast = useCallback((message: string, type: ToastType = 'info', durationMs = 5000) => {
    const id = ++toastId;
    setToasts(prev => [...prev, { id, type, message }]);
    if (durationMs > 0) {
      setTimeout(() => {
        setToasts(prev => prev.filter(t => t.id !== id));
      }, durationMs);
    }
  }, []);

  const dismissToast = useCallback((id: number) => {
    setToasts(prev => prev.filter(t => t.id !== id));
  }, []);

  const ToastContainer = useCallback(() => {
    if (toasts.length === 0) return null;
    return createPortal(
      <div className="fixed top-4 right-4 z-[9999] space-y-2 max-w-sm">
        {toasts.map(toast => (
          <Alert
            key={toast.id}
            variant={toast.type}
            onClose={() => dismissToast(toast.id)}
          >
            {toast.message}
          </Alert>
        ))}
      </div>,
      document.body
    );
  }, [toasts, dismissToast]);

  return { showToast, ToastContainer };
}
