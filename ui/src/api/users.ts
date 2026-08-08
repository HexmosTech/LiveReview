import apiClient from './apiClient';

export interface UserCheckResponse {
  exists: boolean;
  id?: number;
  first_name?: string;
  last_name?: string;
}

export const checkUserByEmail = async (orgId: string, email: string): Promise<UserCheckResponse> => {
  return apiClient.get<UserCheckResponse>(`/orgs/${orgId}/users/check?email=${encodeURIComponent(email)}`);
};

export interface BulkCheckUserRow {
  email: string;
  first_name: string;
  last_name: string;
  role: string;
}

export interface BulkCheckResultRow {
  email: string;
  first_name: string;
  last_name: string;
  role: string;
  exists: boolean;
  exists_globally: boolean;
  old_email?: string;
  old_first_name?: string;
  old_last_name?: string;
  old_role?: string;
}

export interface BulkCheckResponse {
  results: BulkCheckResultRow[];
}

export const bulkCheckUsers = async (orgId: string, users: BulkCheckUserRow[]): Promise<BulkCheckResultRow[]> => {
  const response = await apiClient.post<BulkCheckResponse>(`/orgs/${orgId}/users/bulk-check`, { users });
  return response.results;
};

export interface BulkInviteUserRow {
  email: string;
  first_name: string;
  last_name: string;
  role: string;
  password: string;
  confirm_password: string;
}

export interface BulkInviteResultRow {
  email: string;
  status: 'invited' | 'updated' | 'unchanged' | 'error';
  message?: string;
  onboarding_api_key?: string;
}

export interface BulkInviteResponse {
  results: BulkInviteResultRow[];
}

export const submitBulkInvite = async (orgId: string, users: BulkInviteUserRow[]): Promise<BulkInviteResultRow[]> => {
  const response = await apiClient.post<BulkInviteResponse>(`/orgs/${orgId}/users/bulk-invite`, { users });
  return response.results;
};

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
  onboarding_api_key?: string;
}

export interface FetchUsersResponse {
  members: Member[];
  total: number;
  page: number;
  limit: number;
  total_pages: number;
}

export interface FetchOrgUsersParams {
  page?: number;
  perPage?: number;
  search?: string;
  sort?: 'user' | 'status' | 'role' | 'joined';
  order?: 'asc' | 'desc';
}

export const fetchOrgUsers = async (orgId: string, params: FetchOrgUsersParams = {}): Promise<FetchUsersResponse> => {
  const query = new URLSearchParams();
  if (params.page) query.append('page', params.page.toString());
  if (params.perPage) query.append('limit', params.perPage.toString());
  if (params.search) query.append('search', params.search);
  if (params.sort) query.append('sort', params.sort);
  if (params.order) query.append('order', params.order);
  const qs = query.toString();
  return apiClient.get<FetchUsersResponse>(`/orgs/${orgId}/users${qs ? `?${qs}` : ''}`);
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
  password?: string;
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