import { createSlice, createAsyncThunk, PayloadAction } from '@reduxjs/toolkit';
import { checkAdminPasswordStatus, verifyAdminPassword, setAdminPassword } from '../../api/auth';

export type AuthState = {
  isAuthenticated: boolean;
  isPasswordSet: boolean;
  isLoading: boolean;
  error: string | null;
  token: string | null;
};

// Check if we have a stored token already
const storedToken = localStorage.getItem('authToken');

const initialState: AuthState = {
  isAuthenticated: !!storedToken, // Authenticated if we have a token
  isPasswordSet: false,
  isLoading: false,
  error: null,
  token: storedToken,
};

// Async thunks
export const checkPasswordStatus = createAsyncThunk(
  'auth/checkPasswordStatus',
  async (_, { rejectWithValue }) => {
    try {
      return await checkAdminPasswordStatus();
    } catch (error) {
      return rejectWithValue((error as Error).message);
    }
  }
);

export const loginAdmin = createAsyncThunk(
  'auth/loginAdmin',
  async (password: string, { rejectWithValue }) => {
    try {
      const result = await verifyAdminPassword(password);
      if (!result.valid) {
        throw new Error('Invalid password');
      }
      return result.token || null;
    } catch (error) {
      return rejectWithValue((error as Error).message);
    }
  }
);

export const setInitialPassword = createAsyncThunk(
  'auth/setInitialPassword',
  async (password: string, { rejectWithValue }) => {
    try {
      const result = await setAdminPassword(password);
      if (!result.success) {
        throw new Error('Failed to set password');
      }
      return result.token || null;
    } catch (error) {
      return rejectWithValue((error as Error).message);
    }
  }
);

const authSlice = createSlice({
  name: 'auth',
  initialState,
  reducers: {
    logout: (state) => {
      state.isAuthenticated = false;
      state.token = null;
      state.error = null;
      // Also remove from localStorage
      localStorage.removeItem('authToken');
    },
    clearError: (state) => {
      state.error = null;
    },
  },
  extraReducers: (builder) => {
    builder
      // Check password status
      .addCase(checkPasswordStatus.pending, (state) => {
        state.isLoading = true;
        state.error = null;
      })
      .addCase(checkPasswordStatus.fulfilled, (state, action: PayloadAction<boolean>) => {
        state.isLoading = false;
        state.isPasswordSet = action.payload;
      })
      .addCase(checkPasswordStatus.rejected, (state, action) => {
        state.isLoading = false;
        state.error = action.payload as string;
      })
      
      // Login
      .addCase(loginAdmin.pending, (state) => {
        state.isLoading = true;
        state.error = null;
      })
      .addCase(loginAdmin.fulfilled, (state, action: PayloadAction<string | null>) => {
        state.isLoading = false;
        state.isAuthenticated = true;
        state.token = action.payload;
      })
      .addCase(loginAdmin.rejected, (state, action) => {
        state.isLoading = false;
        state.error = action.payload as string;
      })
      
      // Set initial password
      .addCase(setInitialPassword.pending, (state) => {
        state.isLoading = true;
        state.error = null;
      })
      .addCase(setInitialPassword.fulfilled, (state, action: PayloadAction<string | null>) => {
        state.isLoading = false;
        state.isPasswordSet = true;
        state.isAuthenticated = true;
        state.token = action.payload;
      })
      .addCase(setInitialPassword.rejected, (state, action) => {
        state.isLoading = false;
        state.error = action.payload as string;
      });
  },
});

export const { logout, clearError } = authSlice.actions;

export default authSlice.reducer;
