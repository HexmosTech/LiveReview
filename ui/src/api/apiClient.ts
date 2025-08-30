/**
 * Simple API client with base URL and authentication support
 */

import { AppStore } from '../store/configureStore';
import { logout, setTokens } from '../store/Auth/reducer';
import { refreshToken } from './auth';
import { tokenManager } from '../utils/tokenManager';

// Extend window interface to include our configuration
declare global {
  interface Window {
    LIVEREVIEW_CONFIG?: {
      apiUrl: string;
    };
    __REDUX_STORE__?: any;
  }
}

// Base URL for all API requests
// Priority: 1) Runtime injected config, 2) Build-time env var, 3) Auto-detect
const getBaseUrl = (): string => {
  // Check for runtime injected configuration (highest priority)
  if (window.LIVEREVIEW_CONFIG?.apiUrl) {
    return window.LIVEREVIEW_CONFIG.apiUrl;
  }
  
  // Check for build-time environment variable (fallback)
  if (process.env.REACT_APP_API_URL) {
    return process.env.REACT_APP_API_URL;
  }
  
  // In development with webpack dev server, use current origin (proxy handles routing)
  // In production, determine API URL based on current location
  const currentOrigin = window.location.origin;
  
  // If running on port 8081 (UI port), API is likely on port 8888
  if (currentOrigin.includes(':8081')) {
    return currentOrigin.replace(':8081', ':8888');
  }
  
  // Otherwise use current origin (for cases where API and UI are on same port)
  return currentOrigin;
};

const BASE_URL = getBaseUrl();

// Default request options
const defaultOptions: RequestInit = {
  headers: {
    'Content-Type': 'application/json',
  },
};

// Track ongoing refresh request to prevent multiple simultaneous refreshes
let refreshPromise: Promise<string> | null = null;

/**
 * Attempt to refresh the access token using the stored refresh token
 */
async function attemptTokenRefresh(): Promise<string> {
  const refreshToken = localStorage.getItem('refreshToken');
  
  if (!refreshToken) {
    throw new Error('No refresh token available');
  }

  // If a refresh is already in progress, wait for it
  if (refreshPromise) {
    return refreshPromise;
  }

  refreshPromise = (async () => {
    try {
      const response = await fetch(`${BASE_URL}/api/v1/auth/refresh`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ refresh_token: refreshToken }),
      });

      if (!response.ok) {
        throw new Error(`Token refresh failed: ${response.status}`);
      }

      const tokenData = await response.json();
      
      // Update localStorage with new tokens
      localStorage.setItem('accessToken', tokenData.access_token);
      localStorage.setItem('refreshToken', tokenData.refresh_token);
      
      // Dispatch update to Redux store if available
      if (window.__REDUX_STORE__) {
        window.__REDUX_STORE__.dispatch({
          type: 'auth/setTokens',
          payload: tokenData
        });
      }

      return tokenData.access_token;
    } catch (error) {
      // Clear invalid tokens
      localStorage.removeItem('accessToken');
      localStorage.removeItem('refreshToken');
      throw error;
    } finally {
      refreshPromise = null;
    }
  })();

  return refreshPromise;
}

let store: AppStore;

export const injectStore = (_store: AppStore) => {
  store = _store;
  tokenManager.injectStore(_store);
};

/**
 * Make an API request with authentication and common handling
 */
async function apiRequest<T>(
  method: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH',
  path: string,
  options: RequestInit = {},
  isFormData = false
): Promise<T> {
  const { accessToken } = store.getState().Auth;
  const { currentOrgId: selectedOrgId } = store.getState().Organizations;

  const headers: HeadersInit = {};

  if (!isFormData) {
    headers['Content-Type'] = 'application/json';
  }

  if (accessToken) {
    headers['Authorization'] = `Bearer ${accessToken}`;
  }

  // Paths that should not have the organization context header.
  const excludedPaths = [
    '/auth/login',
    '/auth/refresh',
    '/auth/logout',
    '/auth/setup',
    '/auth/setup-status',
    '/auth/me',
    '/organizations',
  ];

  // Add X-Org-Context header if an organization is selected and the path is not excluded.
  if (
    selectedOrgId &&
    !excludedPaths.some(p => path.includes(p)) &&
    !path.startsWith('/admin')
  ) {
    headers['X-Org-Context'] = selectedOrgId.toString();
  }

  const config: RequestInit = {
    method,
    headers,
    body: options.body ? (isFormData ? options.body : JSON.stringify(options.body)) : undefined,
  };

  const url = path.startsWith('/api/v1') ? path : `/api/v1${path}`;
  let response = await fetch(url, config);

  if (response.status === 401 && !url.endsWith('/auth/refresh')) {
    // Access token might be expired, try to refresh it
    try {
      const { refreshToken: currentRefreshToken } = store.getState().Auth;
      if (!currentRefreshToken) {
        // If there's no refresh token, we can't refresh, so logout.
        throw new Error('No refresh token available.');
      }
      const newTokens = await refreshToken(currentRefreshToken);
      store.dispatch(setTokens(newTokens));
      tokenManager.onTokenUpdate(); // Reschedule proactive refresh

      // Retry the original request with the new token
      const newHeaders = { ...headers };
      (newHeaders as Record<string, string>)['Authorization'] = `Bearer ${newTokens.access_token}`;
      const newConfig = { ...config, headers: newHeaders };
      response = await fetch(url, newConfig);
    } catch (error) {
      // Refresh token failed or was unavailable, logout the user
      tokenManager.onLogout(); // Clear timers
      store.dispatch(logout());
      // Redirect to login page or show a message
      // Use replace to prevent the user from navigating back to the broken page
      window.location.replace('/login');
      // We throw an error to stop the promise chain of the original call.
      throw new Error('Session expired. Please log in again.');
    }
  }

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({})); // Gracefully handle non-JSON error responses
    let errorMessage = `Request failed with status ${response.status}: ${response.statusText}`;
    
    // Try to extract a more detailed error message from the response
    if (errorData.message) {
      errorMessage = errorData.message;
    } else if (errorData.error) {
      errorMessage = errorData.error;
    }

    const error = new Error(errorMessage);
    // Add status code to the error object for better error handling
    (error as any).status = response.status;
    (error as any).statusText = response.statusText;
    (error as any).url = url;
    (error as any).data = errorData;
    throw error;
  }
  
  // Handle cases where the response body might be empty (e.g., 204 No Content)
  const text = await response.text();
  if (!text) {
    return {} as T;
  }

  // Parse the JSON response
  const responseData = JSON.parse(text);
  console.log(`API response from ${path}:`, JSON.stringify(responseData));
  return responseData as T;
}

/**
 * Convenience methods for common HTTP methods
 */
const apiClient = {
  /**
   * Make a GET request
   */
  get: <T>(endpoint: string, options: RequestInit = {}): Promise<T> => {
    return apiRequest<T>('GET', endpoint, options);
  },

  /**
   * Make a POST request with JSON body
   */
  post: <T>(endpoint: string, body: any, options: RequestInit = {}): Promise<T> => {
    return apiRequest<T>('POST', endpoint, {
      ...options,
      body,
    });
  },
  
  /**
   * Make a PUT request with JSON body
   */
  put: <T>(endpoint: string, body: any, options: RequestInit = {}): Promise<T> => {
    return apiRequest<T>('PUT', endpoint, {
      ...options,
      body,
    });
  },
  
  /**
   * Make a DELETE request
   */
  delete: <T>(endpoint: string, options: RequestInit = {}): Promise<T> => {
    return apiRequest<T>('DELETE', endpoint, options);
  },

  /**
   * Make a PATCH request with JSON body
   */
  patch: <T>(endpoint: string, body: any, options: RequestInit = {}): Promise<T> => {
    return apiRequest<T>('PATCH', endpoint, {
      ...options,
      body,
    });
  },
};

export default apiClient;
