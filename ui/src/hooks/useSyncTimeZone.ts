import { useEffect } from 'react';
import { useAppSelector } from '../store/configureStore';
import { saveTimeZone } from '../api/profile';

// Sends the browser's time zone once per user, and again only if it changes.
export function useSyncTimeZone(): void {
  const userId = useAppSelector((state) => state.Auth.user?.id);
  useEffect(() => {
    if (!userId) return;
    const key = `lr_tz_synced:${userId}`;
    let zone = '';
    try {
      zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
      if (!zone || localStorage.getItem(key) === zone) return;
    } catch {
      return;
    }
    saveTimeZone(zone)
      .then(() => {
        try { localStorage.setItem(key, zone); } catch { /* storage blocked: retry next visit */ }
      })
      .catch(() => { /* not critical: the PR table falls back to UTC */ });
  }, [userId]);
}
