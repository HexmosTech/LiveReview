import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { Provider } from 'react-redux';
import configureStore from 'redux-mock-store';
import LicenseStatusBar from './LicenseStatusBar';

const mockStore = configureStore([]);

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
    fireEvent.click(screen.getByText(/Enter License/i));
    expect(onOpenModal).toHaveBeenCalled();
  });

  it('renders active state and can refresh', () => {
    const { store } = setup({ status: 'active', expiresAt: new Date(Date.now()+86400000).toISOString() });
    expect(screen.getByText(/Active/)).toBeInTheDocument();
    const refreshBtn = screen.getByRole('button', { name: /Refresh license/i });
    fireEvent.click(refreshBtn);
    const actions = store.getActions().map(a => a.type);
    expect(actions.some(a => a.includes('license/refresh'))).toBeTruthy();
  });
});
