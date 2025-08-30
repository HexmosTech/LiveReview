import apiClient from './apiClient';

// --- TypeScript Interfaces ---

export interface Member {
  id: number;
  email: string;
  first_name?: string;
  last_name?: string;
  is_active: boolean;
  last_login_at?: string;
  created_at: string;
  updated_at: string;
  password_reset_required: boolean;
  role: string;
  role_id: number;
  org_id: number;
}

export interface FetchUsersResponse {
  members: Member[];
  total: number;
  page: number;
  limit: number;
  total_pages: number;
}

export const fetchOrgUsers = async (orgId: string): Promise<FetchUsersResponse> => {
  return apiClient.get<FetchUsersResponse>(`/orgs/${orgId}/users`);
};

export interface UserFormData {
  email: string;
  first_name: string;
  last_name: string;
  role: 'member' | 'owner' | 'super_admin';
  password?: string;
  password_confirmation?: string;
}

export interface CreateUserPayload {
  email: string;
  first_name: string;
  last_name: string;
  role_id: number; // Assuming role is sent as an ID
  password?: string;
}

export const createOrgUser = async (orgId: string, userData: CreateUserPayload): Promise<Member> => {
  return apiClient.post<Member>(`/orgs/${orgId}/users`, userData);
};

export const fetchOrgUser = async (orgId: string, userId: string): Promise<Member> => {
  return apiClient.get<Member>(`/orgs/${orgId}/users/${userId}`);
};

export interface UpdateUserPayload {
  first_name?: string;
  last_name?: string;
  role_id?: number;
}

export const updateOrgUser = async (
  orgId: string,
  userId: string,
  userData: UpdateUserPayload
): Promise<Member> => {
  return apiClient.put<Member>(`/orgs/${orgId}/users/${userId}`, userData);
};

export const deactivateOrgUser = async (orgId: string, userId: string): Promise<void> => {
  return apiClient.delete<void>(`/orgs/${orgId}/users/${userId}`);
};