import { createAsyncThunk, createSlice, PayloadAction } from '@reduxjs/toolkit';
import { getLicenseStatus, updateLicense, refreshLicense, LicenseStatusResponse } from '../../api/license';
import { initialLicenseState, LicenseStateSlice } from './types';

// Thunks
export const fetchLicenseStatus = createAsyncThunk('license/fetchStatus', async () => {
  const data = await getLicenseStatus();
  return data as LicenseStatusResponse;
});

export const submitLicenseToken = createAsyncThunk('license/submitToken', async (token: string) => {
  const data = await updateLicense(token);
  return data as LicenseStatusResponse;
});

export const triggerLicenseRefresh = createAsyncThunk('license/refresh', async () => {
  const data = await refreshLicense();
  return data as LicenseStatusResponse;
});

function applyStatus(state: LicenseStateSlice, payload: LicenseStatusResponse) {
  state.status = payload.status as any;
  state.subject = payload.subject;
  state.appName = payload.appName;
  state.seatCount = payload.seatCount;
  state.unlimited = payload.unlimited;
  state.expiresAt = payload.expiresAt;
  state.lastValidatedAt = payload.lastValidatedAt;
  state.lastValidationCode = payload.lastValidationCode as any;
}

const slice = createSlice({
  name: 'License',
  initialState: initialLicenseState,
  reducers: {
    openModal: state => { state.modalOpen = true; },
    closeModal: state => { state.modalOpen = false; },
  },
  extraReducers: builder => {
    builder
      .addCase(fetchLicenseStatus.pending, state => {
        state.loading = true; state.lastError = undefined;
      })
      .addCase(fetchLicenseStatus.fulfilled, (state, action: PayloadAction<LicenseStatusResponse>) => {
        state.loading = false; state.loadedOnce = true; applyStatus(state, action.payload);
      })
      .addCase(fetchLicenseStatus.rejected, (state, action) => {
        state.loading = false; state.loadedOnce = true; state.lastError = action.error.message;
      })
      .addCase(submitLicenseToken.pending, state => {
        state.updating = true; state.lastError = undefined;
      })
      .addCase(submitLicenseToken.fulfilled, (state, action: PayloadAction<LicenseStatusResponse>) => {
        state.updating = false; applyStatus(state, action.payload);
      })
      .addCase(submitLicenseToken.rejected, (state, action) => {
        state.updating = false; state.lastError = action.error.message;
      })
      .addCase(triggerLicenseRefresh.pending, state => {
        state.refreshing = true; state.lastError = undefined;
      })
      .addCase(triggerLicenseRefresh.fulfilled, (state, action: PayloadAction<LicenseStatusResponse>) => {
        state.refreshing = false; applyStatus(state, action.payload);
      })
      .addCase(triggerLicenseRefresh.rejected, (state, action) => {
        state.refreshing = false; state.lastError = action.error.message;
      });
  }
});

export const { openModal, closeModal } = slice.actions;
export default slice.reducer;
