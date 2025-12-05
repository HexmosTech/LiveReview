/**
 * Simple API client with base URL and authentication support
 */

import { AppStore, StoreDispatch } from '../store/configureStore';
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
// Priority: 1) Runtime injected config, 2) Auto-detect based on deployment mode
function getBaseUrl(): string {
  console.log('�🚨🚨 === API CLIENT DEBUG START === 🚨🚨🚨');
  console.log('�🔍 getBaseUrl() called at:', new Date().toISOString());
  console.log('🔍 window.LIVEREVIEW_CONFIG:', JSON.stringify(window.LIVEREVIEW_CONFIG, null, 2));
  console.log('🔍 typeof window.LIVEREVIEW_CONFIG:', typeof window.LIVEREVIEW_CONFIG);
  
  // First try runtime config injected by Go server
  console.log('🔍 Checking runtime config...');
  console.log('🔍 window.LIVEREVIEW_CONFIG exists:', !!window.LIVEREVIEW_CONFIG);
  console.log('🔍 window.LIVEREVIEW_CONFIG.apiUrl:', window.LIVEREVIEW_CONFIG?.apiUrl);
  console.log('🔍 apiUrl is not null:', window.LIVEREVIEW_CONFIG?.apiUrl !== null);
  
  if (window.LIVEREVIEW_CONFIG?.apiUrl && window.LIVEREVIEW_CONFIG.apiUrl !== null) {
    console.log('✅✅✅ Using runtime config apiUrl:', window.LIVEREVIEW_CONFIG.apiUrl);
    console.log('🚨🚨🚨 === API CLIENT DEBUG END === 🚨🚨🚨');
    return window.LIVEREVIEW_CONFIG.apiUrl;
  }
  
  console.log('⚠️⚠️⚠️ No runtime config found, using auto-detection');
  
  // Auto-detection based on current URL and deployment mode
  const currentUrl = new URL(window.location.href);
  const port = currentUrl.port;
  
  console.log('🔍 Current URL:', currentUrl.href);
  console.log('🔍 Current URL port:', port);
  console.log('🔍 Current URL hostname:', currentUrl.hostname);
  
  // If we're on localhost with port 8081, we're in demo mode (API on port 8888)
  if (currentUrl.hostname === 'localhost' && port === '8081') {
    const detectedUrl = `${currentUrl.protocol}//localhost:8888`;
    console.log('🔍 Demo mode detected, using localhost:8888');
    return detectedUrl;
  }
  
  // If we're on localhost with a different port, use the same port for API
  if (currentUrl.hostname === 'localhost' && port) {
    const detectedUrl = `${currentUrl.protocol}//${currentUrl.hostname}:${port}`;
    console.log('🔍 Localhost with port detected:', detectedUrl);
    return detectedUrl;
  }
  
  // For production (reverse proxy), construct API URL from current URL
  // Remove port if present and use same protocol/hostname
  const baseUrl = `${currentUrl.protocol}//${currentUrl.hostname}`;
  console.log('🔍 Production mode detected, using current URL base:', baseUrl);
  console.log('🔍 This will be used for API calls - no /api suffix needed as it will be added by URL construction');
  
  return baseUrl;
}

const BASE_URL = getBaseUrl();
console.log('🚀 API Client initialized with BASE_URL:', BASE_URL);
console.log('Final BASE_URL configured as:', BASE_URL);
console.log('🔍 Current window location:', window.location.href);
console.log('🔍 Current window.location details:', {
  protocol: window.location.protocol,
  hostname: window.location.hostname,
  port: window.location.port,
  pathname: window.location.pathname
});

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
  ];

  // Explicitly exclude only the base /organizations endpoint (for listing user orgs), not sub-paths
  const isBaseOrganizationsEndpoint = path === '/organizations' || path.startsWith('/organizations?');

  // Add X-Org-Context header if an organization is selected and the path is not excluded.
  if (
    selectedOrgId &&
    !excludedPaths.some(p => path.includes(p)) &&
    !isBaseOrganizationsEndpoint &&
    !path.startsWith('/admin')
  ) {
    headers['X-Org-Context'] = selectedOrgId.toString();
  }

  const config: RequestInit = {
    method,
    headers,
    body: options.body ? (isFormData ? options.body : JSON.stringify(options.body)) : undefined,
  };

  // Construct the full URL - handle cases where BASE_URL already includes /api
  console.log('🚨🚨🚨 === URL CONSTRUCTION DEBUG === 🚨🚨🚨');
  console.log('🔍 URL Construction Inputs:', {
    path,
    BASE_URL,
    pathStartsWithApiV1: path.startsWith('/api/v1'),
    baseUrlEndsWithApi: BASE_URL.endsWith('/api'),
    timestamp: new Date().toISOString()
  });
  
  let url: string;
  if (path.startsWith('/api/v1')) {
    // Path already includes /api/v1, just append to base URL
    url = `${BASE_URL}${path}`;
    console.log('🔍 Path starts with /api/v1, direct append:', url);
  } else {
    // Need to add /api/v1 prefix, but check if BASE_URL already ends with /api
    const baseUrlEndsWithApi = BASE_URL.endsWith('/api');
    console.log('🔍 BASE_URL ends with /api:', baseUrlEndsWithApi);
    if (baseUrlEndsWithApi) {
      url = `${BASE_URL}/v1${path}`;
      console.log('🔍 BASE_URL ends with /api, constructed:', url);
    } else {
      url = `${BASE_URL}/api/v1${path}`;
      console.log('🔍 BASE_URL does not end with /api, constructed:', url);
    }
  }
  console.log('🚨🚨🚨 FINAL API REQUEST URL:', url);
  console.log('🚨🚨🚨 === URL CONSTRUCTION DEBUG END === 🚨🚨🚨');
  let response = await fetch(url, config);

  if (response.status === 401 && !url.endsWith('/auth/refresh')) {
    // Do NOT trigger refresh/logout for auth endpoints like login/setup.
    const isAuthInitiationRequest =
      path.includes('/auth/login') ||
      path.includes('/auth/setup') ||
      path.includes('/auth/setup-status') ||
      path.includes('/auth/logout');

    if (isAuthInitiationRequest) {
      // Surface the 401 to the caller so the UI can show the error message.
      const errorData = await response.json().catch(() => ({}));
      const err: any = new Error(
        errorData?.message || errorData?.error || 'Unauthorized'
      );
      err.status = response.status;
      err.url = url;
      err.data = errorData;
      throw err;
    }

    // If the user isn't authenticated (no refresh token), don't try to refresh or logout.
    const { refreshToken: currentRefreshToken } = store.getState().Auth;
    if (!currentRefreshToken) {
      const errorData = await response.json().catch(() => ({}));
      const err: any = new Error(
        errorData?.message || errorData?.error || 'Unauthorized'
      );
      err.status = response.status;
      err.url = url;
      err.data = errorData;
      throw err;
    }

    // Otherwise, access token might be expired, try to refresh it
    try {
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
  (store.dispatch as StoreDispatch)(logout());
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
