import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { Provider } from 'react-redux';
import configureStore from 'redux-mock-store';
import LicenseTab from './LicenseTab';

const mockStore = configureStore([]);

const baseLicense = { status: 'active', updating: false, loading: false, refreshing: false, unlimited: false, loadedOnce: true };

function setup(orgRole: string) {
  const store = mockStore({
    License: baseLicense,
    Auth: { organizations: [{ id: 1, name: 'Org', role: orgRole }], user: { id: 1, email: 'a@b.c', created_at: '', updated_at: '' } }
  });
  render(
    <Provider store={store}>
      <LicenseTab />
    </Provider>
  );
  return store;
}

describe('LicenseTab', () => {
  it('denies access for member role', () => {
    setup('member');
    expect(screen.getByTestId('license-tab-deny')).toBeInTheDocument();
  });

  it('shows tab for owner role', () => {
    setup('owner');
    expect(screen.getByTestId('license-tab')).toBeInTheDocument();
  });
});
