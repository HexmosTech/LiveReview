import { createSlice, createAsyncThunk, PayloadAction } from '@reduxjs/toolkit';
import {
  checkSetupStatus as apiCheckSetupStatus,
  setupAdmin as apiSetupAdmin,
  login as apiLogin,
  logout as apiLogout,
  getMe as apiGetMe,
  LoginResponse,
  UserInfo,
  OrgInfo,
  SetupStatusResponse,
  SetupAdminResponse,
} from '../../api/auth';
import { TokenPair } from '../../api/auth';
import { isCloudMode } from '../../utils/deploymentMode';

export type AuthState = {
  isAuthenticated: boolean;
  isSetupRequired: boolean;
  isLoading: boolean;
  error: string | null;
  user: UserInfo | null;
  organizations: OrgInfo[];
  accessToken: string | null;
  refreshToken: string | null;
};

// Check for tokens in localStorage
const accessToken = localStorage.getItem('accessToken');
const refreshToken = localStorage.getItem('refreshToken');

const initialState: AuthState = {
  isAuthenticated: !!accessToken,
  isSetupRequired: false,
  isLoading: false,
  error: null,
  user: null,
  organizations: [],
  accessToken: accessToken,
  refreshToken: refreshToken,
};

// --- New Async Thunks ---

export const checkSetupStatus = createAsyncThunk(
  'auth/checkSetupStatus',
  async (_, { rejectWithValue }) => {
    try {
      const status = await apiCheckSetupStatus();
      return status;
    } catch (error) {
      return rejectWithValue((error as Error).message);
    }
  }
);

export const setupAdmin = createAsyncThunk(
  'auth/setupAdmin',
  async (params: { email: string; password: string; orgName: string }, { rejectWithValue, dispatch }) => {
    try {
      const response = await apiSetupAdmin(params.email, params.password, params.orgName);
      
      // Immediately populate Organizations store to avoid extra API call
      if (response.organizations && response.organizations.length > 0) {
        dispatch({ type: 'organizations/setOrganizationsFromAuth', payload: response.organizations });
      }
      
      return response;
    } catch (error) {
      return rejectWithValue((error as Error).message);
    }
  }
);

export const login = createAsyncThunk(
  'auth/login',
  async (params: { email: string; password: string }, { rejectWithValue, dispatch }) => {
    try {
      const response = await apiLogin(params.email, params.password);
      
      // Immediately populate Organizations store to avoid extra API call
      if (response.organizations && response.organizations.length > 0) {
        dispatch({ type: 'organizations/setOrganizationsFromAuth', payload: response.organizations });
      }
      
      return response;
    } catch (error) {
      return rejectWithValue((error as Error).message);
    }
  }
);

export const logout = createAsyncThunk('auth/logout', async (_, { getState }) => {
  const state = getState() as { Auth: AuthState };
  const token = state.Auth.refreshToken;
  
  try {
    await apiLogout(token || undefined);
  } catch (error) {
    // Even if the server logout fails, we should still clear local state
    console.warn('Server logout failed, but clearing local state:', error);
  }
  // Always return success so the UI can clear state
  return;
});

export const fetchUser = createAsyncThunk('auth/fetchUser', async (_, { rejectWithValue }) => {
  try {
    const response = await apiGetMe();
    return response;
  } catch (error) {
    return rejectWithValue((error as Error).message);
  }
});


const authSlice = createSlice({
  name: 'auth',
  initialState,
  reducers: {
    clearError: (state) => {
      state.error = null;
    },
    setTokens: (state, action: PayloadAction<TokenPair>) => {
      state.accessToken = action.payload.access_token;
      state.refreshToken = action.payload.refresh_token;
      state.isAuthenticated = true;
      localStorage.setItem('accessToken', action.payload.access_token);
      localStorage.setItem('refreshToken', action.payload.refresh_token);
      
      // Notify token manager of token update (for proactive refresh scheduling)
      if (typeof window !== 'undefined' && (window as any).tokenManager) {
        (window as any).tokenManager.onTokenUpdate();
      }
    },
  },
  extraReducers: (builder) => {
    builder
      // Check setup status
      .addCase(checkSetupStatus.pending, (state) => {
        state.isLoading = true;
      })
      .addCase(checkSetupStatus.fulfilled, (state, action: PayloadAction<SetupStatusResponse>) => {
        state.isLoading = false;
        state.isSetupRequired = action.payload.setup_required;
      })
      .addCase(checkSetupStatus.rejected, (state, action) => {
        state.isLoading = false;
        state.error = action.payload as string;
      })

      // Setup Admin
      .addCase(setupAdmin.pending, (state) => {
        state.isLoading = true;
        state.error = null;
      })
      .addCase(setupAdmin.fulfilled, (state, action: PayloadAction<SetupAdminResponse>) => {
        state.isLoading = false;
        state.isAuthenticated = true;
        state.isSetupRequired = false;
        state.user = action.payload.user;
        state.organizations = action.payload.organizations;
        state.accessToken = action.payload.tokens.access_token;
        state.refreshToken = action.payload.tokens.refresh_token;
        localStorage.setItem('accessToken', action.payload.tokens.access_token);
        localStorage.setItem('refreshToken', action.payload.tokens.refresh_token);
        
        // Set the first organization as current to avoid API calls without org context
        if (action.payload.organizations && action.payload.organizations.length > 0) {
          const firstOrg = action.payload.organizations[0];
          localStorage.setItem('currentOrgId', firstOrg.id.toString());
        }
      })
      .addCase(setupAdmin.rejected, (state, action) => {
        state.isLoading = false;
        state.error = action.payload as string;
      })

      // Login
      .addCase(login.pending, (state) => {
        state.isLoading = true;
        state.error = null;
      })
      .addCase(login.fulfilled, (state, action: PayloadAction<LoginResponse>) => {
        state.isLoading = false;
        state.isAuthenticated = true;
        state.user = action.payload.user;
        state.organizations = action.payload.organizations;
        state.accessToken = action.payload.tokens.access_token;
        state.refreshToken = action.payload.tokens.refresh_token;
        localStorage.setItem('accessToken', action.payload.tokens.access_token);
        localStorage.setItem('refreshToken', action.payload.tokens.refresh_token);
        
        // Set the first organization as current to avoid API calls without org context
        if (action.payload.organizations && action.payload.organizations.length > 0) {
          const firstOrg = action.payload.organizations[0];
          localStorage.setItem('currentOrgId', firstOrg.id.toString());
        }
      })
      .addCase(login.rejected, (state, action) => {
        state.isLoading = false;
        state.error = action.payload as string;
      })

      // Logout
      .addCase(logout.pending, (state) => {
        state.isLoading = true;
      })
      .addCase(logout.fulfilled, (state) => {
        state.isLoading = false;
        state.isAuthenticated = false;
        state.user = null;
        state.accessToken = null;
        state.refreshToken = null;
        state.organizations = [];
        localStorage.removeItem('accessToken');
        localStorage.removeItem('refreshToken');
        
        // Clear Hexmos SSO cookies in cloud mode
        if (isCloudMode()) {
          document.cookie = 'hexmos-one=; path=/; domain=.hexmos.com; expires=Thu, 01 Jan 1970 00:00:00 GMT';
          document.cookie = 'hexmos-one-id=; path=/; domain=.hexmos.com; expires=Thu, 01 Jan 1970 00:00:00 GMT';
        }
      })
      .addCase(logout.rejected, (state) => {
        // Even if logout fails on the server, clear local state
        state.isLoading = false;
        state.isAuthenticated = false;
        state.user = null;
        state.accessToken = null;
        state.refreshToken = null;
        state.organizations = [];
        localStorage.removeItem('accessToken');
        localStorage.removeItem('refreshToken');
        
        // Clear Hexmos SSO cookies in cloud mode
        if (isCloudMode()) {
          document.cookie = 'hexmos-one=; path=/; domain=.hexmos.com; expires=Thu, 01 Jan 1970 00:00:00 GMT';
          document.cookie = 'hexmos-one-id=; path=/; domain=.hexmos.com; expires=Thu, 01 Jan 1970 00:00:00 GMT';
        }
      })

      // Fetch User
      .addCase(fetchUser.fulfilled, (state, action) => {
        state.user = action.payload.user;
        state.organizations = action.payload.organizations;
        state.isAuthenticated = true;
      })
      .addCase(fetchUser.rejected, (state) => {
        // This can happen if the token is invalid, so we log out
        state.isAuthenticated = false;
        state.user = null;
        state.accessToken = null;
        state.refreshToken = null;
        state.organizations = [];
        localStorage.removeItem('accessToken');
        localStorage.removeItem('refreshToken');
      });
  },
});

export const { clearError, setTokens } = authSlice.actions;

export default authSlice.reducer;
