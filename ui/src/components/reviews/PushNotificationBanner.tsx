import React, { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Alert, Button, Icons } from '../UIPrimitives';
import { getExistingSubscription, isPushSupported, subscribe } from '../../utils/pushNotifications';
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
  // Tracks whether a PushManager subscription actually exists - not the
  // same thing as `permission === 'granted'`. requestPermission() can
  // succeed while the subsequent subscribe() call still fails (VAPID key
  // fetch, PushManager.subscribe() rejecting, the backend POST failing),
  // and permission alone can't tell those apart. Checked once on mount so
  // the banner doesn't show again for a browser that's already subscribed
  // (e.g. enabled from Settings instead of this banner).
  const [subscribed, setSubscribed] = useState<boolean | null>(null);

  useEffect(() => {
    let cancelled = false;
    if (isPushSupported()) {
      getExistingSubscription().then((sub) => {
        if (!cancelled) setSubscribed(!!sub);
      });
    } else {
      setSubscribed(false);
    }
    return () => {
      cancelled = true;
    };
  }, []);

  if (dismissed || !isPushSupported() || subscribed !== false) {
    return null;
  }

  // Browsers never let a site re-trigger the native permission prompt once
  // blocked, and give no way to tell "closed the prompt" apart from
  // "clicked Block" - the only recovery path is the browser's own site
  // settings (the icon next to the address bar), so we point there instead
  // of silently hiding or re-prompting.
  const blocked = permission === 'denied';

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
      setSubscribed(true);
      notify.success("You're set — we'll notify you about your reviews.");
    } catch (err) {
      console.error('Failed to enable push notifications:', err);
      notify.error('Could not enable notifications. Please try again, or from Settings.');
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
    <Alert variant={blocked ? 'warning' : 'info'} className="mb-6">
      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <Icons.Bell />
            {blocked ? (
              <p>Notifications are blocked for this site in your browser.</p>
            ) : (
              <p>Want a browser notification when your reviews' findings are ready?</p>
            )}
          </div>
          <div className="flex items-center gap-3">
            <Link
              to="/settings#notifications"
              className={`text-sm font-medium hover:text-white hover:underline ${blocked ? 'text-amber-200' : 'text-blue-200'}`}
            >
              Settings
            </Link>
            {!blocked && (
              <Button size="sm" onClick={handleEnable} isLoading={enabling}>
                Enable notifications
              </Button>
            )}
            <button
              type="button"
              aria-label="Dismiss"
              onClick={handleDismiss}
              className={`hover:text-white ${blocked ? 'text-amber-200' : 'text-blue-200'}`}
            >
              <Icons.Close />
            </button>
          </div>
        </div>
        <p className={`text-xs ${blocked ? 'text-amber-200/80' : 'text-blue-200/80'}`}>
          {blocked
            ? 'Click the lock or info icon next to your browser\'s address bar, allow notifications for this site, then reload the page.'
            : 'Your browser may show a permission request near its address bar - allow it there to finish.'}
        </p>
      </div>
    </Alert>
  );
};

export default PushNotificationBanner;
