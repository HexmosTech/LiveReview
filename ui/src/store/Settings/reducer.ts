import { createSlice, PayloadAction } from '@reduxjs/toolkit';

export type SettingsState = {
    domain: string;
    isConfigured: boolean;
};

const initialState: SettingsState = {
    domain: '',
    isConfigured: false
};

const settingsSlice = createSlice({
    name: 'settings',
    initialState,
    reducers: {
        updateDomain: (state, action: PayloadAction<string>) => {
            state.domain = action.payload;
            state.isConfigured = action.payload.trim() !== '';
        },
    },
});

export const { updateDomain } = settingsSlice.actions;

export default settingsSlice.reducer;
