import apiClient from './apiClient';

interface PasswordStatusResponse {
  is_set: boolean;  // Changed from isSet to is_set to match API response
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
    console.log('Checking admin password status...');
    const response = await apiClient.get<PasswordStatusResponse>('/api/v1/password/status');
    console.log('Password status response:', response);
    return response.is_set;  // Changed from isSet to is_set
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
    console.log('Setting admin password:', { passwordLength: password.length });
    
    // Validate password before sending to API
    if (!password || password.length < 8) {
      console.error('Password validation failed: Password must be at least 8 characters');
      throw new Error('Password must be at least 8 characters long');
    }
    
    const response = await apiClient.post<PasswordResponse>('/api/v1/password', { password });
    console.log('Password set response:', response);
    
    // For now, this API doesn't return a token but we'll handle it as if it might in the future
    // If successful, manually generate a token in localStorage for now
    // This is a temporary solution until the backend implements proper JWT auth
    if (response.success) {
      // Generate a token for auth purposes
      const tempToken = `temp_${Math.random().toString(36).substring(2, 15)}`;
      console.log('Generated auth token');
      
      // Store in localStorage so it persists across page reloads
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
      // Generate a token for auth purposes
      const tempToken = `temp_${Math.random().toString(36).substring(2, 15)}`;
      // Store in localStorage so it persists across page reloads
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
