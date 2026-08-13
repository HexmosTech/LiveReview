import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { Provider } from 'react-redux';
import configureStore from 'redux-mock-store';
import { thunk } from 'redux-thunk';
import LicenseModal from './LicenseModal';

// submitLicenseToken is a createAsyncThunk — needs thunk middleware to actually
// dispatch its pending/fulfilled/rejected actions instead of pushing the raw
// thunk function into the mock store's action log. redux-thunk v3's bundled
// types target a newer `redux` (UnknownAction) than @types/redux-mock-store's
// pinned Middleware type (AnyAction) — the `any` cast is a test-only shim for
// that version skew, not a real behavioral difference.
const mockStore = configureStore([thunk as any]);

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
    expect(actions.some(t => t.includes('license/submitToken'))).toBeTruthy();
  });
});
