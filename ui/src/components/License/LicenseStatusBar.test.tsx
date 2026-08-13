import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { Provider } from 'react-redux';
import configureStore from 'redux-mock-store';
import { thunk } from 'redux-thunk';
import LicenseStatusBar from './LicenseStatusBar';

jest.mock('../../api/system', () => ({
  getSystemInfo: jest.fn(() => Promise.resolve({ deployment_mode: 'production' }))
}));

// triggerLicenseRefresh is a createAsyncThunk — needs thunk middleware to actually
// dispatch its pending action instead of pushing the raw thunk function into the log.
// redux-thunk v3's bundled types target a newer `redux` (UnknownAction) than
// @types/redux-mock-store's pinned Middleware type (AnyAction) — the `any` cast
// is a test-only shim for that version skew, not a real behavioral difference.
const mockStore = configureStore([thunk as any]);

function setup(stateOverrides: any = {}) {
  const store = mockStore({
    License: {
      status: 'missing', updating: false, loading: false, refreshing: false, lastError: undefined,
      unlimited: false, loadedOnce: true, ...stateOverrides
    }
  });
  const onOpenModal = jest.fn();
  render(
    <Provider store={store}>
      <LicenseStatusBar onOpenModal={onOpenModal} />
    </Provider>
  );
  return { store, onOpenModal };
}

describe('LicenseStatusBar', () => {
  it('renders missing state and triggers modal open', () => {
    const { onOpenModal } = setup();
    expect(screen.getByTestId('license-status-bar')).toBeInTheDocument();
    expect(screen.getByText(/Missing/i)).toBeInTheDocument();
    fireEvent.click(screen.getByText(/Have a license/i));
    expect(onOpenModal).toHaveBeenCalled();
  });

  it('renders active state and can refresh', () => {
    const { store } = setup({ status: 'active', expiresAt: new Date(Date.now()+86400000).toISOString() });
    expect(screen.getByText(/Licensed/)).toBeInTheDocument();
    const refreshBtn = screen.getByRole('button', { name: /Refresh license/i });
    fireEvent.click(refreshBtn);
    const actions = store.getActions().map(a => a.type);
    expect(actions.some(a => a.includes('license/refresh'))).toBeTruthy();
  });

  it('shows loading state before first status load', () => {
    setup({ loadedOnce: false, loading: true });
    expect(screen.getByText(/Loading license/i)).toBeInTheDocument();
  });
});
