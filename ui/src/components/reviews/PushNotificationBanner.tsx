import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import { Alert, Button, Icons } from '../UIPrimitives';
import { isPushSupported, subscribe } from '../../utils/pushNotifications';
import { notify } from '../../utils/notify';

const DISMISSED_KEY = 'livereview:push-notify-banner-dismissed';

const readDismissed = (): boolean => {
  try {
    return localStorage.getItem(DISMISSED_KEY) === '1';
  } catch {
    return false;
  }
};

// Inline, in-page banner (not a toast) offering to enable browser push
// notifications for review progress. Lives on the review detail page itself
// - visible above the tabs regardless of which one is active - with the
// "Enable" action and a close button both part of the banner, per this
// repo's standard dismissible-Alert pattern. Stays up until the user acts on
// it or closes it (a close is remembered via localStorage so it doesn't
// reappear on every review page visit); it never auto-dismisses on a timer.
export const PushNotificationBanner: React.FC = () => {
  const [dismissed, setDismissed] = useState<boolean>(readDismissed);
  const [enabling, setEnabling] = useState(false);
  const [permission, setPermission] = useState<NotificationPermission | null>(
    typeof Notification !== 'undefined' ? Notification.permission : null
  );

  if (dismissed || !isPushSupported() || permission !== 'default') {
    return null;
  }

  const handleDismiss = () => {
    try {
      localStorage.setItem(DISMISSED_KEY, '1');
    } catch {
      // Best-effort only (e.g. private browsing) - just hide for this page view.
    }
    setDismissed(true);
  };

  const handleEnable = async () => {
    setEnabling(true);
    try {
      const result = await Notification.requestPermission();
      setPermission(result);
      if (result !== 'granted') return;
      await subscribe();
      notify.success("You're set — we'll notify you about your reviews.");
    } catch (err) {
      console.error('Failed to enable push notifications:', err);
      notify.error('Could not enable notifications. Please try again from Settings.');
    } finally {
      setEnabling(false);
    }
  };

  return (
    // Icon, Settings link, CTA and the close button are all placed inline
    // inside our own items-center row rather than via Alert's `icon`/
    // `onClose` props - Alert's own icon/close slots aren't vertically
    // centered against taller children (ours includes a button), which
    // misaligns them against the text.
    <Alert variant="info" className="mb-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Icons.Bell />
          <p>Want a browser notification when your reviews' findings are ready or complete — even with this tab closed?</p>
        </div>
        <div className="flex items-center gap-3">
          <Link to="/settings#notifications" className="text-sm font-medium text-blue-200 hover:text-white hover:underline">
            Settings
          </Link>
          <Button size="sm" onClick={handleEnable} isLoading={enabling}>
            Enable notifications
          </Button>
          <button
            type="button"
            aria-label="Dismiss"
            onClick={handleDismiss}
            className="text-blue-200 hover:text-white"
          >
            <Icons.Close />
          </button>
        </div>
      </div>
    </Alert>
  );
};

export default PushNotificationBanner;
