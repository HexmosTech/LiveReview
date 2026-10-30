// Web Push subscribe/unsubscribe plumbing for review-progress notifications.
// See the Browser Push Notifications plan for the full design.
import { createPushSubscription, deletePushSubscription, getVapidPublicKey } from '../api/push';
import { notify } from './notify';

export const PUSH_ENABLE_ACTION_ID = 'push-notify:enable';

export function isPushSupported(): boolean {
  return (
    typeof window !== 'undefined' &&
    'serviceWorker' in navigator &&
    'PushManager' in window &&
    window.isSecureContext
  );
}

// The VAPID public key comes back base64url-encoded (SherClockHolmes/webpush-go's
// GenerateVAPIDKeys output); pushManager.subscribe wants it as a raw Uint8Array.
function base64UrlToUint8Array(base64Url: string): Uint8Array {
  const padding = '='.repeat((4 - (base64Url.length % 4)) % 4);
  const base64 = (base64Url + padding).replace(/-/g, '+').replace(/_/g, '/');
  const rawData = window.atob(base64);
  const output = new Uint8Array(rawData.length);
  for (let i = 0; i < rawData.length; i++) {
    output[i] = rawData.charCodeAt(i);
  }
  return output;
}

let swRegistrationPromise: Promise<ServiceWorkerRegistration> | null = null;

export function registerServiceWorker(): Promise<ServiceWorkerRegistration> {
  if (!isPushSupported()) {
    return Promise.reject(new Error('Push notifications are not supported in this browser/context.'));
  }
  if (!swRegistrationPromise) {
    swRegistrationPromise = navigator.serviceWorker.register('/sw.js');
  }
  return swRegistrationPromise;
}

export async function getExistingSubscription(): Promise<PushSubscription | null> {
  if (!isPushSupported()) return null;
  const registration = await navigator.serviceWorker.getRegistration('/sw.js');
  if (!registration) return null;
  return registration.pushManager.getSubscription();
}

function subscriptionToPayload(subscription: PushSubscription) {
  const keys = subscription.toJSON().keys;
  return {
    endpoint: subscription.endpoint,
    keys: { p256dh: keys?.p256dh || '', auth: keys?.auth || '' },
  };
}

// Subscribes this browser and registers it with the backend. Caller is
// responsible for requesting Notification permission first (the toast
// action handler does this, since it's the one user gesture the browser
// requires permission to be tied to).
export async function subscribe(): Promise<void> {
  const registration = await registerServiceWorker();
  const vapidPublicKey = await getVapidPublicKey();
  const subscription = await registration.pushManager.subscribe({
    userVisibleOnly: true,
    applicationServerKey: base64UrlToUint8Array(vapidPublicKey) as BufferSource,
  });
  await createPushSubscription(subscriptionToPayload(subscription));
}

export async function unsubscribe(): Promise<void> {
  const subscription = await getExistingSubscription();
  if (!subscription) return;
  const endpoint = subscription.endpoint;
  await subscription.unsubscribe();
  await deletePushSubscription(endpoint);
}

// Call this right after ANY code path successfully triggers a review -
// there are several (the "Create Review" form, the "Start a review" link
// posted on PRs/MRs, the Explore > Pull/Merge Requests list) and this must
// not be missed from any of them. Offers to enable browser push
// notifications via a toast with an inline "Enable" action button (handled
// by PushNotificationPrompt, mounted once at the app root) - no navigation
// to Settings required. No-ops if push isn't supported, or the browser has
// already been asked (permission is 'granted' or 'denied').
export function maybePromptForPushNotifications(): void {
  if (!isPushSupported() || typeof Notification === 'undefined' || Notification.permission !== 'default') {
    return;
  }
  notify.info("Want a browser notification when this review's findings are ready?", {
    source: 'push-notify',
    dedupeKey: 'push-notify-prompt',
    actions: [{ label: 'Enable', actionId: PUSH_ENABLE_ACTION_ID }],
  });
}
