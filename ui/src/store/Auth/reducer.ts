import { createSlice, createAsyncThunk, PayloadAction } from '@reduxjs/toolkit';
import { checkAdminPasswordStatus, verifyAdminPassword, setAdminPassword } from '../../api/auth';

export type AuthState = {
  isAuthenticated: boolean;
  isPasswordSet: boolean;
  isLoading: boolean;
  error: string | null;
  password: string | null; // Only store the actual password
};

// Check if we have a stored password
const storedPassword = localStorage.getItem('authPassword');

const initialState: AuthState = {
  isAuthenticated: !!storedPassword, // Authenticated if we have a password
  isPasswordSet: false,
  isLoading: false,
  error: null,
  password: storedPassword,
};

// Async thunks
export const checkPasswordStatus = createAsyncThunk(
  'auth/checkPasswordStatus',
  async (_, { rejectWithValue }) => {
    try {
      console.log('Dispatching checkPasswordStatus thunk...');
      const isSet = await checkAdminPasswordStatus();
      console.log('Password status API response - isSet:', isSet);
      return isSet;
    } catch (error) {
      console.error('Error in checkPasswordStatus thunk:', error);
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
      // Just return the password
      return password;
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
      // Just return the password
      return password;
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
      // Clear all auth state on logout
      state.isAuthenticated = false;
      state.isPasswordSet = false;  // Explicitly clear this flag
      state.password = null;  // Clear the password
      state.error = null;
      
      // Remove from localStorage
      localStorage.removeItem('authPassword');
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
        console.log('checkPasswordStatus fulfilled - setting isPasswordSet to:', action.payload);
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
      .addCase(loginAdmin.fulfilled, (state, action: PayloadAction<string>) => {
        state.isLoading = false;
        state.isAuthenticated = true;
        state.password = action.payload;
        
        // Store password in localStorage for persistence
        localStorage.setItem('authPassword', action.payload);
        
        console.log('Login successful - stored password in Redux state');
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
      .addCase(setInitialPassword.fulfilled, (state, action: PayloadAction<string>) => {
        state.isLoading = false;
        state.isPasswordSet = true;
        state.isAuthenticated = true;
        state.password = action.payload;
        
        // Store password in localStorage for persistence
        localStorage.setItem('authPassword', action.payload);
        
        console.log('Password set successfully - stored password in Redux state');
      })
      .addCase(setInitialPassword.rejected, (state, action) => {
        state.isLoading = false;
        state.error = action.payload as string;
      });
  },
});

export const { logout, clearError } = authSlice.actions;

export default authSlice.reducer;
