// API proxy utilities for backend authentication APIs

/**
 * Proxy the check admin password status API
 * @returns Promise with the API response
 */
export const proxyCheckAdminPasswordStatus = async () => {
  try {
    const response = await fetch('/api/proxy/check-admin-password-status', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
    });
    
    if (!response.ok) {
      throw new Error('Failed to check password status');
    }
    
    return await response.json();
  } catch (error) {
    console.error('Error proxying check password status:', error);
    throw error;
  }
};

/**
 * Proxy the set admin password API
 * @param password The password to set
 * @returns Promise with the API response
 */
export const proxySetAdminPassword = async (password: string) => {
  try {
    const response = await fetch('/api/proxy/set-admin-password', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ password }),
    });
    
    if (!response.ok) {
      throw new Error('Failed to set admin password');
    }
    
    return await response.json();
  } catch (error) {
    console.error('Error proxying set admin password:', error);
    throw error;
  }
};

/**
 * Proxy the verify admin password API
 * @param password The password to verify
 * @returns Promise with the API response
 */
export const proxyVerifyAdminPassword = async (password: string) => {
  try {
    const response = await fetch('/api/proxy/verify-admin-password', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ password }),
    });
    
    if (!response.ok) {
      // If response is unauthorized (401), we still want to return the response
      if (response.status === 401) {
        return { valid: false };
      }
      throw new Error('Failed to verify admin password');
    }
    
    return await response.json();
  } catch (error) {
    console.error('Error proxying verify admin password:', error);
    throw error;
  }
};
