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
  async (params: { email: string; password: string; orgName: string }, { rejectWithValue }) => {
    try {
      const response = await apiSetupAdmin(params.email, params.password, params.orgName);
      return response;
    } catch (error) {
      return rejectWithValue((error as Error).message);
    }
  }
);

export const login = createAsyncThunk(
  'auth/login',
  async (params: { email: string; password: string }, { rejectWithValue }) => {
    try {
      const response = await apiLogin(params.email, params.password);
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
