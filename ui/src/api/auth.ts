import apiClient from './apiClient';

// --- New API Types ---

export interface UserInfo {
  id: number;
  email: string;
  created_at: string;
  updated_at: string;
  plan_type?: string;
  license_expires_at?: string;
}

export interface OrgInfo {
  id: number;
  name: string;
  role: string;
}

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  expires_at: string;
  token_type: string;
}

export interface LoginResponse {
  user: UserInfo;
  tokens: TokenPair;
  organizations: OrgInfo[];
}

export interface MeResponse {
  user: UserInfo;
  organizations: OrgInfo[];
}

export interface SetupStatusResponse {
  setup_required: boolean;
  user_count: number;
}

export interface SetupAdminResponse {
  message: string;
  user: UserInfo;
  tokens: TokenPair;
  organizations: OrgInfo[];
}

// --- New API Functions ---

/**
 * Check if initial setup is needed
 */
export const checkSetupStatus = async (): Promise<SetupStatusResponse> => {
  const response = await apiClient.get<SetupStatusResponse>('/auth/setup-status');
  return response;
};

/**
 * Setup the initial admin user and organization
 */
export const setupAdmin = async (email: string, password: string, orgName: string): Promise<SetupAdminResponse> => {
  const response = await apiClient.post<SetupAdminResponse>('/auth/setup', {
    email,
    password,
    org_name: orgName,
  });
  return response;
};

/**
 * Login a user
 */
export const login = async (email: string, password: string): Promise<LoginResponse> => {
  const response = await apiClient.post<LoginResponse>('/auth/login', { email, password });
  return response;
};

/**
 * Logout a user
 */
export const logout = async (refreshToken?: string): Promise<void> => {
  await apiClient.post('/auth/logout', { refresh_token: refreshToken });
};

/**
 * Get current user info
 */
export const getMe = async (): Promise<MeResponse> => {
  const response = await apiClient.get<MeResponse>('/auth/me');
  return response;
};

/**
 * Refresh access token
 */
export const refreshToken = async (token: string): Promise<TokenPair> => {
  const response = await apiClient.post<TokenPair>('/auth/refresh', { refresh_token: token });
  return response;
};


// --- Deprecated Legacy Functions ---

interface PasswordStatusResponse {
  is_set: boolean;
  message: string;
}

interface PasswordVerifyResponse {
  valid: boolean;
  message: string;
}

interface PasswordResponse {
  success: boolean;
  message: string;
}

/**
 * @deprecated Use checkSetupStatus instead.
 * Check if admin password is set
 * @returns Promise with password status 
 */
export const checkAdminPasswordStatus = async (): Promise<boolean> => {
  try {
    console.log('Checking admin password status...');
    const response = await apiClient.get<PasswordStatusResponse>('/api/v1/password/status');
    console.log('Password status response:', response);
    return response.is_set;
  } catch (error) {
    console.error('Error checking password status:', error);
    throw error;
  }
};

/**
 * @deprecated Use setupAdmin instead.
 * Set admin password
 * @param password The password to set
 * @returns Promise with success status
 */
export const setAdminPassword = async (password: string): Promise<{ success: boolean }> => {
  try {
    console.log('Setting admin password:', { passwordLength: password.length });
    
    if (!password || password.length < 8) {
      console.error('Password validation failed: Password must be at least 8 characters');
      throw new Error('Password must be at least 8 characters long');
    }
    
    const response = await apiClient.post<PasswordResponse>('/api/v1/password', { password });
    console.log('Password set response:', response);
    
    return {
      success: response.success
    };
  } catch (error) {
    console.error('Error setting admin password:', error);
    throw error;
  }
};

/**
 * @deprecated Use login instead.
 * Verify admin password
 * @param password The password to verify
 * @returns Promise with success status
 */
export const verifyAdminPassword = async (password: string): Promise<{ success: boolean }> => {
  try {
    console.log('Verifying admin password');
    
    const response = await apiClient.post<PasswordVerifyResponse>('/api/v1/password/verify', { password });
    console.log('Password verification raw response:', JSON.stringify(response));
    
    const result = {
      success: response.valid
    };
    
    console.log('Mapped verification response:', result);
    return result;
  } catch (error) {
    console.error('Error verifying admin password:', error);
    throw error;
  }
};

/**
 * Get system information including dev mode status
 */
export const getSystemInfo = async (): Promise<{ dev_mode: boolean; version: any }> => {
  const response = await apiClient.get<{ dev_mode: boolean; version: any }>('/api/v1/system/info');
  return response;
};
