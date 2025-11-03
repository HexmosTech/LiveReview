// API client for license operations
import apiClient from './apiClient';

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

export async function getLicenseStatus(signal?: AbortSignal): Promise<LicenseStatusResponse> {
  return apiClient.get<LicenseStatusResponse>('/license/status');
}

export async function updateLicense(token: string): Promise<LicenseStatusResponse> {
  return apiClient.post<LicenseStatusResponse>('/license/update', { token });
}

export async function refreshLicense(): Promise<LicenseStatusResponse> {
  return apiClient.post<LicenseStatusResponse>('/license/refresh', {});
}

export async function deleteLicense(): Promise<{ message: string }> {
  return apiClient.delete<{ message: string }>('/license/delete');
}
