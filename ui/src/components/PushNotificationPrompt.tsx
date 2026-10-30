import React, { useEffect } from 'react';
import { registerNotificationAction } from '../store/Notifications/actionRegistry';
import { subscribe } from '../utils/pushNotifications';
import { notify } from '../utils/notify';

export const PUSH_ENABLE_ACTION_ID = 'push-notify:enable';

// Headless (renders nothing) — mounted once at the app root (App.tsx), same
// pattern as URLMismatchBanner/LicenseStatusBar. Registers the handler the
// "Enable" button on the post-trigger-review toast calls, so granting
// permission happens right there without navigating to Settings.
export const PushNotificationPrompt: React.FC = () => {
  useEffect(() => {
    const unregister = registerNotificationAction(PUSH_ENABLE_ACTION_ID, async () => {
      try {
        const permission = await Notification.requestPermission();
        if (permission !== 'granted') {
          return;
        }
        await subscribe();
        notify.success("You're set — we'll notify you about your reviews.");
      } catch (err) {
        console.error('Failed to enable push notifications:', err);
        notify.error('Could not enable notifications. Please try again from Settings.');
      }
    });
    return unregister;
  }, []);

  return null;
};

export default PushNotificationPrompt;
