/**
 * Simple API client with base URL and authentication support
 */

// Base URL for all API requests
const BASE_URL = 'http://localhost:8888';

// Default request options
const defaultOptions: RequestInit = {
  headers: {
    'Content-Type': 'application/json',
  },
};

/**
 * Make an API request with authentication and common handling
 */
async function apiRequest<T>(
  endpoint: string, 
  options: RequestInit = {}
): Promise<T> {
  // Combine the endpoint with the base URL
  const url = `${BASE_URL}${endpoint}`;
  
  // Merge default options with provided options
  const requestOptions: RequestInit = {
    ...defaultOptions,
    ...options,
  };
  
  // If we have a password, add it to request headers for authentication
  const password = localStorage.getItem('authPassword');
  if (password) {
    requestOptions.headers = {
      ...requestOptions.headers,
      'X-Admin-Password': password,
    };
  }
  
  try {
    // Make the request
    const response = await fetch(url, requestOptions);
    
    // Handle unauthorized errors globally
    if (response.status === 401) {
      // Clear any stored auth data
      localStorage.removeItem('authPassword');
      
      // Redirect to login if we're not already there
      if (!window.location.hash.includes('login')) {
        window.location.hash = 'login';
      }
      
      throw new Error('Unauthorized');
    }
    
    // Handle any other errors
    if (!response.ok) {
      // Try to get error details from the response
      try {
        const errorData = await response.json();
        throw new Error(errorData.error || `API error: ${response.status}`);
      } catch (jsonError) {
        // If we can't parse the JSON, just throw a generic error
        throw new Error(`API error: ${response.status}`);
      }
    }
    
    // Parse the JSON response
    const data = await response.json();
    console.log(`API response from ${endpoint}:`, JSON.stringify(data));
    return data as T;
  } catch (error) {
    console.error('API request failed:', error);
    throw error;
  }
}

/**
 * Convenience methods for common HTTP methods
 */
const apiClient = {
  /**
   * Make a GET request
   */
  get: <T>(endpoint: string, options: RequestInit = {}): Promise<T> => {
    return apiRequest<T>(endpoint, {
      ...options,
      method: 'GET',
    });
  },
  
  /**
   * Make a POST request with JSON body
   */
  post: <T>(endpoint: string, data: any, options: RequestInit = {}): Promise<T> => {
    return apiRequest<T>(endpoint, {
      ...options,
      method: 'POST',
      body: JSON.stringify(data),
    });
  },
  
  /**
   * Make a PUT request with JSON body
   */
  put: <T>(endpoint: string, data: any, options: RequestInit = {}): Promise<T> => {
    return apiRequest<T>(endpoint, {
      ...options,
      method: 'PUT',
      body: JSON.stringify(data),
    });
  },
  
  /**
   * Make a DELETE request
   */
  delete: <T>(endpoint: string, options: RequestInit = {}): Promise<T> => {
    return apiRequest<T>(endpoint, {
      ...options,
      method: 'DELETE',
    });
  },
};

export default apiClient;
