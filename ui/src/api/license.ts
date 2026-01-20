// API client for license operations
import apiClient from './apiClient';

export interface LicenseStatusResponse {
  status: string;
  subject?: string;
  appName?: string;
  seatCount?: number;
  activeUsers?: number;
  assignedSeats?: number;
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

// Seat Assignment Types (self-hosted only)
export interface SeatAssignment {
  id: number;
  user_id: number;
  email: string;
  first_name?: string;
  last_name?: string;
  assigned_by_user_id?: number;
  assigned_by_email?: string;
  assigned_at: string;
  is_active: boolean;
}

export interface SeatAssignmentListResponse {
  assignments: SeatAssignment[];
  total_seats: number;
  assigned_seats: number;
  available_seats: number;
  unlimited: boolean;
}

export interface UnassignedUser {
  id: number;
  email: string;
  first_name?: string;
  last_name?: string;
  is_active: boolean;
  role?: string;
}

// Seat Assignment API (self-hosted only)
export async function getSeatAssignments(): Promise<SeatAssignmentListResponse> {
  return apiClient.get<SeatAssignmentListResponse>('/license/seats');
}

export async function getUnassignedUsers(): Promise<{ users: UnassignedUser[] }> {
  return apiClient.get<{ users: UnassignedUser[] }>('/license/seats/unassigned');
}

export async function assignSeat(userId: number): Promise<{ message: string }> {
  return apiClient.post<{ message: string }>('/license/seats/assign', { user_id: userId });
}

export async function bulkAssignSeats(userIds: number[]): Promise<{ message: string; assigned: number; total: number }> {
  return apiClient.post<{ message: string; assigned: number; total: number }>('/license/seats/assign-bulk', { user_ids: userIds });
}

export async function revokeSeat(userId: number): Promise<{ message: string }> {
  return apiClient.delete<{ message: string }>(`/license/seats/${userId}`);
}

export async function bulkRevokeSeats(userIds: number[]): Promise<{ message: string; revoked: number; total: number }> {
  return apiClient.post<{ message: string; revoked: number; total: number }>('/license/seats/revoke-bulk', { user_ids: userIds });
}
