import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { Provider } from 'react-redux';
import configureStore from 'redux-mock-store';
import LicenseModal from './LicenseModal';

const mockStore = configureStore([]);

describe('LicenseModal', () => {
  it('renders when open and can submit token', () => {
    const store = mockStore({
      License: { status: 'missing', loading: false, updating: false, error: null, lastChecked: null }
    });
    const onClose = jest.fn();
    render(
      <Provider store={store}>
        <LicenseModal open={true} onClose={onClose} />
      </Provider>
    );

  expect(screen.getByText(/Enter License Token/i)).toBeInTheDocument();
  const textarea = screen.getByPlaceholderText(/Paste license JWT here/i);
    fireEvent.change(textarea, { target: { value: 'abc.def.ghi' } });
  const btn = screen.getByRole('button', { name: /Save Token/i });
    fireEvent.click(btn);
    const actions = store.getActions().map(a => a.type);
    expect(actions.some(t => t.includes('submitLicenseToken'))).toBeTruthy();
  });
});
