// Minimal push-only service worker for LiveReview — not a full PWA/offline
// cache, just enough to receive and show a browser notification for review
// progress (first finding / completion) even when no tab is open, and to
// focus/open the right review page on click.
//
// Registered by ui/src/utils/pushNotifications.ts. Served as-is from
// ui/public/ via webpack's existing CopyPlugin (ui/webpack.config.js) —
// no build config change needed.

self.addEventListener('push', (event) => {
  let payload = { title: 'LiveReview', body: 'A review you triggered has an update.' };
  try {
    if (event.data) {
      payload = { ...payload, ...event.data.json() };
    }
  } catch (err) {
    // Non-JSON payload (shouldn't happen — the backend always sends JSON) — fall back to the default text above.
  }

  const options = {
    body: payload.body,
    tag: payload.tag,
    data: { url: payload.url },
    // .ico isn't reliably rendered as a notification icon across OS/browser
    // notification UIs (Chrome/Windows in particular) - a PNG is required.
    // notification-icon.png is favicon.svg rasterized at 192x192 (vl-convert
    // svg2png), the size Chrome's Web Push notifications expect.
    icon: '/notification-icon.png',
    badge: '/notification-icon.png',
  };

  event.waitUntil(self.registration.showNotification(payload.title, options));
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const targetUrl = event.notification.data && event.notification.data.url;
  if (!targetUrl) {
    return;
  }

  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clients) => {
      for (const client of clients) {
        if (client.url === targetUrl && 'focus' in client) {
          return client.focus();
        }
      }
      return self.clients.openWindow(targetUrl);
    })
  );
});
