import apiClient from './apiClient';

interface PasswordStatusResponse {
  isSet: boolean;
  message: string;
}

interface PasswordVerifyResponse {
  valid: boolean;
  message: string;
}

interface PasswordResponse {
  success: boolean;
  message: string;
  token?: string;
}

/**
 * Check if admin password is set
 * @returns Promise with password status 
 */
export const checkAdminPasswordStatus = async (): Promise<boolean> => {
  try {
    const response = await apiClient.get<PasswordStatusResponse>('/api/v1/password/status');
    return response.isSet;
  } catch (error) {
    console.error('Error checking password status:', error);
    throw error;
  }
};

/**
 * Set admin password
 * @param password The password to set
 * @returns Promise with success status and token
 */
export const setAdminPassword = async (password: string): Promise<{ success: boolean, token?: string }> => {
  try {
    const response = await apiClient.post<PasswordResponse>('/api/v1/password', { password });
    
    // For now, this API doesn't return a token but we'll handle it as if it might in the future
    // If successful, manually generate a token in localStorage for now
    // This is a temporary solution until the backend implements proper JWT auth
    if (response.success) {
      const tempToken = `temp_${Math.random().toString(36).substring(2, 15)}`;
      localStorage.setItem('authToken', tempToken);
      
      return {
        success: true,
        token: tempToken
      };
    }
    
    return {
      success: response.success
    };
  } catch (error) {
    console.error('Error setting admin password:', error);
    throw error;
  }
};

/**
 * Verify admin password
 * @param password The password to verify
 * @returns Promise with verification status and token
 */
export const verifyAdminPassword = async (password: string): Promise<{ valid: boolean, token?: string }> => {
  try {
    const response = await apiClient.post<PasswordVerifyResponse>('/api/v1/password/verify', { password });
    
    // For now, this API doesn't return a token but we'll handle it as if it might in the future
    // If successful login, manually generate a token in localStorage for now
    // This is a temporary solution until the backend implements proper JWT auth
    if (response.valid) {
      const tempToken = `temp_${Math.random().toString(36).substring(2, 15)}`;
      localStorage.setItem('authToken', tempToken);
      
      return {
        valid: true,
        token: tempToken
      };
    }
    
    return {
      valid: response.valid
    };
  } catch (error) {
    // For unauthorized errors, return a structured response
    if (error instanceof Error && error.message === 'Unauthorized') {
      return { valid: false };
    }
    console.error('Error verifying admin password:', error);
    throw error;
  }
};
