// API client for license operations
// Assumes the backend is served from same origin; adjust base if needed.

export interface LicenseStatusResponse {
  status: string;
  subject?: string;
  appName?: string;
  seatCount?: number;
  unlimited: boolean;
  expiresAt?: string;
  lastValidatedAt?: string;
  lastValidationCode?: string;
}

export interface LicenseErrorResponse { error: string }

const base = '/api/v1/license';

export async function getLicenseStatus(signal?: AbortSignal): Promise<LicenseStatusResponse> {
  const res = await fetch(`${base}/status`, { signal, credentials: 'include' });
  if (!res.ok) throw new Error(`status_http_${res.status}`);
  return res.json();
}

export async function updateLicense(token: string): Promise<LicenseStatusResponse> {
  const res = await fetch(`${base}/update`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({ token }),
  });
  if (!res.ok) throw new Error((await res.json()).error || `update_http_${res.status}`);
  return res.json();
}

export async function refreshLicense(): Promise<LicenseStatusResponse> {
  const res = await fetch(`${base}/refresh`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: '{}',
  });
  if (!res.ok) throw new Error((await res.json()).error || `refresh_http_${res.status}`);
  return res.json();
}
