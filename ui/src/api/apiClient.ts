/**
 * Simple API client with base URL and authentication support
 */

// Base URL for all API requests
// Dynamically determine the base URL - use the current origin for API calls
// This ensures API calls work correctly in development and production
const BASE_URL = window.location.origin;

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
      let errorMessage = `Request failed with status ${response.status}: ${response.statusText}`;
      let errorData = null;
      
      try {
        const responseText = await response.text();
        console.log('API error response text:', responseText); // Debug log
        
        if (responseText) {
          try {
            errorData = JSON.parse(responseText);
            console.log('API error response parsed:', errorData, response.status); // Debug log
            
            // The server returns errors in an "error" field (from ErrorResponse struct)
            if (errorData.error) {
              errorMessage = errorData.error;
            } else if (errorData.message) {
              errorMessage = errorData.message;
            }
          } catch (parseError) {
            // If JSON parsing fails, use the text as error message if it's reasonable
            if (responseText.length < 200 && !responseText.includes('<html')) {
              errorMessage = responseText;
            }
          }
        }
        
        const error = new Error(errorMessage);
        // Add status code to the error object for better error handling
        (error as any).status = response.status;
        (error as any).statusText = response.statusText;
        (error as any).url = url;
        (error as any).data = errorData;
        throw error;
      } catch (networkError) {
        console.log('Failed to read error response:', networkError);
        // If we can't read the response, throw an error with status information
        const error = new Error(errorMessage);
        (error as any).status = response.status;
        (error as any).statusText = response.statusText;
        (error as any).url = url;
        throw error;
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
