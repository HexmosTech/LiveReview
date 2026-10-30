import React, { useEffect, useState } from 'react';
import { Card, Toggle, Button, Spinner } from '../../components/UIPrimitives';
import { notify } from '../../utils/notify';
import {
  getNotificationPreferences,
  updateNotificationPreferences,
  NotificationPreferences,
} from '../../api/push';
import { getExistingSubscription, isPushSupported, subscribe, unsubscribe } from '../../utils/pushNotifications';

const DEFAULT_PREFS: NotificationPreferences = { notify_first_finding: true, notify_completion: true };

// Self-service: every authenticated user manages their own notification
// settings, mirroring AccountTab's gating (no org-role check needed).
const NotificationsTab: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [prefs, setPrefs] = useState<NotificationPreferences>(DEFAULT_PREFS);
  const [saving, setSaving] = useState(false);
  const [subscribed, setSubscribed] = useState(false);
  const [subscribing, setSubscribing] = useState(false);
  const [permission, setPermission] = useState<NotificationPermission | null>(
    typeof Notification !== 'undefined' ? Notification.permission : null
  );
  const supported = isPushSupported();
  const blocked = permission === 'denied';

  useEffect(() => {
    let cancelled = false;
    Promise.all([getNotificationPreferences(), getExistingSubscription()])
      .then(([p, sub]) => {
        if (cancelled) return;
        setPrefs(p);
        setSubscribed(!!sub);
      })
      .catch((err) => {
        console.error('Failed to load notification settings:', err);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const savePrefs = async (next: NotificationPreferences) => {
    setPrefs(next);
    setSaving(true);
    try {
      await updateNotificationPreferences(next);
    } catch (err) {
      notify.error('Failed to save notification preferences.');
    } finally {
      setSaving(false);
    }
  };

  const handleToggleBrowser = async () => {
    setSubscribing(true);
    try {
      if (subscribed) {
        await unsubscribe();
        setSubscribed(false);
      } else {
        if (Notification.permission === 'denied') {
          setPermission('denied');
          return;
        }
        const result = await Notification.requestPermission();
        setPermission(result);
        if (result !== 'granted') return;
        await subscribe();
        setSubscribed(true);
        notify.success("You're set — we'll notify you about your reviews.");
      }
    } catch (err) {
      console.error('Failed to toggle push notifications:', err);
      notify.error('Could not update browser notification settings.');
    } finally {
      setSubscribing(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-10">
        <Spinner />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <Card>
        <h2 className="text-lg font-semibold text-white mb-1">Browser Notifications</h2>
        <p className="text-sm text-slate-400 mb-4">
          Get a notification on this device when your reviews have news — even if LiveReview isn't open in a tab.
        </p>
        {!supported ? (
          <p className="text-sm text-slate-500">
            Browser notifications aren't available in this browser or over an insecure connection (HTTPS is required).
          </p>
        ) : blocked ? (
          <p className="text-sm text-amber-400">
            Notifications are blocked for this site. Click the lock or info icon next to your browser's address bar,
            allow notifications for this site, then reload the page.
          </p>
        ) : (
          <>
            <Button onClick={handleToggleBrowser} isLoading={subscribing} variant={subscribed ? 'outline' : 'primary'}>
              {subscribed ? 'Disable on this browser' : 'Enable on this browser'}
            </Button>
            {!subscribed && (
              <p className="text-xs text-slate-500 mt-2">
                Your browser may show a permission request near its address bar - allow it there to finish.
              </p>
            )}
          </>
        )}
      </Card>

      <Card>
        <h2 className="text-lg font-semibold text-white mb-1">What to notify me about</h2>
        <p className="text-sm text-slate-400 mb-4">Both are on by default. Applies to reviews you trigger yourself.</p>
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-white">First finding is ready</p>
              <p className="text-xs text-slate-500">As soon as the first batch of results streams in.</p>
            </div>
            <Toggle
              checked={prefs.notify_first_finding}
              disabled={saving}
              onChange={(checked) => savePrefs({ ...prefs, notify_first_finding: checked })}
              aria-label="Notify on first finding"
            />
          </div>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-white">Review completed or failed</p>
              <p className="text-xs text-slate-500">When the whole review finishes.</p>
            </div>
            <Toggle
              checked={prefs.notify_completion}
              disabled={saving}
              onChange={(checked) => savePrefs({ ...prefs, notify_completion: checked })}
              aria-label="Notify on completion"
            />
          </div>
        </div>
      </Card>
    </div>
  );
};

export default NotificationsTab;
