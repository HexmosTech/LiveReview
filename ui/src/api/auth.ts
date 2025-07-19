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
 * @returns Promise with success status
 */
export const setAdminPassword = async (password: string): Promise<{ success: boolean }> => {
  try {
    console.log('Setting admin password:', { passwordLength: password.length });
    
    // Validate password before sending to API
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
 * Verify admin password
 * @param password The password to verify
 * @returns Promise with success status
 */
export const verifyAdminPassword = async (password: string): Promise<{ success: boolean }> => {
  try {
    console.log('Verifying admin password');
    
    const response = await apiClient.post<PasswordVerifyResponse>('/api/v1/password/verify', { password });
    console.log('Password verification raw response:', JSON.stringify(response));
    
    // Map the API's 'valid' property to our 'success' property
    const result = {
      success: response.valid // Map 'valid' to 'success'
    };
    
    console.log('Mapped verification response:', result);
    return result;
  } catch (error) {
    console.error('Error verifying admin password:', error);
    throw error;
  }
};
